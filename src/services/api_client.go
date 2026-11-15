package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"futrou-cli/src/api"
	"futrou-cli/src/cliconfig"
	"futrou-cli/src/logger"
	"futrou-cli/src/utils"

	"github.com/quic-go/quic-go/http3"
)

func init() {
	// Suppress quic-go's informational warning about the OS UDP receive buffer.
	if os.Getenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING") == "" {
		os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	}
}

// SetContext makes every request of the client stop when ctx is cancelled (Ctrl+C).
func (ac *ApiClient) SetContext(ctx context.Context) { ac.ctx = ctx }

func (ac *ApiClient) context() context.Context {
	if ac.ctx == nil {
		return context.Background()
	}
	return ac.ctx
}

// Detached is a copy of the client that a cancellation does not stop: what must
// still be sent after a cancel (marking a version failed) goes through it.
func (ac *ApiClient) Detached() *ApiClient {
	clone := *ac
	clone.ctx = nil
	return &clone
}

// ApiClient handles communication with the Futrou API.
type ApiClient struct {
	client *http.Client
	// transfer carries file uploads and downloads. It has no limit on the whole
	// exchange (a big file on a slow link takes as long as it takes) but still
	// gives up on a server that stops answering (see newTransferClient).
	transfer      *http.Client
	apiUrl        string
	apiToken      string
	afterMutation func() error
	ctx           context.Context
}

// SetAfterMutation installs a callback run after a successful write request.
func (ac *ApiClient) SetAfterMutation(callback func() error) { ac.afterMutation = callback }

// NewApiClient creates a client loaded from config/env.
// apiUrl and token override config/env values when non-empty.
func NewApiClient(apiUrl, token string) (*ApiClient, error) {
	cfg, err := cliconfig.Load()
	if err != nil {
		return nil, err
	}
	cfg.ApiUrl = utils.NormalizeApiUrl(cfg.ApiUrl)
	if apiUrl != "" {
		cfg.ApiUrl = utils.NormalizeApiUrl(apiUrl)
	}
	resolvedToken := cfg.TokenFor(cfg.ApiUrl)
	if token != "" {
		resolvedToken = token
	}
	return &ApiClient{
		apiUrl:   utils.NormalizeApiUrl(cfg.ApiUrl),
		apiToken: resolvedToken,
		client:   newHttpClient(requestTimeout),
		transfer: newTransferClient(),
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
	resp, err := ac.do(ac.context(), http.MethodGet, "/v2/openapi.json", nil)
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
		apiUrl:   utils.NormalizeApiUrl(apiUrl),
		apiToken: apiToken,
		client:   newHttpClient(requestTimeout),
		transfer: newTransferClient(),
	}
}

// Request makes a JSON request and returns (body, statusCode, error).
// On HTTP 4xx/5xx it returns an *api.APIError.
func (ac *ApiClient) Request(method, path string, body interface{}) (interface{}, int, error) {
	resp, err := ac.do(ac.context(), method, path, body)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, readError(method, path, err)
	}
	logger.Debug("← %s %s %d (%d bytes): %s", method, path, resp.StatusCode, len(respBody), truncateForLog(respBody))

	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("%s (HTTP %d %s %s)", http.StatusText(resp.StatusCode), resp.StatusCode, method, path)}
		}
		return nil, resp.StatusCode, nil
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/plain") {
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("%s (HTTP %d %s %s)", strings.TrimSpace(string(respBody)), resp.StatusCode, method, path)}
		}
		return string(respBody), resp.StatusCode, nil
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parsing response from %s %s: %s", method, path, string(respBody))
	}

	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return nil, resp.StatusCode, &apiErr
		}
		return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d (%s %s)", resp.StatusCode, method, path)}
	}

	return result, resp.StatusCode, nil
}

// RequestInto makes a JSON request and unmarshals the response body into v.
func (ac *ApiClient) RequestInto(method, path string, body interface{}, v interface{}) (int, error) {
	resp, err := ac.do(ac.context(), method, path, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, readError(method, path, err)
	}
	logger.Debug("← %s %s %d (%d bytes): %s", method, path, resp.StatusCode, len(respBody), truncateForLog(respBody))

	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return resp.StatusCode, &apiErr
		}
		if message := strings.TrimSpace(string(respBody)); message != "" {
			return resp.StatusCode, &api.APIError{Message: fmt.Sprintf("%s (HTTP %d %s %s)", message, resp.StatusCode, method, path)}
		}
		return resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d (%s %s)", resp.StatusCode, method, path)}
	}

	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, ac.runAfterMutation(method)
	}

	if err := json.Unmarshal(respBody, v); err != nil {
		return resp.StatusCode, fmt.Errorf("parsing response: %w", err)
	}
	return resp.StatusCode, ac.runAfterMutation(method)
}

// UploadFile PUTs raw file contents to path with the given content type and
// returns the response body, decoded as JSON when possible.
func (ac *ApiClient) UploadFile(path, contentType string, content io.Reader) (interface{}, int, error) {
	return ac.SendBody(http.MethodPut, path, contentType, content)
}

// SendBody sends a raw body (a file, or an archive of many) with method to path
// and returns the response body, decoded as JSON when possible.
func (ac *ApiClient) SendBody(method, path, contentType string, content io.Reader) (interface{}, int, error) {
	url := strings.TrimSuffix(ac.apiUrl, "/") + path
	req, err := http.NewRequestWithContext(ac.context(), method, url, content)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if ac.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+ac.apiToken)
	}

	logger.Debug("→ %s %s", method, url)
	resp, err := ac.transfer.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, readError(method, path, err)
	}
	logger.Debug("← %s %s %d (%d bytes): %s", method, path, resp.StatusCode, len(respBody), truncateForLog(respBody))
	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return nil, resp.StatusCode, &apiErr
		}
		if message := strings.TrimSpace(string(respBody)); message != "" {
			return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("%s (HTTP %d %s %s)", message, resp.StatusCode, method, path)}
		}
		return nil, resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d (%s %s)", resp.StatusCode, method, path)}
	}
	if len(respBody) == 0 {
		return nil, resp.StatusCode, ac.runAfterMutation(method)
	}
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), resp.StatusCode, ac.runAfterMutation(method)
	}
	return result, resp.StatusCode, ac.runAfterMutation(method)
}

