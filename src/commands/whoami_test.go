package commands

import (
	"strings"
	"testing"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

func loggedInConfig(t *testing.T, apiURL, workspaceID string) {
	t.Helper()
	writeCLIConfig(t, apiURL, workspaceID)
	cfg, err := cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetToken(apiURL, "stored-token")
	if err := cliconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestWhoamiWhenNotLoggedInSaysSoWithoutStartingALogin(t *testing.T) {
	ts := newTestServer(t) // any request would fail the test
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")
	_, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertError(t, err)
	assertContains(t, err.Error(), "not logged in")
}

func TestWhoamiShowsTheUserAndWorkspace(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "fullname": "Ada Lovelace", "email": "ada@example.com"}}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(200, map[string]any{"id": "ws-9", "name": "acme"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "User: Ada Lovelace <ada@example.com>")
	assertContains(t, out, "Workspace: acme (ws-9)")
}

func TestWhoamiWithAnExpiredTokenSaysNotLoggedIn(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(401, map[string]any{"message": "Unauthorized"}))
	_, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertError(t, err)
	assertContains(t, err.Error(), "no longer valid")
}

func TestWhoamiWithoutADefaultWorkspaceUsesTheTokensWorkspace(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"workspaceId": "ws-3"},
		"user":     map[string]any{"id": "u1", "email": "bot@example.com"},
	}))
	ts.on("GET", "/v2/workspaces/ws-3", respond(200, map[string]any{"id": "ws-3", "name": "ci"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--api-token", "t", "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "bot@example.com")
	assertContains(t, out, "Workspace: ci (ws-3)")
}

func TestWhoamiShowsTheNameOfTheAPIKey(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"id": "k1", "name": "ci deploy", "type": "workspace", "workspaceId": "ws-3"},
		"user":     map[string]any{"id": "u1", "email": "bot@example.com"},
	}))
	ts.on("GET", "/v2/workspaces/ws-3", respond(200, map[string]any{"id": "ws-3", "name": "ci"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--api-token", "t", "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "API Key: ci deploy <bot@example.com>")

	out, err = captureRun([]string{"futrou", "--api-url", ts.URL, "--api-token", "t", "--log-format", "json", "whoami"})
	assertNoError(t, err)
	assertContains(t, out, `\"apiKey\"`)
	assertContains(t, out, `\"name\": \"ci deploy\"`)
	assertContains(t, out, `\"type\": \"workspace\"`)
}

func TestWhoamiTheAPIKeyFromTheEnvironmentIsShownToo(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "env-token")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"name": "from env"},
		"user":     map[string]any{"id": "u1", "email": "bot@example.com"},
	}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "API Key: from env <bot@example.com>")
}

func TestWhoamiAnUnnamedAPIKeyShowsTheEmail(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "bot@example.com"}}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--api-token", "t", "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "API Key: bot@example.com\n")
}

// A login made with `futrou login` is not an API key, whatever name the server gives its token.
func TestWhoamiAStoredLoginShowsNoAPIKey(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	t.Setenv("FUTROU_API_TOKEN", "")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"name": "cli login"},
		"user":     map[string]any{"id": "u1", "email": "ada@example.com"},
	}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(200, map[string]any{"id": "ws-9", "name": "acme"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	if strings.Contains(out, "API Key") || strings.Contains(out, "cli login") {
		t.Errorf("a stored login is not an API key:\n%s", out)
	}
	out, err = captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "whoami"})
	assertNoError(t, err)
	if strings.Contains(out, "apiKey") {
		t.Errorf("json shows an API key for a stored login:\n%s", out)
	}
}

