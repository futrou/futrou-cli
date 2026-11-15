package commands

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"futrou-cli/src/api"
	"futrou-cli/src/cliconfig"
)

func TestCheckSession(t *testing.T) {
	user := map[string]any{"id": "u1", "email": "ada@example.com"}
	// the API answers a token it does not know with 200 and only server info
	anonymous := map[string]any{"name": "Futrou API", "version": "2.0.184", "clientIp": "1.2.3.4"}
	for name, tc := range map[string]struct {
		status int
		body   any
		want   sessionState
		err    bool
	}{
		"a known user":           {200, map[string]any{"user": user}, sessionValid, false},
		"only an email":          {200, map[string]any{"user": map[string]any{"email": "a@b.c"}}, sessionValid, false},
		"401":                    {401, map[string]any{"message": "Unauthorized"}, sessionRejected, false},
		"403":                    {403, map[string]any{"message": "Forbidden"}, sessionUnverified, true},
		"200 without a user":     {200, anonymous, sessionRejected, false},
		"200 with an empty user": {200, map[string]any{"user": map[string]any{}}, sessionRejected, false},
		"server error":           {500, map[string]any{"message": "boom"}, sessionUnverified, true},
		"bad gateway":            {502, map[string]any{"message": "bad gateway"}, sessionUnverified, true},
	} {
		t.Run(name, func(t *testing.T) {
			ts := newTestServer(t)
			ts.on("GET", "/v2/auth/context", respond(tc.status, tc.body))
			state, context, err := checkSession(ts.URL, "tok")
			if state != tc.want || (err != nil) != tc.err {
				t.Fatalf("state %v, err %v; want %v, error %v", state, err, tc.want, tc.err)
			}
			if (state == sessionValid) != (context != nil) {
				t.Errorf("context = %v for state %v", context, state)
			}
		})
	}
}

func TestCheckSessionWhenTheAPIIsUnreachableIsUnverifiedNotRejected(t *testing.T) {
	ts := newTestServer(t)
	url := ts.URL
	ts.Close()
	if state, _, err := checkSession(url, "tok"); state != sessionUnverified || err == nil {
		t.Errorf("state %v, err %v: a login must not be thrown away because the network is down", state, err)
	}
}

func TestExplainAuthError(t *testing.T) {
	noUser := &api.APIError{Message: "Bad Request", Errors: []api.FieldError{{Message: "No userId provided", Code: "BAD_REQUEST"}}}
	unauthorized := &api.APIError{Message: "Unauthorized"}
	other := &api.APIError{Message: "Not Found"}
	for name, tc := range map[string]struct {
		err    error
		hint   bool
		sameAs error
	}{
		"no user id":         {noUser, true, nil},
		"wrapped no user id": {fmt.Errorf("listing: %w", noUser), true, nil},
		"unauthorized":       {unauthorized, true, nil},
		"another API error":  {other, false, other},
		"a plain error":      {errors.New("disk full"), false, nil},
		"no error":           {nil, false, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := explainAuthError(tc.err)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("got %v", got)
				}
				return
			}
			if hasHint := strings.Contains(got.Error(), "futrou login"); hasHint != tc.hint {
				t.Errorf("hint = %v for %q, want %v", hasHint, got, tc.hint)
			}
			if tc.hint && !errors.Is(got, errSessionInvalid) {
				t.Errorf("%v does not wrap errSessionInvalid", got)
			}
			if tc.hint && !strings.Contains(got.Error(), tc.err.Error()) {
				t.Errorf("the original error is lost: %q", got)
			}
			if !tc.hint && got.Error() != tc.err.Error() {
				t.Errorf("an unrelated error changed: %q", got)
			}
		})
	}
}

// The case that locked a user out: a token is stored, the API does not know it,
// whoami says "not logged in", and `futrou login` used to answer "Already
// logged in" without ever asking the API.
func TestLoginReplacesAStoredLoginTheAPIRejects(t *testing.T) {
	for name, status := range map[string]struct {
		code int
		body any
	}{
		"200 without a user": {200, map[string]any{"name": "Futrou API", "version": "2"}},
		"401":                {401, map[string]any{"message": "Unauthorized"}},
	} {
		t.Run(name, func(t *testing.T) {
			ts := newTestServer(t)
			loggedInConfig(t, ts.URL, "ws-1")
			ts.on("GET", "/v2/auth/context", respond(status.code, status.body))
			discoveries := 0
			ts.on("GET", "/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
				discoveries++
				respond(500, map[string]any{"message": "stop here"})(w, r)
			})
			out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
			if err == nil {
				t.Fatalf("the login flow must have started (and failed at discovery):\n%s", out)
			}
			if strings.Contains(out, "Already logged in") {
				t.Fatalf("a login the API rejects is not a login:\n%s", out)
			}
			assertContains(t, out, "no longer valid")
			if discoveries != 1 {
				t.Errorf("the OAuth flow was started %d times, want 1", discoveries)
			}
		})
	}
}

