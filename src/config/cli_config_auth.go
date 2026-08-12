// Package config additions: session lifecycle (login/logout/whoami) built
// on top of CliConfig, using raw net/http rather than the services package
// to avoid an import cycle (services already imports cliconfig -> config).
package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
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

// NormalizeApiUrl strips a trailing slash and a trailing "/v2" so that both
// "https://api.futrou.com" and "https://api.futrou.com/v2" resolve the same.
func NormalizeApiUrl(apiUrl string) string {
	apiUrl = strings.TrimRight(apiUrl, "/")
	apiUrl = strings.TrimSuffix(apiUrl, "/v2")
	return apiUrl
}

// Workspace is a minimal workspace summary used by SelectWorkspaceFunc.
type Workspace struct {
	ID   string
	Name string
}

// SelectWorkspaceFunc resolves which workspace to store as the default
// after a successful login. It's injected by the caller (the commands
// package) so this package never depends on an interactive prompt
// library. A nil SelectWorkspaceFunc means "don't select a workspace."
type SelectWorkspaceFunc func(workspaces []Workspace) (id, name string, err error)

// Login runs the OAuth PKCE browser flow for apiUrl: discovery, dynamic
// client registration (with logo_uri), a local callback server, opening
// the authorize URL via openBrowser, exchanging the returned code for a
// token, and (if selectWorkspace is non-nil) resolving a default
// workspace. The resulting token and default workspace are stored on cfg
// and saved to disk.
//
// onAuthURL is called once with the URL the user should visit (the
// caller is expected to print it); openBrowser is called with the same
// URL to attempt opening it automatically.
//
// If a token is already stored for apiUrl, Login returns immediately
// with alreadyLoggedIn=true and does not start a new flow.
//
// Under CI (CI=true), Login returns ErrCIRequiresToken instead of
// starting an interactive flow nobody in that environment could
// complete.
func (cfg *CliConfig) Login(
	apiUrl, workspaceFlag string,
	selectWorkspace SelectWorkspaceFunc,
	openBrowser func(string),
	onAuthURL func(string),
	loginTimeout time.Duration,
) (email string, alreadyLoggedIn bool, err error) {
	apiUrl = NormalizeApiUrl(apiUrl)

	if cfg.TokenFor(apiUrl) != "" {
		return "", true, nil
	}

	if os.Getenv("CI") == "true" {
		return "", false, ErrCIRequiresToken
	}

	discovery, err := fetchOAuthDiscovery(apiUrl)
	if err != nil {
		return "", false, fmt.Errorf("fetching OAuth config: %w", err)
	}

	clientID, err := registerClient(discovery.RegistrationEndpoint)
	if err != nil {
		return "", false, fmt.Errorf("registering OAuth client: %w", err)
	}

	verifier, challenge, err := pkce()
	if err != nil {
		return "", false, fmt.Errorf("generating PKCE: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", false, fmt.Errorf("starting local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errCh <- fmt.Errorf("authorization denied: %s", errParam)
			fmt.Fprintf(w, "<html><body><h2>Authorization denied. You can close this window.</h2></body></html>")
			return
		}
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		codeCh <- code
		fmt.Fprintf(w, "<html><body><h2>Logged in! You can close this window.</h2></body></html>")
	})
	srv := &http.Server{Handler: mux}

	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	authURL := buildAuthURL(discovery.AuthorizationEndpoint, clientID, redirectURI, challenge)
	shortURL := buildShortAuthURL(apiUrl, challenge, port)
	if verifyShortAuthURL(shortURL, clientID, redirectURI, challenge) {
		authURL = shortURL
	}

	if onAuthURL != nil {
		onAuthURL(authURL)
	}
	if openBrowser != nil {
		openBrowser(authURL)
	}

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return "", false, err
	case <-time.After(loginTimeout):
		return "", false, fmt.Errorf("login timed out")
	}

	token, userEmail, err := exchangeCode(discovery.TokenEndpoint, clientID, code, verifier, redirectURI)
	if err != nil {
		return "", false, fmt.Errorf("exchanging code for token: %w", err)
	}

	cfg.ApiUrl = apiUrl
	cfg.SetToken(apiUrl, token)

	if selectWorkspace != nil {
		workspaces, err := listWorkspaces(apiUrl, token)
		if err != nil {
			return "", false, fmt.Errorf("listing workspaces: %w", err)
		}
		if len(workspaces) > 0 {
			workspaceID, _, err := resolveOrPromptWorkspace(workspaces, workspaceFlag, selectWorkspace)
			if err != nil {
				return "", false, fmt.Errorf("selecting default workspace: %w", err)
			}
			if workspaceID != "" {
				cfg.SetDefaultWorkspace(apiUrl, workspaceID)
			}
		}
	}

	if err := Save(cfg); err != nil {
		return "", false, fmt.Errorf("saving config: %w", err)
	}

	return userEmail, false, nil
}

