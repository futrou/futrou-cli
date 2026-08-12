package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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
