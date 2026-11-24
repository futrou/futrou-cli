package cliconfig

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestErrCIRequiresToken_message(t *testing.T) {
	if !errors.Is(ErrCIRequiresToken, ErrCIRequiresToken) {
		t.Fatal("ErrCIRequiresToken should be comparable via errors.Is")
	}
	got := ErrCIRequiresToken.Error()
	want := "not logged in — CI environment requires a non-empty --api-token (or FUTROU_API_TOKEN)"
	if got != want {
		t.Fatalf("ErrCIRequiresToken.Error() = %q, want %q", got, want)
	}
}

func TestPKCE_challengeMatchesVerifier(t *testing.T) {
	verifier, challenge, err := pkce()
	if err != nil {
		t.Fatalf("pkce() error: %v", err)
	}
	if verifier == "" || challenge == "" {
		t.Fatal("pkce() returned empty verifier or challenge")
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Fatalf("challenge = %q, want %q (sha256 of verifier)", challenge, want)
	}
}

func TestBuildShortAuthURL(t *testing.T) {
	got := buildShortAuthURL("https://api.futrou.com", "chal lenge", 12345)
	// buildShortAuthURL uses url.QueryEscape (exact port from
	// src/commands/login.go), which encodes spaces as "+", not "%20".
	want := "https://api.futrou.com/v2/auth/cli/chal+lenge/12345"
	if got != want {
		t.Fatalf("buildShortAuthURL() = %q, want %q", got, want)
	}
}

func TestBuildAuthURL(t *testing.T) {
	got := buildAuthURL("https://selfhosted.example.com/v2/auth/oauth2/authorize", "client-1", "http://localhost:12345/", "challenge-abc")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("buildAuthURL() produced invalid URL: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "client-1" {
		t.Errorf("client_id = %q, want %q", q.Get("client_id"), "client-1")
	}
	if q.Get("redirect_uri") != "http://localhost:12345/" {
		t.Errorf("redirect_uri = %q, want %q", q.Get("redirect_uri"), "http://localhost:12345/")
	}
	if q.Get("code_challenge") != "challenge-abc" {
		t.Errorf("code_challenge = %q, want %q", q.Get("code_challenge"), "challenge-abc")
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want %q", q.Get("response_type"), "code")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want %q", q.Get("code_challenge_method"), "S256")
	}
}

// shortLink serves a short login link that redirects to an authorize request with these parameters.
func shortLink(t *testing.T, params url.Values) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://api.futrou.com/v2/auth/oauth2/authorize?"+params.Encode(), http.StatusFound)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestShortAuthClientID_returnsTheClientTheServerNames(t *testing.T) {
	ts := shortLink(t, url.Values{"client_id": {"server-cli-client"}, "redirect_uri": {"http://localhost:12345/"}, "code_challenge": {"challenge-abc"}, "response_type": {"code"}})
	if got := shortAuthClientID(ts.URL, "http://localhost:12345/", "challenge-abc"); got != "server-cli-client" {
		t.Errorf("client_id = %q, want the one the link names", got)
	}
}

func TestShortAuthClientID_redirectURIMismatch(t *testing.T) {
	ts := shortLink(t, url.Values{"client_id": {"c"}, "redirect_uri": {"http://localhost:99999/"}, "code_challenge": {"challenge-abc"}})
	if got := shortAuthClientID(ts.URL, "http://localhost:12345/", "challenge-abc"); got != "" {
		t.Errorf("a link for another redirect_uri must not be used: %q", got)
	}
}

func TestShortAuthClientID_codeChallengeMismatch(t *testing.T) {
	ts := shortLink(t, url.Values{"client_id": {"c"}, "redirect_uri": {"http://localhost:12345/"}, "code_challenge": {"different"}})
	if got := shortAuthClientID(ts.URL, "http://localhost:12345/", "challenge-abc"); got != "" {
		t.Errorf("a link for another challenge must not be used: %q", got)
	}
}

// A server without a CLI client configured redirects with an empty client_id.
func TestShortAuthClientID_emptyClientID(t *testing.T) {
	ts := shortLink(t, url.Values{"client_id": {""}, "redirect_uri": {"http://localhost:12345/"}, "code_challenge": {"challenge-abc"}})
	if got := shortAuthClientID(ts.URL, "http://localhost:12345/", "challenge-abc"); got != "" {
		t.Errorf("an empty client_id is no client: %q", got)
	}
}

