package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"futrou-cli/src/logger"
)

// testServer is a programmable HTTP test server for mocking the Futrou API.
type testServer struct {
	*httptest.Server
	routes map[string]http.HandlerFunc
}

func init() {
	openBrowserFunc = func(string) {} // never open a real browser during tests
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{routes: make(map[string]http.HandlerFunc)}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if h, ok := ts.routes[key]; ok {
			h(w, r)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]string{"message": "not found"})
	}))
	t.Cleanup(ts.Server.Close)
	return ts
}

// on registers a handler for "METHOD /path".
func (ts *testServer) on(method, path string, handler http.HandlerFunc) {
	ts.routes[method+" "+path] = handler
}

// respond writes JSON with the given status.
func respond(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		writeJSON(w, body)
	}
}

// respondEmpty writes 204 No Content.
func respondEmpty(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
}

// decodeBody parses JSON request body.
func decodeBody(r *http.Request, v any) {
	json.NewDecoder(r.Body).Decode(v)
}

// runArgs runs the app with test server URL + fake token injected.
// Returns combined logger output and the run error.
func runArgs(t *testing.T, ts *testServer, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"futrou", "--api-url", ts.URL, "--api-key", "test-token"}, args...)
	return captureRun(fullArgs)
}

// runArgsNoAuth runs without injecting credentials (tests "not logged in" path).
func runArgsNoAuth(t *testing.T, ts *testServer, args ...string) (string, error) {
	t.Helper()
	t.Setenv("FUTROU_API_TOKEN", "")
	if os.Getenv("HOME") == "" || strings.HasPrefix(os.Getenv("HOME"), "/home") {
		t.Setenv("HOME", t.TempDir())
	}
	fullArgs := append([]string{"futrou", "--api-url", ts.URL}, args...)
	return captureRun(fullArgs)
}

// captureRun redirects logger output into a buffer, runs the app, returns the captured output.
func captureRun(args []string) (string, error) {
	var buf bytes.Buffer

	// Redirect logger to our buffer
	logger.SetOutput(&buf, &buf)
	defer func() {
		logger.SetOutput(os.Stdout, os.Stderr)
	}()

	// Also capture direct fmt.Print* calls (deploy, init, etc.)
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	app := newApp()
	err := app.Run(args)

	w.Close()
	os.Stdout = origStdout

	var fmtOut bytes.Buffer
	io.Copy(&fmtOut, r)

	combined := buf.String() + fmtOut.String()
	return strings.TrimSpace(combined), err
}

// fixtures

func fixtureServerlet() map[string]any {
	return map[string]any{
		"id":           "sl-123",
		"name":         "my-app",
		"image":        "nginx:latest",
		"state":        "running",
		"instances":    1,
		"minInstances": 1,
		"maxInstances": 3,
		"createdAt":    "2026-01-01T00:00:00Z",
		"updatedAt":    "2026-01-01T00:00:00Z",
	}
}

func fixtureProxy() map[string]any {
	return map[string]any{
		"id":        "px-456",
		"domain":    "example.com",
		"type":      "http",
		"target":    "localhost",
		"port":      8080,
		"status":    "active",
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureDNSZone() map[string]any {
	return map[string]any{
		"id":        "dns-789",
		"name":      "example.com",
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureDNSRecord() map[string]any {
	return map[string]any{
		"id":        "rec-001",
		"dnsId":     "dns-789",
		"name":      "www",
		"type":      "A",
		"value":     "1.2.3.4",
		"ttl":       300,
		"priority":  0,
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureProject() map[string]any {
	return map[string]any{
		"id":          "proj-abc",
		"name":        "my-project",
		"displayName": "My Project",
		"workspaceId": "ws-xyz",
		"createdAt":   "2026-01-01T00:00:00Z",
	}
}

func fixtureVolume() map[string]any {
	return map[string]any{
		"id":        "vol-def",
		"name":      "my-vol",
		"type":      "ssd",
		"sizeGb":    10,
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureAPIError(msg string) map[string]any {
	return map[string]any{"message": msg, "statusCode": 401}
}

// assertions

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q\ngot: %s", substr, s)
	}
}
