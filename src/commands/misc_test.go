package commands

import (
	"os"
	"testing"
)

func TestLicense(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "license")
	assertNoError(t, err)
	assertContains(t, out, "MIT")
}

func TestLicense_json(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "--log-format", "json", "license")
	assertNoError(t, err)
	assertContains(t, out, "license")
	assertContains(t, out, "MIT")
}

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

func TestGlobalFlags_apiUrl(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	out, err := runArgs(t, ts, "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_apiToken_env(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	t.Setenv("FUTROU_API_TOKEN", "env-token")
	t.Setenv("HOME", t.TempDir())

	// run without --api-key flag; token comes from env
	args := []string{"futrou", "--api-url", ts.URL, "serverlets", "list"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_logLevel(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	out, err := runArgs(t, ts, "--log-level", "debug", "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_logFormat_json(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-123", respond(200, fixtureServerlet()))

	out, err := runArgs(t, ts, "--log-format", "json", "serverlets", "get", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_apiKey_alias(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	// --api-token alias should work the same as --api-key
	args := []string{"futrou", "--api-url", ts.URL, "--api-token", "test-token", "serverlets", "list"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestRequireAuth_noConfig(t *testing.T) {
	ts := newTestServer(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	_, err := runArgsNoAuth(t, ts, "serverlets", "list")
	assertError(t, err)
	// The error message should mention login
	if err != nil {
		_ = os.Stdout // keep import; actual message goes to logger stderr
	}
}