func TestShortAuthClientID_notFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer ts.Close()
	if got := shortAuthClientID(ts.URL, "http://localhost:12345/", "challenge-abc"); got != "" {
		t.Errorf("a non-redirect response is no link: %q", got)
	}
}

func TestShortAuthClientID_unreachable(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	if got := shortAuthClientID(url, "http://localhost:12345/", "challenge-abc"); got != "" {
		t.Errorf("an unreachable server has no link: %q", got)
	}
}

func TestExchangeCode_success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "authorization_code" {
			t.Errorf("grant_type = %q", body["grant_type"])
		}
		if body["code"] != "auth-code-123" {
			t.Errorf("code = %q", body["code"])
		}
		if body["code_verifier"] != "verifier-xyz" {
			t.Errorf("code_verifier = %q", body["code_verifier"])
		}
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "the-access-token",
			"email":        "alice@example.com",
		})
	}))
	defer ts.Close()

	token, email, err := exchangeCode(ts.URL, "client-id", "auth-code-123", "verifier-xyz", "http://localhost/callback")
	if err != nil {
		t.Fatalf("exchangeCode() error: %v", err)
	}
	if token != "the-access-token" {
		t.Errorf("token = %q, want %q", token, "the-access-token")
	}
	if email != "alice@example.com" {
		t.Errorf("email = %q, want %q", email, "alice@example.com")
	}
}

func TestExchangeCode_missingToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer ts.Close()

	_, _, err := exchangeCode(ts.URL, "client-id", "bad-code", "verifier", "http://localhost/callback")
	if err == nil {
		t.Error("expected error when access_token missing, got nil")
	}
}

func TestFetchOAuthDiscovery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://x/authorize",
			"token_endpoint":         "https://x/token",
			"registration_endpoint":  "https://x/register",
		})
	}))
	defer ts.Close()

	d, err := fetchOAuthDiscovery(ts.URL)
	if err != nil {
		t.Fatalf("fetchOAuthDiscovery() error: %v", err)
	}
	if d.AuthorizationEndpoint != "https://x/authorize" || d.TokenEndpoint != "https://x/token" || d.RegistrationEndpoint != "https://x/register" {
		t.Fatalf("unexpected discovery result: %#v", d)
	}
}

func TestRegisterClient_sendsLogoUri(t *testing.T) {
	var capturedBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		json.NewEncoder(w).Encode(map[string]string{"client_id": "cid-123"})
	}))
	defer ts.Close()

	clientID, err := registerClient(ts.URL)
	if err != nil {
		t.Fatalf("registerClient() error: %v", err)
	}
	if clientID != "cid-123" {
		t.Fatalf("clientID = %q, want %q", clientID, "cid-123")
	}
	if capturedBody["logo_uri"] == "" || capturedBody["logo_uri"] == nil {
		t.Fatal("expected logo_uri in registration payload")
	}
}

func TestLogin_alreadyLoggedIn(t *testing.T) {
	tempHome(t)
	cfg := &CliConfig{}
	cfg.SetToken("https://api.example.com", "existing-token")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	email, already, err := cfg2.Login("https://api.example.com", "", nil, func(string) {}, func(string) {}, time.Second)
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}
	if !already {
		t.Fatal("expected alreadyLoggedIn=true")
	}
	if email != "" {
		t.Fatalf("expected empty email for already-logged-in, got %q", email)
	}
}

func TestLogin_ciReturnsErrCIRequiresToken(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	browserOpened := false
	_, _, err = cfg.Login("https://api.example.com", "", nil, func(string) { browserOpened = true }, func(string) {}, time.Second)
	if !errors.Is(err, ErrCIRequiresToken) {
		t.Fatalf("Login() error = %v, want ErrCIRequiresToken", err)
	}
	if browserOpened {
		t.Fatal("Login() must not open a browser under CI")
	}
}