func TestWhoamiTextFormats(t *testing.T) {
	user := func(name, email string) authContext {
		var c authContext
		c.User.Fullname, c.User.Email = name, email
		return c
	}
	key := func(keyName, email string) authContext {
		c := user("Ignored Person", email)
		c.APIToken.Name = keyName
		return c
	}
	for name, tc := range map[string]struct {
		context      authContext
		viaKey       bool
		wsName, wsID string
		want         string
	}{
		"user with a name":           {user("Ada Lovelace", "ada@example.com"), false, "acme", "ws-1", "API URL: https://api.example.com\nUser: Ada Lovelace <ada@example.com>\nWorkspace: acme (ws-1)\n"},
		"user without a name":        {user("", "ada@example.com"), false, "acme", "ws-1", "API URL: https://api.example.com\nUser: ada@example.com\nWorkspace: acme (ws-1)\n"},
		"name that is the email":     {user("admin@futrou.com", "admin@futrou.com"), false, "", "ws-1", "API URL: https://api.example.com\nUser: admin@futrou.com\nWorkspace: ws-1\n"},
		"api key with a name":        {key("ci deploy", "bot@example.com"), true, "ci", "ws-3", "API URL: https://api.example.com\nAPI Key: ci deploy <bot@example.com>\nWorkspace: ci (ws-3)\n"},
		"api key without a name":     {key("", "bot@example.com"), true, "ci", "ws-3", "API URL: https://api.example.com\nAPI Key: bot@example.com\nWorkspace: ci (ws-3)\n"},
		"api key never shows person": {key("k", "bot@example.com"), true, "", "ws-3", "API URL: https://api.example.com\nAPI Key: k <bot@example.com>\nWorkspace: ws-3\n"},
		"no workspace":               {user("Ada", "a@b.c"), false, "", "", "API URL: https://api.example.com\nUser: Ada <a@b.c>\nWorkspace: none selected — pass --workspace or run 'futrou login'\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := whoamiText("https://api.example.com", tc.context, tc.viaKey, tc.wsName, tc.wsID); got != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestNameAndEmail(t *testing.T) {
	for _, tc := range [][3]string{
		{"Ada", "a@b.c", "Ada <a@b.c>"}, {"", "a@b.c", "a@b.c"}, {"a@b.c", "a@b.c", "a@b.c"}, {"Ada", "", "Ada"}, {"", "", ""},
	} {
		if got := nameAndEmail(tc[0], tc[1]); got != tc[2] {
			t.Errorf("nameAndEmail(%q, %q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}

func enableLoader(t *testing.T, on bool) {
	t.Helper()
	old := loaderEnabled
	loaderEnabled = func(*cli.Context) bool { return on }
	t.Cleanup(func() { loaderEnabled = old })
}

func whoamiServer(t *testing.T) *testServer {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	t.Setenv("FUTROU_API_TOKEN", "")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "ada@example.com"}}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(200, map[string]any{"id": "ws-9", "name": "acme"}))
	return ts
}

// While whoami waits for the API a terminal shows "Checking...". (Outside a
// terminal the logger prints the message as a line, which is what the test sees.)
func TestWhoamiShowsCheckingWhileItAsksTheAPI(t *testing.T) {
	enableLoader(t, true)
	ts := whoamiServer(t)
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "Checking...")
	if i, j := strings.Index(out, "Checking..."), strings.Index(out, "User:"); i > j {
		t.Errorf("Checking... must come before the answer:\n%s", out)
	}
}

func TestWhoamiShowsNoLoaderOutsideATerminalOrInJSONMode(t *testing.T) {
	ts := whoamiServer(t)
	// the real rule: no terminal in a test, so no loader
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	if strings.Contains(out, "Checking") {
		t.Errorf("piped output must hold only the answer:\n%s", out)
	}
	// even on a terminal, JSON stays JSON
	enableLoader(t, false)
	out, err = captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "whoami"})
	assertNoError(t, err)
	if strings.Contains(out, "Checking") {
		t.Errorf("json output must not hold the loader:\n%s", out)
	}
}

