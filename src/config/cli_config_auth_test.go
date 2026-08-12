package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestWhoami_noTokenReturnsError(t *testing.T) {
	tempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	_, err = cfg.Whoami("https://api.example.com", "")
	if err == nil {
		t.Fatal("expected error when no token is available")
	}
}

func TestWhoami_returnsUserFromApi(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/auth/context" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok-abc" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": map[string]string{"id": "u1", "fullname": "Ada Lovelace", "email": "ada@example.com"},
		})
	}))
	defer ts.Close()

	tempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	user, err := cfg.Whoami(ts.URL, "tok-abc")
	if err != nil {
		t.Fatalf("Whoami() error: %v", err)
	}
	if user.Email != "ada@example.com" || user.Fullname != "Ada Lovelace" || user.ID != "u1" {
		t.Fatalf("unexpected user: %#v", user)
	}
}