func TestLogout_removesTokenAndReportsPriorState(t *testing.T) {
	tempHome(t)
	var revoked []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		revoked = append(revoked, r.Method+" "+r.URL.Path+" "+body["token"]+" "+r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()
	cfg := &CliConfig{}
	cfg.SetToken(ts.URL, "tok")
	cfg.SetDefaultWorkspace(ts.URL, "ws-1")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	wasLoggedIn, revokeErr, err := cfg2.Logout(ts.URL)
	if err != nil || revokeErr != nil {
		t.Fatalf("Logout() error: %v, revoke error: %v", err, revokeErr)
	}
	if len(revoked) != 1 || revoked[0] != "POST /v2/auth/oauth2/revoke tok Bearer tok" {
		t.Fatalf("the token must be destroyed at the API first: %v", revoked)
	}
	if !wasLoggedIn {
		t.Fatal("expected wasLoggedIn=true")
	}
	if cfg2.TokenFor(ts.URL) != "" {
		t.Fatal("expected token removed after Logout()")
	}

	cfg3, _ := Load()
	wasLoggedIn2, _, err := cfg3.Logout(ts.URL)
	if err != nil {
		t.Fatalf("Logout() second call error: %v", err)
	}
	if wasLoggedIn2 {
		t.Fatal("expected wasLoggedIn=false on second logout")
	}
}

func TestEnsureLoggedIn_usesTokenOverrideWithoutPrompting(t *testing.T) {
	tempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	browserOpened := false
	token, err := cfg.EnsureLoggedIn("https://api.example.com", "override-token", nil, func(string) { browserOpened = true }, func(string) {}, time.Second)
	if err != nil {
		t.Fatalf("EnsureLoggedIn() error: %v", err)
	}
	if token != "override-token" {
		t.Fatalf("token = %q, want %q", token, "override-token")
	}
	if browserOpened {
		t.Fatal("EnsureLoggedIn() must not start a login flow when a token override is given")
	}
}

func TestEnsureLoggedIn_usesStoredTokenWithoutPrompting(t *testing.T) {
	tempHome(t)
	cfg := &CliConfig{}
	cfg.SetToken("https://api.example.com", "stored-token")
	Save(cfg)

	cfg2, _ := Load()
	browserOpened := false
	token, err := cfg2.EnsureLoggedIn("https://api.example.com", "", nil, func(string) { browserOpened = true }, func(string) {}, time.Second)
	if err != nil {
		t.Fatalf("EnsureLoggedIn() error: %v", err)
	}
	if token != "stored-token" {
		t.Fatalf("token = %q, want %q", token, "stored-token")
	}
	if browserOpened {
		t.Fatal("EnsureLoggedIn() must not start a login flow when a token is already stored")
	}
}

func TestEnsureLoggedIn_ciNoTokenReturnsErrCIRequiresToken(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	_, err = cfg.EnsureLoggedIn("https://api.example.com", "", nil, func(string) {}, func(string) {}, time.Second)
	if !errors.Is(err, ErrCIRequiresToken) {
		t.Fatalf("EnsureLoggedIn() error = %v, want ErrCIRequiresToken", err)
	}
}

func TestResolveOrPromptWorkspace_uuidMatchesExistingByID(t *testing.T) {
	workspaces := []Workspace{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "prod"},
		{ID: "22222222-2222-2222-2222-222222222222", Name: "staging"},
	}
	id, name, err := resolveOrPromptWorkspace(workspaces, "11111111-1111-1111-1111-111111111111", nil)
	if err != nil {
		t.Fatalf("resolveOrPromptWorkspace() error: %v", err)
	}
	if id != "11111111-1111-1111-1111-111111111111" || name != "prod" {
		t.Fatalf("got id=%q name=%q, want id=%q name=%q", id, name, "11111111-1111-1111-1111-111111111111", "prod")
	}
}

func TestResolveOrPromptWorkspace_uuidNoMatchPassesThrough(t *testing.T) {
	workspaces := []Workspace{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "prod"},
	}
	unlisted := "99999999-9999-9999-9999-999999999999"
	id, name, err := resolveOrPromptWorkspace(workspaces, unlisted, nil)
	if err != nil {
		t.Fatalf("resolveOrPromptWorkspace() error: %v", err)
	}
	if id != unlisted || name != unlisted {
		t.Fatalf("got id=%q name=%q, want passthrough id=%q name=%q", id, name, unlisted, unlisted)
	}
}

func TestResolveOrPromptWorkspace_nameMatchesExisting(t *testing.T) {
	workspaces := []Workspace{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "prod"},
		{ID: "22222222-2222-2222-2222-222222222222", Name: "staging"},
	}
	id, name, err := resolveOrPromptWorkspace(workspaces, "staging", nil)
	if err != nil {
		t.Fatalf("resolveOrPromptWorkspace() error: %v", err)
	}
	if id != "22222222-2222-2222-2222-222222222222" || name != "staging" {
		t.Fatalf("got id=%q name=%q, want id=%q name=%q", id, name, "22222222-2222-2222-2222-222222222222", "staging")
	}
}

