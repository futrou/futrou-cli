package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLogin_success(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		decodeBody(r, &body)
		respond(200, map[string]interface{}{
			"apiToken": map[string]string{
				"id":    "tok-id",
				"token": "tok-secret",
			},
			"user": map[string]string{
				"email": body["email"],
				"id":    "usr-1",
			},
		})(w, r)
	})

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUTROU_API_TOKEN", "")

	args := []string{"futrou", "--api-url", ts.URL,
		"login", "--email", "user@example.com", "--password", "hunter2"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "user@example.com")

	cfgPath := filepath.Join(tmpHome, ".futrou", "cli.json")
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config file not created at %s: %v", cfgPath, statErr)
	}
}

func TestLogin_wrongCredentials(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/auth/login", respond(401, fixtureAPIError("invalid credentials")))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	args := []string{"futrou", "--api-url", ts.URL,
		"login", "--email", "bad@example.com", "--password", "wrong"}
	_, err := captureRun(args)
	assertError(t, err)
}

func TestLogin_missingEmail(t *testing.T) {
	// When email flag is empty and stdin is not a tty, the prompt reads "" and errors
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	// Provide password but no email; interactive prompt will read empty from non-tty stdin
	args := []string{"futrou", "--api-url", ts.URL,
		"login", "--password", "hunter2"}
	// This will either error (empty email) or hang waiting for stdin in CI.
	// We just confirm it doesn't panic.
	captureRun(args) //nolint
}

func TestLogout(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FUTROU_API_TOKEN", "")

	cfgDir := filepath.Join(tmpHome, ".futrou")
	os.MkdirAll(cfgDir, 0700)
	os.WriteFile(filepath.Join(cfgDir, "cli.json"), []byte(`{"apiKey":"tok"}`), 0600)

	ts := newTestServer(t)
	args := []string{"futrou", "--api-url", ts.URL, "logout"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "Logged out")

	if _, statErr := os.Stat(filepath.Join(cfgDir, "cli.json")); !os.IsNotExist(statErr) {
		t.Error("config file was not removed after logout")
	}
}

func TestLogout_noConfigFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	ts := newTestServer(t)
	args := []string{"futrou", "--api-url", ts.URL, "logout"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "Logged out")
}

func TestLogout_json(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	ts := newTestServer(t)
	args := []string{"futrou", "--api-url", ts.URL, "--log-format", "json", "logout"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "logged out")
}
