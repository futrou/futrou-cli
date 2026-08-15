package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestVerifyShortAuthURL_matches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := "https://api.futrou.com/v2/auth/oauth2/authorize?" + url.Values{
			"client_id":      {"client-1"},
			"redirect_uri":   {"http://localhost:12345/"},
			"code_challenge": {"challenge-abc"},
			"response_type":  {"code"},
		}.Encode()
		http.Redirect(w, r, loc, http.StatusFound)
	}))
	defer ts.Close()

	ok := verifyShortAuthURL(ts.URL, "client-1", "http://localhost:12345/", "challenge-abc")
	if !ok {
		t.Error("expected verifyShortAuthURL to match, got false")
	}
}

func TestVerifyShortAuthURL_clientIDMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := "https://api.futrou.com/v2/auth/oauth2/authorize?" + url.Values{
			"client_id":      {"unexpected-client"},
			"redirect_uri":   {"http://localhost:12345/"},
			"code_challenge": {"challenge-abc"},
		}.Encode()
		http.Redirect(w, r, loc, http.StatusFound)
	}))
	defer ts.Close()

	ok := verifyShortAuthURL(ts.URL, "client-1", "http://localhost:12345/", "challenge-abc")
	if ok {
		t.Error("expected verifyShortAuthURL to reject a client_id mismatch")
	}
}

func TestVerifyShortAuthURL_redirectURIMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := "https://api.futrou.com/v2/auth/oauth2/authorize?" + url.Values{
			"client_id":      {"client-1"},
			"redirect_uri":   {"http://localhost:99999/"},
			"code_challenge": {"challenge-abc"},
		}.Encode()
		http.Redirect(w, r, loc, http.StatusFound)
	}))
	defer ts.Close()

	ok := verifyShortAuthURL(ts.URL, "client-1", "http://localhost:12345/", "challenge-abc")
	if ok {
		t.Error("expected verifyShortAuthURL to reject a redirect_uri mismatch")
	}
}

func TestVerifyShortAuthURL_codeChallengeMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := "https://api.futrou.com/v2/auth/oauth2/authorize?" + url.Values{
			"client_id":      {"client-1"},
			"redirect_uri":   {"http://localhost:12345/"},
			"code_challenge": {"different-challenge"},
		}.Encode()
		http.Redirect(w, r, loc, http.StatusFound)
	}))
	defer ts.Close()

	ok := verifyShortAuthURL(ts.URL, "client-1", "http://localhost:12345/", "challenge-abc")
	if ok {
		t.Error("expected verifyShortAuthURL to reject a code_challenge mismatch")
	}
}

func TestVerifyShortAuthURL_notFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer ts.Close()

	ok := verifyShortAuthURL(ts.URL, "client-1", "http://localhost:12345/", "challenge-abc")
	if ok {
		t.Error("expected verifyShortAuthURL to fail on a non-redirect response")
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
	cfg := &CliConfig{}
	cfg.SetToken("https://api.example.com", "tok")
	cfg.SetDefaultWorkspace("https://api.example.com", "ws-1")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	wasLoggedIn, err := cfg2.Logout("https://api.example.com")
	if err != nil {
		t.Fatalf("Logout() error: %v", err)
	}
	if !wasLoggedIn {
		t.Fatal("expected wasLoggedIn=true")
	}
	if cfg2.TokenFor("https://api.example.com") != "" {
		t.Fatal("expected token removed after Logout()")
	}

	cfg3, _ := Load()
	wasLoggedIn2, err := cfg3.Logout("https://api.example.com")
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

func TestLooksLikeUUID(t *testing.T) {
	cases := map[string]bool{
		"11111111-1111-1111-1111-111111111111": true,
		"prod":                                 false,
		"":                                     false,
		"11111111-1111-1111-1111-11111111111":  false, // one char short
	}
	for in, want := range cases {
		if got := looksLikeUUID(in); got != want {
			t.Errorf("looksLikeUUID(%q) = %v, want %v", in, got, want)
		}
	}
}

