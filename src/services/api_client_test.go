package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"futrou-cli/src/api"
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
	if got := NormalizeApiUrl("https://example.test/v2/"); got != "https://example.test" {
		t.Errorf("NormalizeApiUrl() = %q", got)
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