func TestLoginKeepsALoginTheAPIAccepts(t *testing.T) {
	ts := newTestServer(t) // discovery is not routed: a new login would fail the test
	loggedInConfig(t, ts.URL, "ws-1")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "a@b.c"}}))
	ts.on("GET", "/v2/workspaces/ws-1", respond(200, map[string]any{"id": "ws-1", "name": "acme"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Already logged in")
}

func TestLoginKeepsTheLoginWhenTheAPICannotBeAsked(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-1")
	ts.on("GET", "/v2/auth/context", respond(503, map[string]any{"message": "down"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Already logged in")
	cfg, err := cliconfig.Load()
	if err != nil || cfg.TokenFor(ts.URL) == "" {
		t.Errorf("the stored token must survive an outage: %v", err)
	}
}

func TestLoginDoesNotCheckTheStoredLoginWhenATokenIsGiven(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-1")
	ts.on("GET", "/v2/auth/context", func(w http.ResponseWriter, r *http.Request) {
		t.Error("the stored login must not be checked when --api-token is given")
	})
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--api-token", "given", "login"})
	assertNoError(t, err)
	assertContains(t, out, "Already logged in")
}

func TestWhoamiRejectedLoginTellsToLogInAndThatWorks(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-1")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"name": "Futrou API"}))
	_, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertError(t, err)
	assertContains(t, err.Error(), "futrou login")
	assertContains(t, err.Error(), "no longer valid")
}

func TestLoginShowsCheckingWhileItVerifiesAStoredLogin(t *testing.T) {
	enableLoader(t, true)
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-1")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "a@b.c"}}))
	ts.on("GET", "/v2/workspaces/ws-1", respond(200, map[string]any{"id": "ws-1", "name": "acme"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Checking the stored login...")
	assertContains(t, out, "Already logged in")
}

// Already logged in shows the same as `futrou whoami`: the API, the user (or the
// API key) and the workspace.
func TestLoginWhenLoggedInShowsWhatWhoamiShows(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "fullname": "Ada Lovelace", "email": "ada@example.com"}}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(200, map[string]any{"id": "ws-9", "name": "acme"}))
	login, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	for _, want := range []string{"Already logged in", "API URL: " + ts.URL, "User: Ada Lovelace <ada@example.com>", "Workspace: acme (ws-9)", "futrou logout"} {
		assertContains(t, login, want)
	}
	whoami, err := captureRun([]string{"futrou", "--api-url", ts.URL, "whoami"})
	assertNoError(t, err)
	if !strings.Contains(login, whoami) {
		t.Errorf("login must contain exactly what whoami prints.\nwhoami:\n%s\nlogin:\n%s", whoami, login)
	}
}

func TestLoginWhenLoggedInJSONHasTheWhoamiFields(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "ada@example.com"}}))
	ts.on("GET", "/v2/workspaces/ws-9", respond(200, map[string]any{"id": "ws-9", "name": "acme"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "--log-format", "json", "login"})
	assertNoError(t, err)
	for _, want := range []string{`\"status\": \"already logged in\"`, `\"apiUrl\"`, `\"email\": \"ada@example.com\"`, `\"name\": \"acme\"`} {
		assertContains(t, out, want)
	}
}

// When the API could not be asked there is nothing to show but the plain message.
func TestLoginWhenLoggedInButUnverifiedShowsThePlainMessage(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "ws-9")
	ts.on("GET", "/v2/auth/context", respond(503, map[string]any{"message": "down"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Already logged in to "+ts.URL)
	if strings.Contains(out, "User:") {
		t.Errorf("no user is known:\n%s", out)
	}
}

// A login that has no default workspace (made before defaults existed, or by an
// account whose workspace list is empty) gets one the next time `futrou login`
// runs, so no command asks for --workspace.
func TestLoginWhenLoggedInSetsAMissingDefaultWorkspace(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"workspaceId": "ws-token"},
		"user":     map[string]any{"id": "u1", "email": "a@b.c"},
	}))
	ts.on("GET", "/v2/workspaces", respond(200, []any{})) // the list is empty
	ts.on("GET", "/v2/workspaces/ws-token", respond(200, map[string]any{"id": "ws-token", "name": "tokens"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Workspace: tokens (ws-token)")
	cfg, err := cliconfig.Load()
	if err != nil || cfg.DefaultWorkspaceFor(ts.URL) != "ws-token" {
		t.Errorf("default workspace = %q (%v), want ws-token", cfg.DefaultWorkspaceFor(ts.URL), err)
	}
}

func TestLoginWhenLoggedInKeepsTheDefaultWorkspaceTheUserChose(t *testing.T) {
	ts := newTestServer(t)
	loggedInConfig(t, ts.URL, "chosen")
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "a@b.c"}}))
	ts.on("GET", "/v2/workspaces/chosen", respond(200, map[string]any{"id": "chosen", "name": "mine"}))
	out, err := captureRun([]string{"futrou", "--api-url", ts.URL, "login"})
	assertNoError(t, err)
	assertContains(t, out, "Workspace: mine (chosen)")
}

func TestExplainAuthErrorOnlyForMissingLogins(t *testing.T) {
	for msg, want := range map[string]bool{
		"No userId provided":              true,
		"Unauthorized":                    true,
		"Unauthorized: insufficient role": false,
		"Forbidden":                       false,
	} {
		err := explainAuthError(&api.APIError{Message: msg})
		if got := errors.Is(err, errSessionInvalid); got != want {
			t.Errorf("%q: login advice = %v, want %v", msg, got, want)
		}
	}
}
