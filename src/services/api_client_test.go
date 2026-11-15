package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"futrou-cli/src/api"
	"futrou-cli/src/cliconfig"
	"futrou-cli/src/utils"
)

func TestApiClientRequestInto_AllHTTPMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("method = %q, want %q", r.Method, method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Errorf("Authorization = %q", got)
				}
				if got := r.Header.Get("Accept"); got != "application/json" {
					t.Errorf("Accept = %q", got)
				}

				if method == http.MethodGet {
					if got := r.Header.Get("Content-Type"); got != "" {
						t.Errorf("GET Content-Type = %q, want empty", got)
					}
				} else {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("Content-Type = %q", got)
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatalf("decode request body: %v", err)
					}
					if body["name"] != "test" {
						t.Errorf("body = %#v", body)
					}
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"result-1"}`))
			}))
			defer server.Close()

			client := NewApiClientWithToken(server.URL, "test-token")
			var result struct {
				ID string `json:"id"`
			}
			body := interface{}(nil)
			if method != http.MethodGet {
				body = map[string]string{"name": "test"}
			}
			status, err := client.RequestInto(method, "/resource", body, &result)
			if err != nil {
				t.Fatalf("RequestInto() error = %v", err)
			}
			if status != http.StatusOK || result.ID != "result-1" {
				t.Errorf("status/result = %d/%#v", status, result)
			}
		})
	}
}

func TestApiClientRequest_ResponseKinds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Futrou"}`))
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ready"))
		case "/api-error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"invalid request","errors":[{"field":"name","message":"required"}]}`))
		case "/empty-error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/bad-json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not json"))
		}
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")

	result, status, err := client.Request(http.MethodGet, "/json", nil)
	if err != nil || status != http.StatusOK || result.(map[string]interface{})["name"] != "Futrou" {
		t.Errorf("JSON result/status/error = %#v/%d/%v", result, status, err)
	}

	result, status, err = client.Request(http.MethodGet, "/text", nil)
	if err != nil || status != http.StatusOK || result != "ready" {
		t.Errorf("text result/status/error = %#v/%d/%v", result, status, err)
	}

	_, status, err = client.Request(http.MethodGet, "/api-error", nil)
	apiErr, ok := err.(*api.APIError)
	if !ok || status != http.StatusBadRequest || apiErr.Message != "invalid request" {
		t.Errorf("API error/status = %#v/%d", err, status)
	}

	_, status, err = client.Request(http.MethodGet, "/empty-error", nil)
	if _, ok := err.(*api.APIError); !ok || status != http.StatusInternalServerError {
		t.Errorf("empty error/status = %#v/%d", err, status)
	}

	_, status, err = client.Request(http.MethodGet, "/bad-json", nil)
	if err == nil || status != http.StatusOK || !strings.Contains(err.Error(), "parsing response") {
		t.Errorf("bad JSON error/status = %v/%d", err, status)
	}
}

func TestApiClientRequestInto_ErrorAndMutationCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"not authorized"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	callbacks := 0
	client.SetAfterMutation(func() error {
		callbacks++
		return nil
	})

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if _, err := client.RequestInto(method, "/empty", nil, nil); err != nil {
			t.Fatalf("RequestInto(%s) error = %v", method, err)
		}
	}
	if callbacks != 4 {
		t.Errorf("mutation callbacks = %d, want 4", callbacks)
	}

	_, err := client.RequestInto(http.MethodGet, "/error", nil, nil)
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Message != "not authorized" {
		t.Errorf("RequestInto error = %#v", err)
	}
}

// TestApiClientRequestInto_PlainTextErrorIncludesRequestContext guards
// against a regression where a non-JSON error body (e.g. a bare "Bad
// Request" from an upstream proxy) surfaced with no indication of which
// request failed, making --debug output useless for diagnosing it.
func TestApiClientRequestInto_PlainTextErrorIncludesRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	status, err := client.RequestInto(http.MethodGet, "/v2/storages", nil, nil)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("error = %#v, want *api.APIError", err)
	}
	if !strings.Contains(apiErr.Message, "Bad Request") {
		t.Errorf("message = %q, want it to contain the raw response body", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, "400") || !strings.Contains(apiErr.Message, "/v2/storages") {
		t.Errorf("message = %q, want it to contain the status and path for debugging", apiErr.Message)
	}
}

func TestApiClientRequestInto_EmptyErrorBodyIncludesRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	_, err := client.RequestInto(http.MethodDelete, "/v2/storages/s1", nil, nil)
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("error = %#v, want *api.APIError", err)
	}
	if !strings.Contains(apiErr.Message, "500") || !strings.Contains(apiErr.Message, "/v2/storages/s1") {
		t.Errorf("message = %q, want it to contain the status and path", apiErr.Message)
	}
}

