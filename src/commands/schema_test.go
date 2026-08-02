package commands

import "testing"

func TestSchema(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/openapi.json", respond(200, map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Futrou API", "version": "2"},
	}))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "schema")
	assertNoError(t, err)
	assertContains(t, out, "Futrou API")
}

func TestSchema_go(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/openapi.json", respond(200, map[string]interface{}{
		"openapi": "3.0.0",
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{},
		},
	}))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "schema", "--format", "go")
	assertNoError(t, err)
	assertContains(t, out, "package api")
	assertContains(t, out, "type APIError struct")
}