// Stream GETs path and copies the response body to w as it arrives, so a big
// download is never held in memory. It returns the HTTP status.
func (ac *ApiClient) Stream(path string, w io.Writer) (int, error) {
	resp, err := ac.send(ac.transfer, ac.context(), http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var apiErr api.APIError
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return resp.StatusCode, &apiErr
		}
		return resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d (GET %s)", resp.StatusCode, path)}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return resp.StatusCode, fmt.Errorf("reading response from GET %s: %w", path, err)
	}
	return resp.StatusCode, nil
}

// DownloadFile GETs path and returns the raw response body alongside its
// Content-Type, for endpoints that serve file contents instead of JSON.
func (ac *ApiClient) DownloadFile(path string) ([]byte, string, int, error) {
	resp, err := ac.send(ac.transfer, ac.context(), http.MethodGet, path, nil)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, fmt.Errorf("reading response from GET %s: %w", path, err)
	}
	logger.Debug("← GET %s %d (%d bytes)", path, resp.StatusCode, len(data))
	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		if jsonErr := json.Unmarshal(data, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return nil, "", resp.StatusCode, &apiErr
		}
		if message := strings.TrimSpace(string(data)); message != "" {
			return nil, "", resp.StatusCode, &api.APIError{Message: fmt.Sprintf("%s (HTTP %d GET %s)", message, resp.StatusCode, path)}
		}
		return nil, "", resp.StatusCode, &api.APIError{Message: fmt.Sprintf("request failed: %d (GET %s)", resp.StatusCode, path)}
	}
	return data, resp.Header.Get("Content-Type"), resp.StatusCode, nil
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

// truncateForLog caps a response body shown in debug logs so a large or
// binary payload doesn't flood the terminal.
func truncateForLog(body []byte) string {
	const maxLen = 2000
	s := string(body)
	if len(s) > maxLen {
		return s[:maxLen] + fmt.Sprintf("... (%d more bytes)", len(s)-maxLen)
	}
	return s
}

func (ac *ApiClient) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	return ac.send(ac.client, ctx, method, path, body)
}

// readError is the error of a response whose body could not be read whole (the
// connection dropped): what was read is not the answer, so it is never used.
func readError(method, path string, err error) error {
	return fmt.Errorf("reading response from %s %s: %w", method, path, err)
}

func (ac *ApiClient) send(client *http.Client, ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
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
	return client.Do(req)
}

// requestTimeout is how long a JSON request may take, the answer included.
const requestTimeout = 30 * time.Second

// responseHeaderTimeout is how long a server may stay silent after it got the
// whole request before the client gives up.
const responseHeaderTimeout = 60 * time.Second

// newHttpClient builds an http.Client restricted to TLS 1.2/1.3, with
// HTTP/2 support over TLS and an opportunistic HTTP/3 upgrade. timeout limits a
// whole exchange, the body included; 0 means no limit.
func newHttpClient(timeout time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	h3Transport := &http3.Transport{TLSClientConfig: tlsConfig}
	h1h2Transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &fallbackTransport{primary: h3Transport, fallback: h1h2Transport},
	}
}

// newTransferClient is the client of file uploads and downloads. http.Client's
// Timeout covers reading the body, so it would cut off any transfer that takes
// longer than that; this client has none and relies on the dial, TLS and
// response header timeouts of its transport instead.
func newTransferClient() *http.Client { return newHttpClient(0) }

// fallbackTransport tries HTTP/3 first for HTTPS and falls back to HTTP/1.1 or
// HTTP/2 when QUIC is unavailable. Only a request that is safe to send twice
// tries HTTP/3 (see triesQUIC), so a failed attempt can never make the server do
// something twice or leave a half-read body behind.
type fallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
	// noQUIC holds the hosts HTTP/3 failed for, which go straight to the
	// fallback from then on (a blocked UDP port would otherwise cost a failed
	// handshake on every request).
	noQUIC sync.Map
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.triesQUIC(req) {
		return t.fallback.RoundTrip(req)
	}
	resp, err := t.primary.RoundTrip(req.Clone(req.Context()))
	if err == nil {
		return resp, nil
	}
	if req.Context().Err() != nil {
		return nil, err // cancelled or timed out by the caller: QUIC is not at fault
	}
	t.noQUIC.Store(req.URL.Host, true)
	retry := req
	if req.Body != nil && req.Body != http.NoBody {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		retry = req.Clone(req.Context()) // never change the caller's request
		retry.Body = body
	}
	return t.fallback.RoundTrip(retry)
}

// triesQUIC reports whether a request goes over HTTP/3 first: it is HTTPS, QUIC
// did not fail for the host before, the method is idempotent (a POST or PATCH
// that failed after QUIC connected may have been done already, and sending it
// again could do it twice), and its body is none or can be made again
// (GetBody). A file or a pipe cannot, so uploads go over TCP at once instead of
// being half read by a failed HTTP/3 attempt.
func (t *fallbackTransport) triesQUIC(req *http.Request) bool {
	if req.URL.Scheme != "https" {
		return false
	}
	if _, failed := t.noQUIC.Load(req.URL.Host); failed {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
	default:
		return false
	}
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}