func TestApiClientToJSONSchemaAndNormalizeURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/openapi.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openapi":"3.0.0"}`))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL+"/v2/", "")
	schema, err := client.ToJSONSchema()
	if err != nil || string(schema) != `{"openapi":"3.0.0"}` {
		t.Errorf("ToJSONSchema() = %q, %v", schema, err)
	}
	if got := utils.NormalizeApiUrl("https://example.test/v2/"); got != "https://example.test" {
		t.Errorf("utils.NormalizeApiUrl() = %q", got)
	}
}

func TestApiClientUploadFile_Success(t *testing.T) {
	var gotContentType, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"file-1"}`))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "test-token")
	result, status, err := client.UploadFile("/v2/storages/s1/files/a.txt", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if gotContentType != "text/plain" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody != "hello" {
		t.Errorf("uploaded body = %q", gotBody)
	}
	m, ok := result.(map[string]interface{})
	if !ok || m["id"] != "file-1" {
		t.Errorf("result = %#v", result)
	}
}

func TestApiClientUploadFile_DefaultContentType(t *testing.T) {
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	result, status, err := client.UploadFile("/v2/storages/s1/files/a.bin", "", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
	if result != nil {
		t.Errorf("expected nil result for empty body, got %#v", result)
	}
	if gotContentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want default octet-stream", gotContentType)
	}
}

func TestApiClientUploadFile_NonJSONSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text ok"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	result, status, err := client.UploadFile("/v2/storages/s1/files/a.txt", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if result != "plain text ok" {
		t.Errorf("result = %#v, want raw string fallback", result)
	}
}

func TestApiClientUploadFile_JSONErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"quota exceeded"}`))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	_, status, err := client.UploadFile("/v2/storages/s1/files/a.txt", "text/plain", strings.NewReader("x"))
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", status)
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Message != "quota exceeded" {
		t.Errorf("error = %#v", err)
	}
}

func TestApiClientUploadFile_NonJSONErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	_, status, err := client.UploadFile("/v2/storages/s1/files/a.txt", "text/plain", strings.NewReader("x"))
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || !strings.Contains(apiErr.Message, "500") {
		t.Errorf("error = %#v", err)
	}
}

func TestApiClientDownloadFile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("binary-data"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	data, contentType, status, err := client.DownloadFile("/v2/storages/s1/files/a.bin")
	if err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if string(data) != "binary-data" {
		t.Errorf("data = %q", data)
	}
	if contentType != "application/octet-stream" {
		t.Errorf("Content-Type = %q", contentType)
	}
}