func TestResolveOrPromptWorkspace_nameNoMatchReturnsError(t *testing.T) {
	workspaces := []Workspace{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "prod"},
	}
	_, _, err := resolveOrPromptWorkspace(workspaces, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unmatched non-UUID workspace flag")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error %q should mention the unmatched value", err.Error())
	}
}

// A server that accepts the connection and never answers must not hang the login.
func TestLoginRequestsGiveUpOnAServerThatNeverAnswers(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	old := oauthTimeout
	oauthTimeout = 100 * time.Millisecond
	defer func() { oauthTimeout = old }()

	start := time.Now()
	if _, err := fetchOAuthDiscovery(server.URL); err == nil {
		t.Error("discovery: no error from a server that never answers")
	}
	if _, err := registerClient(server.URL); err == nil {
		t.Error("registration: no error from a server that never answers")
	}
	if _, _, err := exchangeCode(server.URL, "id", "code", "verifier", "http://localhost/"); err == nil {
		t.Error("token exchange: no error from a server that never answers")
	}
	if _, err := listWorkspaces(server.URL, "t"); err == nil {
		t.Error("workspace list: no error from a server that never answers")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v: the requests were not limited", elapsed)
	}
}

func TestOldestFirst(t *testing.T) {
	at := func(year int) time.Time { return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) }
	input := []Workspace{{ID: "no-date-1"}, {ID: "c", CreatedAt: at(2026)}, {ID: "a", CreatedAt: at(2022)}, {ID: "no-date-2"}, {ID: "b", CreatedAt: at(2024)}}
	got := OldestFirst(input)
	var ids []string
	for _, w := range got {
		ids = append(ids, w.ID)
	}
	if want := "a,b,c,no-date-1,no-date-2"; strings.Join(ids, ",") != want {
		t.Errorf("order = %v, want %s", ids, want)
	}
	if input[0].ID != "no-date-1" {
		t.Error("the input must not be reordered")
	}
	if len(OldestFirst(nil)) != 0 {
		t.Error("nothing in, nothing out")
	}
}

