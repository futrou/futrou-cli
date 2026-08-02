package services

import (
	"encoding/json"
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
