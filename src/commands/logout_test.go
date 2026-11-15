package commands

import (
	"net/http"
	"testing"

	"futrou-cli/src/cliconfig"
)

func TestLogoutRemovesTheStoredLoginOfThatAPIOnly(t *testing.T) {
	ts := newTestServer(t)
	revoked := 0
	ts.on("POST", "/v2/auth/oauth2/revoke", func(w http.ResponseWriter, r *http.Request) { revoked++; w.WriteHeader(201) })
	loggedInConfig(t, ts.URL, "ws-9")
	cfg, err := cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetToken("https://other.example.com", "other-token"); err != nil {
		t.Fatal(err)
	}
	if err := cliconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "logout"})
	assertNoError(t, err)
	assertContains(t, out, "Logged out")
	if revoked != 1 {
		t.Errorf("the token was destroyed at the API %d times, want 1", revoked)
	}

	cfg, err = cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenFor(ts.URL) != "" || cfg.DefaultWorkspaceFor(ts.URL) != "" {
		t.Error("the token and default workspace of the logged out API must be gone")
	}
	if cfg.TokenFor("https://other.example.com") != "other-token" {
		t.Error("logging out of one API must keep the login of another")
	}
}

func TestLogoutWhenNotLoggedInSaysSo(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "logout"})
	assertNoError(t, err)
	assertContains(t, out, "Not logged in")
}

func TestLogoutJSONStatus(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/auth/oauth2/revoke", respond(201, map[string]any{}))
	loggedInConfig(t, ts.URL, "ws-9")
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "logout"})
	assertNoError(t, err)
	assertContains(t, out, `\"status\": \"logged out\"`)
	assertContains(t, out, `\"revoked\": true`)
	out, err = captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "logout"})
	assertNoError(t, err)
	assertContains(t, out, `\"status\": \"not logged in\"`)
}

func TestLogoutStillLogsOutWhenTheTokenCannotBeDestroyed(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/auth/oauth2/revoke", respond(500, map[string]any{"message": "boom"}))
	loggedInConfig(t, ts.URL, "ws-9")
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "logout"})
	assertNoError(t, err)
	assertContains(t, out, `\"revoked\": false`)
	cfg, err := cliconfig.Load()
	if err != nil || cfg.TokenFor(ts.URL) != "" {
		t.Errorf("the token must be removed locally anyway: %v", err)
	}
}
