package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"futrou-cli/src/api"
	"futrou-cli/src/cliconfig"
	"futrou-cli/src/logger"

	"github.com/quic-go/quic-go/http3"
)

func init() {
	// Suppress quic-go's informational warning about the OS UDP receive buffer.
	if os.Getenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING") == "" {
		os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	}
}

// ApiClient handles communication with the Futrou API.
type ApiClient struct {
	client        *http.Client
	apiUrl        string
	apiToken      string
	afterMutation func() error
}

// SetAfterMutation installs a callback run after a successful write request.
func (ac *ApiClient) SetAfterMutation(callback func() error) { ac.afterMutation = callback }

// NormalizeApiUrl strips a trailing slash and a trailing "/v2" so that both
// "https://api.futrou.com" and "https://api.futrou.com/v2" resolve the same.
func NormalizeApiUrl(apiUrl string) string {
	apiUrl = strings.TrimRight(apiUrl, "/")
	apiUrl = strings.TrimSuffix(apiUrl, "/v2")
	return apiUrl
}

// NewApiClient creates a client loaded from config/env.
// apiUrl and token override config/env values when non-empty.
func NewApiClient(apiUrl, token string) (*ApiClient, error) {
	cfg, err := cliconfig.Load()
	if err != nil {
		return nil, err
	}
	if apiUrl != "" {
		cfg.ApiUrl = apiUrl
	}
	resolvedToken := cfg.TokenFor(cfg.ApiUrl)
	if token != "" {
		resolvedToken = token
	}
	return &ApiClient{
		apiUrl:   NormalizeApiUrl(cfg.ApiUrl),
		apiToken: resolvedToken,
		client:   newHttpClient(30 * time.Second),
	}, nil
}

// ApiToken returns the API token used by this client.
func (ac *ApiClient) ApiToken() string {
	return ac.apiToken
}

// ApiUrl returns the normalized base API URL used by this client.
func (ac *ApiClient) ApiUrl() string {
	return ac.apiUrl
}

// ToJSONSchema returns the OpenAPI schema served by the configured API.
func (ac *ApiClient) ToJSONSchema() ([]byte, error) {
	resp, err := ac.do(context.Background(), http.MethodGet, "/v2/openapi.json", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching schema: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading schema: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetching schema: API returned %s", resp.Status)
	}
	return data, nil
}

// NewApiClientWithToken creates a client with explicit url and token (no config file lookup).
func NewApiClientWithToken(apiUrl, apiToken string) *ApiClient {
	return &ApiClient{
		apiUrl:   NormalizeApiUrl(apiUrl),
		apiToken: apiToken,
		client:   newHttpClient(30 * time.Second),
	}
}

// Request makes a JSON request and returns (body, statusCode, error).
// On HTTP 4xx/5xx it returns an *api.APIError.
func (ac *ApiClient) Request(method, path string, body interface{}) (interface{}, int, error) {
	resp, err := ac.do(context.Background(), method, path, body)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	logger.Debug("API %s %s → %d (%d bytes)", method, path, resp.StatusCode, len(respBody))

	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, &api.APIError{Message: http.StatusText(resp.StatusCode)}
		}
		return nil, resp.StatusCode, nil
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/plain") {
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, &api.APIError{Message: string(respBody)}
		}
		return string(respBody), resp.StatusCode, nil
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parsing response: %s", string(respBody))
	}

	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return nil, resp.StatusCode, &apiErr
		}
		return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d", resp.StatusCode)}
	}

	return result, resp.StatusCode, nil
}

// RequestInto makes a JSON request and unmarshals the response body into v.
func (ac *ApiClient) RequestInto(method, path string, body interface{}, v interface{}) (int, error) {
	resp, err := ac.do(context.Background(), method, path, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return resp.StatusCode, &apiErr
		}
		if message := strings.TrimSpace(string(respBody)); message != "" {
			return resp.StatusCode, &api.APIError{Message: message}
		}
		return resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d", resp.StatusCode)}
	}

	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, ac.runAfterMutation(method)
	}

	if err := json.Unmarshal(respBody, v); err != nil {
		return resp.StatusCode, fmt.Errorf("parsing response: %w", err)
	}
	return resp.StatusCode, ac.runAfterMutation(method)
}

func (ac *ApiClient) runAfterMutation(method string) error {
	if isMutation(method) && ac.afterMutation != nil {
		return ac.afterMutation()
	}
	return nil
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut || method == http.MethodDelete
}

func (ac *ApiClient) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := strings.TrimSuffix(ac.apiUrl, "/") + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil || method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	}
	if ac.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+ac.apiToken)
	}

	logger.Debug("→ %s %s", method, url)
	return ac.client.Do(req)
}

// newHttpClient builds an http.Client restricted to TLS 1.2/1.3, with
// HTTP/2 support over TLS and an opportunistic HTTP/3 upgrade.
func newHttpClient(timeout time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	h3Transport := &http3.Transport{TLSClientConfig: tlsConfig}
	h1h2Transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		ForceAttemptHTTP2: true,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &fallbackTransport{primary: h3Transport, fallback: h1h2Transport},
	}
}

// fallbackTransport tries HTTP/3 first for HTTPS and transparently falls back
// to HTTP/1.1 or HTTP/2 when QUIC is unavailable.
type fallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		if resp, err := t.primary.RoundTrip(req.Clone(req.Context())); err == nil {
			return resp, nil
		}
	}
	return t.fallback.RoundTrip(req)
}