func TestLoaderEnabledNeedsATerminalAndNoJSON(t *testing.T) {
	app := &cli.App{Flags: []cli.Flag{&cli.StringFlag{Name: "log-format"}}}
	for name, tc := range map[string]struct {
		args []string
		want bool
	}{"text": {[]string{"x"}, false}, "json": {[]string{"x", "--log-format", "json"}, false}} {
		app.Action = func(c *cli.Context) error {
			if got := loaderEnabled(c); got != tc.want {
				t.Errorf("%s: loaderEnabled = %v (stdout is not a terminal in tests)", name, got)
			}
			return nil
		}
		if err := app.Run(tc.args); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWhoamiStopsTheLoaderWhenTheLoginIsRejected(t *testing.T) {
	enableLoader(t, true)
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"name": "Futrou API"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertError(t, err)
	assertContains(t, err.Error(), "no longer valid")
	assertContains(t, out, "Checking...")
}

func TestWhoamiTextShowsTheAPIURLOnlyWhenItIsNotTheDefault(t *testing.T) {
	var context authContext
	context.User.Email = "ada@example.com"
	for name, tc := range map[string]struct {
		apiUrl string
		want   bool
	}{
		"the default":                {constants.ApiUrl, false},
		"the default with a /v2":     {constants.ApiUrl + "/v2", false},
		"the default with a slash":   {constants.ApiUrl + "/", false},
		"the default without scheme": {"api.futrou.com", false},
		"another server":             {"https://api.dev.futrou.com", true},
		"a local server":             {"http://localhost:4000", true},
	} {
		t.Run(name, func(t *testing.T) {
			text := whoamiText(tc.apiUrl, context, false, "acme", "ws-1")
			if shown := strings.Contains(text, "API URL:"); shown != tc.want {
				t.Errorf("API URL shown = %v, want %v:\n%s", shown, tc.want, text)
			}
			if !strings.HasPrefix(text, "User: ada@example.com\n") && !tc.want {
				t.Errorf("the user must come first with the default API:\n%s", text)
			}
			if tc.want && !strings.HasPrefix(text, "API URL: "+tc.apiUrl+"\n") {
				t.Errorf("the API URL must come first:\n%s", text)
			}
		})
	}
}

// The case of a token that may not read the workspace (403, empty list): the
// context still names the user's workspaces.
func TestWhoamiNamesTheWorkspaceFromTheContextWhenReadingItIsRefused(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{
		"id": "u1", "email": "ada@example.com",
		"workspaces": []map[string]any{{"id": "ws-other", "name": "other"}, {"id": "ws-9", "name": "futrou"}},
	}}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(403, map[string]any{"message": "Forbidden"}))
	ts.on("GET", "/v2/workspaces", respond(200, []any{}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	assertContains(t, out, "Workspace: futrou (ws-9)")
}

func TestLookupWorkspaceName(t *testing.T) {
	for name, tc := range map[string]struct {
		direct     int
		directName string
		list       []map[string]any
		want       string
	}{
		"read directly":            {200, "acme", nil, "acme"},
		"read refused, listed":     {403, "", []map[string]any{{"id": "ws-1", "name": "from list"}}, "from list"},
		"read refused, not listed": {403, "", []map[string]any{{"id": "ws-2", "name": "other"}}, ""},
		"read empty, listed":       {200, "", []map[string]any{{"id": "ws-1", "name": "from list"}}, "from list"},
		"nothing":                  {404, "", []map[string]any{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			ts := newTestServer(t)
			ts.on("GET", "/v2/workspaces/ws-1", respond(tc.direct, map[string]any{"id": "ws-1", "name": tc.directName}))
			ts.on("GET", "/v2/workspaces", respond(200, tc.list))
			if got := lookupWorkspaceName(ts.URL, "tok", "ws-1"); got != tc.want {
				t.Errorf("name = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthContextWorkspaceName(t *testing.T) {
	var context authContext
	context.User.Workspaces = []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{{ID: "a", Name: "alpha"}, {ID: "b", Name: "beta"}}
	for id, want := range map[string]string{"a": "alpha", "b": "beta", "c": "", "": ""} {
		if got := context.workspaceName(id); got != want {
			t.Errorf("workspaceName(%q) = %q, want %q", id, got, want)
		}
	}
}