func TestApiClientDownloadFile_JSONErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"file not found"}`))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	_, _, status, err := client.DownloadFile("/v2/storages/s1/files/missing.bin")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Message != "file not found" {
		t.Errorf("error = %#v", err)
	}
}

func TestApiClientDownloadFile_NonJSONErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("gateway error"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "")
	_, _, status, err := client.DownloadFile("/v2/storages/s1/files/a.bin")
	if status != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", status)
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || !strings.Contains(apiErr.Message, "502") {
		t.Errorf("error = %#v", err)
	}
}

func TestNewApiClientResolvesURLAndToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")
	t.Setenv("FUTROU_API_URL", "")

	client, err := NewApiClient("localhost:4000", "explicit")
	if err != nil {
		t.Fatal(err)
	}
	if client.ApiToken() != "explicit" || client.ApiUrl() != utils.NormalizeApiUrl("localhost:4000") {
		t.Fatalf("client = %q %q", client.ApiToken(), client.ApiUrl())
	}

	// the environment overrides apply on top of an existing config file
	stored := cliconfig.CliConfig{ApiUrl: utils.NormalizeApiUrl("localhost:4000")}
	if err := cliconfig.Save(&stored); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FUTROU_API_TOKEN", "from-env")
	client, err = NewApiClient("", "")
	if err != nil {
		t.Fatal(err)
	}
	if client.ApiToken() != "from-env" {
		t.Fatalf("the environment token is used when no flag is given: %q", client.ApiToken())
	}
	if client, _ = NewApiClient("", "flag"); client.ApiToken() != "flag" {
		t.Fatalf("an explicit token wins over the environment: %q", client.ApiToken())
	}
}

type stubTransport struct {
	err   error
	calls int
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
}

func TestFallbackTransportFallsBackWhenHTTP3Fails(t *testing.T) {
	https, _ := http.NewRequest("GET", "https://api.example.com/v2", nil)
	plain, _ := http.NewRequest("GET", "http://localhost/v2", nil)

	primary, fallback := &stubTransport{err: fmt.Errorf("quic unavailable")}, &stubTransport{}
	tr := &fallbackTransport{primary: primary, fallback: fallback}
	if resp, err := tr.RoundTrip(https); err != nil || resp.StatusCode != 200 || primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("a failing HTTP/3 attempt falls back: %v, primary %d, fallback %d", err, primary.calls, fallback.calls)
	}
	primary, fallback = &stubTransport{}, &stubTransport{}
	tr = &fallbackTransport{primary: primary, fallback: fallback}
	if _, err := tr.RoundTrip(https); err != nil || fallback.calls != 0 {
		t.Fatalf("a working HTTP/3 attempt needs no fallback: %v, %d", err, fallback.calls)
	}
	primary, fallback = &stubTransport{}, &stubTransport{}
	tr = &fallbackTransport{primary: primary, fallback: fallback}
	if _, err := tr.RoundTrip(plain); err != nil || primary.calls != 0 || fallback.calls != 1 {
		t.Fatalf("plain HTTP never tries QUIC: %v, primary %d, fallback %d", err, primary.calls, fallback.calls)
	}
	failing := &fallbackTransport{primary: &stubTransport{err: fmt.Errorf("a")}, fallback: &stubTransport{err: fmt.Errorf("b")}}
	if _, err := failing.RoundTrip(https); err == nil || err.Error() != "b" {
		t.Fatalf("the fallback's error is reported: %v", err)
	}
}

// A slow transfer must not be cut off by the limit of a JSON request: the
// Timeout of an http.Client covers reading the body too.
func TestTransfersAreNotLimitedByTheRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond) // the body arrives long after the limit below
		_, _ = w.Write([]byte("late body"))
	}))
	defer server.Close()

	client := NewApiClientWithToken(server.URL, "t")
	client.client.Timeout = 50 * time.Millisecond

	if _, _, err := client.Request("GET", "/x", nil); err == nil {
		t.Fatal("a JSON request is still limited by the request timeout")
	}
	var out strings.Builder
	if status, err := client.Stream("/x", &out); err != nil || status != 200 || out.String() != "late body" {
		t.Fatalf("Stream = %d %q %v, want the whole slow body", status, out.String(), err)
	}
	if data, _, status, err := client.DownloadFile("/x"); err != nil || status != 200 || string(data) != "late body" {
		t.Fatalf("DownloadFile = %d %q %v, want the whole slow body", status, data, err)
	}
	if body, status, err := client.SendBody("PUT", "/x", "", strings.NewReader("data")); err != nil || status != 200 || body != "late body" {
		t.Fatalf("SendBody = %v %d %v, want the whole slow answer", body, status, err)
	}
}

func TestClientTimeouts(t *testing.T) {
	for name, client := range map[string]*ApiClient{
		"with token": NewApiClientWithToken("https://api.example.com", "t"),
	} {
		if client.client.Timeout != requestTimeout {
			t.Errorf("%s: JSON timeout = %v, want %v", name, client.client.Timeout, requestTimeout)
		}
		if client.transfer.Timeout != 0 {
			t.Errorf("%s: transfer timeout = %v, want none", name, client.transfer.Timeout)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	loaded, err := NewApiClient("https://api.example.com", "t")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.client.Timeout != requestTimeout || loaded.transfer == nil || loaded.transfer.Timeout != 0 {
		t.Errorf("NewApiClient timeouts = %v / %v", loaded.client.Timeout, loaded.transfer)
	}
	// a server that never answers is still given up on
	transport := newTransferClient().Transport.(*fallbackTransport).fallback.(*http.Transport)
	if transport.ResponseHeaderTimeout <= 0 || transport.TLSHandshakeTimeout <= 0 || transport.DialContext == nil {
		t.Errorf("the transfer transport has no dial, TLS or response header timeout: %+v", transport)
	}
}

// bodyTransport records the body of every request it gets.
type bodyTransport struct {
	err    error
	calls  int
	bodies []string
}

func (b *bodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	b.calls++
	body := ""
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		body = string(data)
	}
	b.bodies = append(b.bodies, body)
	if b.err != nil {
		return nil, b.err
	}
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
}

func TestFallbackTransportNeverRepeatsAnUnsafeRequest(t *testing.T) {
	cases := map[string]struct {
		method string
		body   io.Reader
	}{
		"POST with a body":  {"POST", strings.NewReader("{}")},
		"PATCH with a body": {"PATCH", strings.NewReader("{}")},
		"POST without body": {"POST", nil},
		"PUT of a file":     {"PUT", io.NopCloser(strings.NewReader("file"))}, // no GetBody: cannot be replayed
		"POST of a pipe":    {"POST", io.NopCloser(strings.NewReader("pipe"))},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, "https://api.example.com/v2/x", tc.body)
			primary, fallback := &bodyTransport{err: fmt.Errorf("quic")}, &bodyTransport{}
			tr := &fallbackTransport{primary: primary, fallback: fallback}
			if _, err := tr.RoundTrip(req); err != nil {
				t.Fatal(err)
			}
			if primary.calls != 0 || fallback.calls != 1 {
				t.Errorf("HTTP/3 was tried (%d) or the fallback not used once (%d)", primary.calls, fallback.calls)
			}
		})
	}
}

func TestFallbackTransportGivesTheFallbackAFreshBody(t *testing.T) {
	req, _ := http.NewRequest("PUT", "https://api.example.com/v2/x", strings.NewReader("whole body"))
	primary, fallback := &bodyTransport{err: fmt.Errorf("quic")}, &bodyTransport{}
	tr := &fallbackTransport{primary: primary, fallback: fallback}
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls: primary %d, fallback %d", primary.calls, fallback.calls)
	}
	if primary.bodies[0] != "whole body" || fallback.bodies[0] != "whole body" {
		t.Errorf("bodies: primary %q, fallback %q; the fallback must get the whole body again", primary.bodies[0], fallback.bodies[0])
	}
}

func TestFallbackTransportRemembersAHostWithoutQUIC(t *testing.T) {
	primary, fallback := &bodyTransport{err: fmt.Errorf("quic")}, &bodyTransport{}
	tr := &fallbackTransport{primary: primary, fallback: fallback}
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "https://api.example.com/v2/x", nil)
		if _, err := tr.RoundTrip(req); err != nil {
			t.Fatal(err)
		}
	}
	if primary.calls != 1 || fallback.calls != 3 {
		t.Errorf("primary %d (want 1), fallback %d (want 3): a host QUIC failed for goes straight to TCP", primary.calls, fallback.calls)
	}
	other, _ := http.NewRequest("GET", "https://other.example.com/v2/x", nil)
	if _, err := tr.RoundTrip(other); err != nil || primary.calls != 2 {
		t.Errorf("another host still tries QUIC: %v, primary %d", err, primary.calls)
	}
}

// A response cut off in the middle is an error, never an answer.
func TestCutOffResponsesAreErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // drops the connection after 6 of 100 bytes
	}))
	defer server.Close()

	mutations := 0
	client := NewApiClientWithToken(server.URL, "t")
	client.SetAfterMutation(func() error { mutations++; return nil })

	if _, status, err := client.Request("POST", "/x", map[string]string{}); err == nil || !strings.Contains(err.Error(), "reading response from POST /x") || status != 200 {
		t.Errorf("Request: status %d, err %v", status, err)
	}
	var out map[string]interface{}
	if status, err := client.RequestInto("POST", "/x", map[string]string{}, &out); err == nil || !strings.Contains(err.Error(), "reading response from POST /x") || status != 200 {
		t.Errorf("RequestInto: status %d, err %v", status, err)
	}
	if _, status, err := client.SendBody("PUT", "/x", "", strings.NewReader("data")); err == nil || !strings.Contains(err.Error(), "reading response from PUT /x") || status != 200 {
		t.Errorf("SendBody: status %d, err %v", status, err)
	}
	if mutations != 0 {
		t.Errorf("the after-mutation callback ran %d times for answers that never arrived", mutations)
	}
}

func TestFallbackTransportKeepsQUICAfterACancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/v2", nil)
	primary, fallback := &stubTransport{err: context.Canceled}, &stubTransport{}
	tr := &fallbackTransport{primary: primary, fallback: fallback}
	if _, err := tr.RoundTrip(req); err == nil || fallback.calls != 0 {
		t.Fatalf("a cancelled request is not retried: %v, fallback %d", err, fallback.calls)
	}
	if _, failed := tr.noQUIC.Load("api.example.com"); failed {
		t.Error("a cancelled request must not disable QUIC for the host")
	}
}

func TestFallbackTransportDoesNotChangeTheCallersRequest(t *testing.T) {
	req, _ := http.NewRequest("PUT", "https://api.example.com/v2/x", strings.NewReader("body"))
	original := req.Body
	tr := &fallbackTransport{primary: &bodyTransport{err: fmt.Errorf("quic")}, fallback: &bodyTransport{}}
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.Body != original {
		t.Error("RoundTrip replaced req.Body")
	}
}