// resolveOrPromptWorkspace resolves workspaceFlag (by UUID or name)
// directly if non-empty; otherwise delegates to selectWorkspace.
func resolveOrPromptWorkspace(workspaces []Workspace, workspaceFlag string, selectWorkspace SelectWorkspaceFunc) (id, name string, err error) {
	if workspaceFlag != "" {
		for _, w := range workspaces {
			if w.ID == workspaceFlag || w.Name == workspaceFlag {
				return w.ID, w.Name, nil
			}
		}
		return "", "", fmt.Errorf("no workspace named %q found", workspaceFlag)
	}
	return selectWorkspace(workspaces)
}

// listWorkspaces fetches the caller's workspaces from apiUrl using token.
func listWorkspaces(apiUrl, token string) ([]Workspace, error) {
	req, err := http.NewRequest("GET", NormalizeApiUrl(apiUrl)+"/v2/workspaces", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("listing workspaces failed with status %d", resp.StatusCode)
	}

	var raw []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	workspaces := make([]Workspace, len(raw))
	for i, w := range raw {
		workspaces[i] = Workspace{ID: w.Id, Name: w.Name}
	}
	return workspaces, nil
}

// Logout removes the stored token and default workspace for apiUrl and
// saves cfg. wasLoggedIn reports whether a token was present beforehand.
func (cfg *CliConfig) Logout(apiUrl string) (wasLoggedIn bool, err error) {
	apiUrl = NormalizeApiUrl(apiUrl)
	wasLoggedIn = cfg.TokenFor(apiUrl) != ""
	cfg.RemoveApiUrl(apiUrl)
	if err := Save(cfg); err != nil {
		return wasLoggedIn, err
	}
	return wasLoggedIn, nil
}

// EnsureLoggedIn returns a usable API token for apiUrl: tokenOverride or
// the stored token if either is non-empty; otherwise (outside CI) it runs
// Login interactively and returns the freshly obtained token. In CI, with
// no token available, it returns ErrCIRequiresToken.
func (cfg *CliConfig) EnsureLoggedIn(
	apiUrl, tokenOverride string,
	selectWorkspace SelectWorkspaceFunc,
	openBrowser func(string),
	onAuthURL func(string),
	loginTimeout time.Duration,
) (token string, err error) {
	apiUrl = NormalizeApiUrl(apiUrl)

	if tokenOverride != "" {
		return tokenOverride, nil
	}
	if stored := cfg.TokenFor(apiUrl); stored != "" {
		return stored, nil
	}
	if os.Getenv("CI") == "true" {
		return "", ErrCIRequiresToken
	}

	_, _, err = cfg.Login(apiUrl, "", selectWorkspace, openBrowser, onAuthURL, loginTimeout)
	if err != nil {
		return "", err
	}
	return cfg.TokenFor(apiUrl), nil
}

// Whoami fetches the authenticated user's identity from apiUrl's
// /v2/auth/context endpoint. tokenOverride, if non-empty, is used instead
// of cfg's stored token.
func (cfg *CliConfig) Whoami(apiUrl, tokenOverride string) (WhoamiUser, error) {
	apiUrl = NormalizeApiUrl(apiUrl)

	token := tokenOverride
	if token == "" {
		token = cfg.TokenFor(apiUrl)
	}
	if token == "" {
		return WhoamiUser{}, fmt.Errorf("not logged in — run 'futrou login' or set FUTROU_API_TOKEN")
	}

	req, err := http.NewRequest("GET", apiUrl+"/v2/auth/context", nil)
	if err != nil {
		return WhoamiUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return WhoamiUser{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return WhoamiUser{}, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	var result struct {
		User struct {
			ID       string `json:"id"`
			Fullname string `json:"fullname"`
			Email    string `json:"email"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return WhoamiUser{}, err
	}
	return WhoamiUser{ID: result.User.ID, Fullname: result.User.Fullname, Email: result.User.Email}, nil
}