// accountServer answers the calls a default-workspace lookup makes.
func accountServer(t *testing.T, workspaces any, workspaceStatus int, tokenWorkspace string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no", 401)
			return
		}
		switch r.URL.Path {
		case "/v2/workspaces":
			w.WriteHeader(workspaceStatus)
			_ = json.NewEncoder(w).Encode(workspaces)
		case "/v2/auth/context":
			_ = json.NewEncoder(w).Encode(map[string]any{"apiToken": map[string]any{"workspaceId": tokenWorkspace}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestEnsureDefaultWorkspace(t *testing.T) {
	list := []map[string]any{
		{"id": "new", "createdAt": "2025-01-01T00:00:00Z"},
		{"id": "old", "createdAt": "2020-01-01T00:00:00Z"},
	}
	for name, tc := range map[string]struct {
		workspaces any
		status     int
		token      string
		stored     string
		want       string
		wantErr    bool
	}{
		"the oldest of the account":           {list, 200, "tok-ws", "", "old", false},
		"an empty list uses the token's own":  {[]any{}, 200, "tok-ws", "", "tok-ws", false},
		"a refused list uses the token's own": {map[string]any{"message": "no"}, 403, "tok-ws", "", "tok-ws", false},
		"a stored default is kept":            {list, 200, "tok-ws", "chosen", "chosen", false},
		"nothing anywhere":                    {[]any{}, 200, "", "", "", false},
		"nothing anywhere and a refused list": {map[string]any{}, 403, "", "", "", true},
	} {
		t.Run(name, func(t *testing.T) {
			server := accountServer(t, tc.workspaces, tc.status, tc.token)
			cfg := &CliConfig{}
			if tc.stored != "" {
				cfg.SetDefaultWorkspace(server.URL, tc.stored)
			}
			got, err := cfg.EnsureDefaultWorkspace(server.URL+"/", "tok")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("id %q, err %v; want %q, error %v", got, err, tc.want, tc.wantErr)
			}
			if stored := cfg.DefaultWorkspaceFor(server.URL); stored != tc.want {
				t.Errorf("stored default = %q, want %q", stored, tc.want)
			}
		})
	}
}

func TestEnsureDefaultWorkspaceWithAnUnreachableAPIChangesNothing(t *testing.T) {
	server := accountServer(t, []any{}, 200, "x")
	url := server.URL
	server.Close()
	cfg := &CliConfig{}
	if id, err := cfg.EnsureDefaultWorkspace(url, "tok"); id != "" || err == nil || cfg.DefaultWorkspaceFor(url) != "" {
		t.Errorf("id %q, err %v, stored %q", id, err, cfg.DefaultWorkspaceFor(url))
	}
}

// loginServer is a server with the OAuth endpoints a login uses. With cliLink the
// short login link names the server's own CLI client; without, the link is 404.
type loginServer struct {
	*httptest.Server
	registered     int
	tokenClientIDs []string
}

func newLoginServer(t *testing.T, cliLink bool) *loginServer {
	t.Helper()
	s := &loginServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": s.URL + "/v2/auth/oauth2/authorize",
			"token_endpoint":         s.URL + "/v2/auth/oauth2/token",
			"registration_endpoint":  s.URL + "/v2/auth/oauth2/register",
		})
	})
	mux.HandleFunc("/v2/auth/oauth2/register", func(w http.ResponseWriter, r *http.Request) {
		s.registered++
		json.NewEncoder(w).Encode(map[string]string{"client_id": "registered-client"})
	})
	mux.HandleFunc("/v2/auth/cli/", func(w http.ResponseWriter, r *http.Request) {
		if !cliLink {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v2/auth/cli/"), "/"), "/")
		http.Redirect(w, r, s.URL+"/v2/auth/oauth2/authorize?"+url.Values{
			"client_id": {"server-cli-client"}, "redirect_uri": {"http://localhost:" + parts[1] + "/"},
			"code_challenge": {parts[0]}, "response_type": {"code"},
		}.Encode(), http.StatusFound)
	})
	mux.HandleFunc("/v2/auth/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Query().Get("redirect_uri")+"?code=the-code", http.StatusFound)
	})
	mux.HandleFunc("/v2/auth/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		s.tokenClientIDs = append(s.tokenClientIDs, body["client_id"])
		json.NewEncoder(w).Encode(map[string]string{"access_token": "login-token", "email": "ada@example.com"})
	})
	mux.HandleFunc("/v2/workspaces", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "newer", "name": "newer", "createdAt": "2025-01-01T00:00:00Z"},
			{"id": "oldest", "name": "oldest", "createdAt": "2021-01-01T00:00:00Z"},
		})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// follow plays the browser: it opens the login link and follows every redirect,
// the last one being the CLI's own callback on localhost.
func follow(t *testing.T) func(string) {
	return func(link string) {
		go func() {
			if resp, err := http.Get(link); err == nil {
				resp.Body.Close()
			}
		}()
	}
}

func TestLoginUsesTheServersCLIClientAndNeverRegistersOne(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	email, already, err := cfg.Login(server.URL, "", nil, follow(t), func(string) {}, 10*time.Second)
	if err != nil || already || email != "ada@example.com" {
		t.Fatalf("Login = %q, %v, %v", email, already, err)
	}
	if server.registered != 0 {
		t.Errorf("a client was registered %d times; the server's own CLI client must be used", server.registered)
	}
	if len(server.tokenClientIDs) != 1 || server.tokenClientIDs[0] != "server-cli-client" {
		t.Errorf("the code was exchanged with client %v, want server-cli-client", server.tokenClientIDs)
	}
	if cfg.TokenFor(server.URL) != "login-token" {
		t.Error("the token must be stored")
	}
}

