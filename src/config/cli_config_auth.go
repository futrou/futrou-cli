// Package config additions: session lifecycle (login/logout/whoami) built
// on top of CliConfig, using raw net/http rather than the services package
// to avoid an import cycle (services already imports cliconfig -> config).
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"futrou-cli/src/constants"
)

// WhoamiUser is the identity returned by CliConfig.Whoami.
type WhoamiUser struct {
	ID       string
	Fullname string
	Email    string
}

// ErrCIRequiresToken is returned by EnsureLoggedIn (and Login, when no
// token is available under CI) instead of attempting an interactive
// browser flow that nobody in a CI environment could complete.
var ErrCIRequiresToken = errors.New("not logged in — CI environment requires a non-empty --api-token (or FUTROU_API_TOKEN)")

type oauthDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func fetchOAuthDiscovery(apiUrl string) (*oauthDiscovery, error) {
	resp, err := http.Get(apiUrl + "/.well-known/oauth-authorization-server")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d oauthDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

func registerClient(registrationEndpoint string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"client_name":    constants.Name,
		"redirect_uris":  []string{"http://localhost"},
		"grant_types":    []string{"authorization_code"},
		"response_types": []string{"code"},
		"logo_uri":       constants.LogoUrl,
	})
	resp, err := http.Post(registrationEndpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return "", err
	}
	if reg.ClientID == "" {
		return "", fmt.Errorf("no client_id in registration response")
	}
	return reg.ClientID, nil
}

func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

// buildAuthURL constructs the full, explicit OAuth authorize URL.
func buildAuthURL(authEndpoint, clientID, redirectURI, challenge string) string {
	params := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"response_type":         {"code"},
		"code_challenge_method": {"S256"},
	}
	return authEndpoint + "?" + params.Encode()
}

// buildShortAuthURL builds the compact path-based login link the default
// Futrou API accepts in place of the full authorize URL: the server looks
// up its own client_id/response_type/code_challenge_method defaults, assumes
// a redirect_uri of http://localhost:<port>/, and resolves this to the real
// authorize request.
func buildShortAuthURL(apiUrl, challenge string, port int) string {
	return apiUrl + "/v2/auth/cli/" + url.QueryEscape(challenge) + "/" + strconv.Itoa(port)
}

// verifyShortAuthURL confirms shortURL redirects (without following it) to
// an authorize request whose client_id, redirect_uri, and code_challenge
// exactly match what this login expects. This guards against the short
// link being unavailable, misconfigured, or resolving to something
// unexpected on a server that merely shares the default API's hostname —
// callers should fall back to the full explicit authorize URL when this
// returns false.
func verifyShortAuthURL(shortURL, clientID, redirectURI, challenge string) bool {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(shortURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return false
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return false
	}
	q := location.Query()
	return q.Get("client_id") == clientID &&
		q.Get("redirect_uri") == redirectURI &&
		q.Get("code_challenge") == challenge
}

func exchangeCode(tokenEndpoint, clientID, code, verifier, redirectURI string) (token, email string, err error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
	})
	resp, err := http.Post(tokenEndpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Email       string `json:"email"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if result.AccessToken == "" {
		return "", "", fmt.Errorf("no access_token in token response")
	}
	if result.Email == "" {
		result.Email = result.User.Email
	}
	return result.AccessToken, result.Email, nil
}