// Every login ends with a default workspace, with no question asked: the oldest.
func TestLoginSetsTheOldestWorkspaceAsTheDefault(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	if _, _, err := cfg.Login(server.URL, "", nil, follow(t), func(string) {}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	saved, err := Load()
	if err != nil || saved.DefaultWorkspaceFor(server.URL) != "oldest" {
		t.Errorf("saved default = %q (%v), want oldest", saved.DefaultWorkspaceFor(server.URL), err)
	}
}

func TestLoginWithAWorkspaceFlagUsesThatWorkspace(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	if _, _, err := cfg.Login(server.URL, "newer", nil, follow(t), func(string) {}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultWorkspaceFor(server.URL) != "newer" {
		t.Errorf("default = %q, want newer", cfg.DefaultWorkspaceFor(server.URL))
	}
}

// A server without the CLI link (an older or self-hosted one) still gets a login,
// with a client registered on the fly.
func TestLoginRegistersAClientWhenTheServerHasNoCLILink(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, false)
	cfg := &CliConfig{}
	if _, _, err := cfg.Login(server.URL, "", nil, follow(t), func(string) {}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if server.registered != 1 || len(server.tokenClientIDs) != 1 || server.tokenClientIDs[0] != "registered-client" {
		t.Errorf("registered %d, exchanged with %v", server.registered, server.tokenClientIDs)
	}
}

// A browser also asks for /favicon.ico (and extension probes may hit the port):
// requests without a result must neither end the login nor block the server.
func TestLoginIgnoresStrayCallbackRequests(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	browser := func(link string) {
		go func() {
			port := link[strings.LastIndex(link, "/")+1:]
			if _, err := strconv.Atoi(port); err != nil {
				return
			}
			base := "http://127.0.0.1:" + port
			for _, path := range []string{"/favicon.ico", "/", "/other"} {
				if resp, err := http.Get(base + path); err == nil {
					resp.Body.Close()
				}
			}
			if resp, err := http.Get(link); err == nil { // the real redirect with the code
				resp.Body.Close()
			}
			for i := 0; i < 5; i++ { // and more requests after it
				if resp, err := http.Get(base + "/favicon.ico"); err == nil {
					resp.Body.Close()
				}
			}
		}()
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := cfg.Login(server.URL, "", nil, browser, func(string) {}, 10*time.Second)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Login = %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Login hung on stray callback requests")
	}
	if cfg.TokenFor(server.URL) != "login-token" {
		t.Errorf("token not stored")
	}
}

func TestOAuthEndpointsReportBadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer ts.Close()
	if _, err := fetchOAuthDiscovery(ts.URL); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("discovery error = %v", err)
	}
	if _, err := registerClient(ts.URL); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("registration error = %v", err)
	}
	if _, _, err := exchangeCode(ts.URL, "c", "code", "v", "http://localhost/"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("token error = %v", err)
	}
}

func TestLogoutRemovesTheLoginEvenWhenTheTokenCannotBeDestroyed(t *testing.T) {
	tempHome(t)
	for name, handler := range map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"unknown":      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) },
	} {
		ts := httptest.NewServer(handler)
		cfg := &CliConfig{}
		cfg.SetToken(ts.URL, "tok")
		wasLoggedIn, revokeErr, err := cfg.Logout(ts.URL)
		ts.Close()
		if err != nil || !wasLoggedIn || cfg.TokenFor(ts.URL) != "" {
			t.Fatalf("%s: removed locally = %v, %v", name, wasLoggedIn, err)
		}
		if (revokeErr != nil) != (name == "server error") {
			t.Errorf("%s: revoke error = %v", name, revokeErr)
		}
	}
	// offline: the API cannot be reached at all
	cfg := &CliConfig{}
	cfg.SetToken("http://127.0.0.1:1", "tok")
	if _, revokeErr, err := cfg.Logout("http://127.0.0.1:1"); err != nil || revokeErr == nil || cfg.TokenFor("http://127.0.0.1:1") != "" {
		t.Errorf("offline: revoke error %v, error %v", revokeErr, err)
	}
}

// A default workspace stored by an earlier login stays only while the new login can
// still see it: the API refuses requests scoped to a workspace the token cannot read.
func TestLoginDropsAStoredDefaultTheAccountCannotSee(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	cfg.SetDefaultWorkspace(server.URL, "from-another-account")
	if _, _, err := cfg.Login(server.URL, "", nil, follow(t), func(string) {}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := cfg.DefaultWorkspaceFor(server.URL); got != "oldest" {
		t.Errorf("default = %q, want oldest (the stored one is not in the account)", got)
	}
}

func TestLoginKeepsAStoredDefaultTheAccountCanSee(t *testing.T) {
	tempHome(t)
	t.Setenv("CI", "")
	server := newLoginServer(t, true)
	cfg := &CliConfig{}
	cfg.SetDefaultWorkspace(server.URL, "newer")
	if _, _, err := cfg.Login(server.URL, "", nil, follow(t), func(string) {}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := cfg.DefaultWorkspaceFor(server.URL); got != "newer" {
		t.Errorf("default = %q, want newer (still in the account)", got)
	}
}
