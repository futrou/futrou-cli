// Session lifecycle (login/logout) built on top of CliConfig. It uses raw
// net/http rather than the services package, which already imports cliconfig.
package cliconfig

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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"
	"futrou-cli/src/utils"
)

// ErrCIRequiresToken is returned by EnsureLoggedIn (and Login, when no
// token is available under CI) instead of attempting an interactive
// browser flow that nobody in a CI environment could complete.
var ErrCIRequiresToken = errors.New("not logged in — CI environment requires a non-empty --api-token (or FUTROU_API_TOKEN)")

// oauthTimeout is how long one request of the login flow may take. Without it a
// server that accepts the connection and never answers would hang `futrou login`
// for ever; tests shorten it.
var oauthTimeout = 30 * time.Second

func oauthClient() *http.Client { return &http.Client{Timeout: oauthTimeout} }

type oauthDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func fetchOAuthDiscovery(apiUrl string) (*oauthDiscovery, error) {
	resp, err := oauthClient().Get(apiUrl + "/.well-known/oauth-authorization-server")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
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
	resp, err := oauthClient().Post(registrationEndpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}
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

// shortAuthClientID asks the server's short login link (without following it)
// which client it logs in with, and returns that client_id when the link
// redirects to an authorize request for exactly this login: the same
// redirect_uri and code_challenge. It returns "" when the link is unavailable,
// misconfigured or resolves to something unexpected (a server that merely shares
// the default API's hostname), and the caller then registers its own client.
func shortAuthClientID(shortURL, redirectURI, challenge string) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(shortURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return ""
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return ""
	}
	q := location.Query()
	if q.Get("redirect_uri") != redirectURI || q.Get("code_challenge") != challenge {
		return ""
	}
	return q.Get("client_id")
}

func exchangeCode(tokenEndpoint, clientID, code, verifier, redirectURI string) (token, email string, err error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
	})
	resp, err := oauthClient().Post(tokenEndpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

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

// Workspace is a minimal workspace summary used by SelectWorkspaceFunc.
type Workspace struct {
	ID        string
	Name      string
	CreatedAt time.Time
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
	apiUrl = utils.NormalizeApiUrl(apiUrl)

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

	// The server's own login link names the CLI's built-in client, whose token is
	// bound to the workspace the user picks and acts with their role there. A client
	// registered on the fly gets no permissions at all, so it is only the fallback
	// for a server without that link.
	shortURL := buildShortAuthURL(apiUrl, challenge, port)
	authURL := shortURL
	clientID := shortAuthClientID(shortURL, redirectURI, challenge)
	if clientID == "" {
		clientID, err = registerClient(discovery.RegistrationEndpoint)
		if err != nil {
			listener.Close()
			return "", false, fmt.Errorf("registering OAuth client: %w", err)
		}
		authURL = buildAuthURL(discovery.AuthorizationEndpoint, clientID, redirectURI, challenge)
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// send delivers the first result and drops later ones, so a repeat request
	// (the browser also asks for /favicon.ico) can never block a handler.
	sendCode := func(code string) {
		select {
		case codeCh <- code:
		default:
		}
	}
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	// the page the browser shows is written only when the login has ended (token
	// exchanged or failed), so it tells the truth about the result
	finished := make(chan struct{})
	var finishOnce sync.Once
	var finishErr error
	finish := func(err error) {
		finishOnce.Do(func() {
			finishErr = err
			close(finished)
		})
	}
	writePage := func(w http.ResponseWriter, failure error, theme string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, renderCallbackPage(failure, theme))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// only the redirect itself counts: anything else (favicon, probes,
		// requests without a result) is ignored and never ends the login
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		code := r.URL.Query().Get("code")
		errParam := r.URL.Query().Get("error")
		theme := r.URL.Query().Get("theme")
		switch {
		case errParam != "":
			sendErr(fmt.Errorf("authorization denied: %s", errParam))
			writePage(w, fmt.Errorf("authorization denied (%s)", errParam), theme)
		case code != "":
			sendCode(code)
			select {
			case <-finished:
				writePage(w, finishErr, theme)
			case <-time.After(30 * time.Second):
				writePage(w, nil, theme)
			case <-r.Context().Done():
			}
		default:
			http.NotFound(w, r)
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go srv.Serve(listener)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(ctx) != nil {
			srv.Close()
		}
	}()

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
		err = fmt.Errorf("exchanging code for token: %w", err)
		finish(err)
		return "", false, err
	}

	cfg.ApiUrl = apiUrl
	if err := cfg.SetToken(apiUrl, token); err != nil {
		err = fmt.Errorf("storing the token: %w", err)
		finish(err)
		return "", false, err
	}
	finish(nil)

	// every login ends with a default workspace, so that no command asks for one:
	// the one named with --workspace, the one the caller's picker chooses, else
	// the one stored before, else the oldest (or the token's own, see EnsureDefaultWorkspace)
	workspaces, listErr := listWorkspaces(apiUrl, token)
	if listErr != nil && workspaceFlag != "" && !utils.IsUuid(workspaceFlag) {
		return "", false, fmt.Errorf("listing workspaces: %w", listErr)
	}
	chosen := ""
	if workspaceFlag != "" || (selectWorkspace != nil && len(workspaces) > 0) {
		picker := selectWorkspace
		if picker == nil {
			picker = func([]Workspace) (string, string, error) { return "", "", nil }
		}
		chosen, _, err = resolveOrPromptWorkspace(workspaces, workspaceFlag, picker)
		if err != nil {
			return "", false, fmt.Errorf("selecting default workspace: %w", err)
		}
	}
	if chosen != "" {
		cfg.SetDefaultWorkspace(apiUrl, chosen)
	} else {
		// a default stored by an earlier login (maybe of another account) is kept
		// only while this one can see it: the API answers 403 to a request scoped
		// to a workspace the token cannot read
		if stored := cfg.DefaultWorkspaceFor(apiUrl); stored != "" && len(workspaces) > 0 && !hasWorkspace(workspaces, stored) {
			delete(cfg.DefaultWorkspaces, normalizeUrlKey(apiUrl))
		}
		if _, err := cfg.EnsureDefaultWorkspace(apiUrl, token); err != nil {
			logger.Warn("no default workspace was set (%v): pass --workspace to the commands that need one", err)
		}
	}

	if err := Save(cfg); err != nil {
		return "", false, fmt.Errorf("saving config: %w", err)
	}

	return userEmail, false, nil
}

// hasWorkspace reports whether one of the workspaces has the ID id.
func hasWorkspace(workspaces []Workspace, id string) bool {
	for _, workspace := range workspaces {
		if workspace.ID == id {
			return true
		}
	}
	return false
}

// resolveOrPromptWorkspace resolves workspaceFlag (by UUID or name)
// directly if non-empty; otherwise delegates to selectWorkspace. A
// UUID-shaped workspaceFlag that isn't found in workspaces is passed
// through as-is (both id and name), since the /v2/workspaces listing may
// be paginated or incomplete — the caller may still know a valid workspace
// ID that just isn't present in this particular listing.
func resolveOrPromptWorkspace(workspaces []Workspace, workspaceFlag string, selectWorkspace SelectWorkspaceFunc) (id, name string, err error) {
	if workspaceFlag != "" {
		for _, w := range workspaces {
			if w.ID == workspaceFlag || w.Name == workspaceFlag {
				return w.ID, w.Name, nil
			}
		}
		if utils.IsUuid(workspaceFlag) {
			return workspaceFlag, workspaceFlag, nil
		}
		return "", "", fmt.Errorf("no workspace named %q found", workspaceFlag)
	}
	return selectWorkspace(workspaces)
}

// listWorkspaces fetches the caller's workspaces from apiUrl using token.
func listWorkspaces(apiUrl, token string) ([]Workspace, error) {
	req, err := http.NewRequest("GET", utils.NormalizeApiUrl(apiUrl)+"/v2/workspaces", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := oauthClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("listing workspaces failed with status %d", resp.StatusCode)
	}

	var raw []struct {
		Id        string    `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"createdAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	workspaces := make([]Workspace, len(raw))
	for i, w := range raw {
		workspaces[i] = Workspace{ID: w.Id, Name: w.Name, CreatedAt: w.CreatedAt}
	}
	return OldestFirst(workspaces), nil
}

// OldestFirst orders workspaces by creation time, the oldest first. Workspaces
// without a creation time keep their order, after the ones that have one.
func OldestFirst(workspaces []Workspace) []Workspace {
	sorted := append([]Workspace(nil), workspaces...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].CreatedAt, sorted[j].CreatedAt
		switch {
		case a.IsZero() || b.IsZero():
			return !a.IsZero() && b.IsZero()
		}
		return a.Before(b)
	})
	return sorted
}

// tokenWorkspace is the workspace the API says the token belongs to (an API key
// of a workspace, or the workspace of a login), or "" when it names none.
func tokenWorkspace(apiUrl, token string) (string, error) {
	req, err := http.NewRequest("GET", utils.NormalizeApiUrl(apiUrl)+"/v2/auth/context", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := oauthClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("reading the login failed with status %d", resp.StatusCode)
	}
	var context struct {
		APIToken struct {
			WorkspaceID string `json:"workspaceId"`
		} `json:"apiToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&context); err != nil {
		return "", err
	}
	return context.APIToken.WorkspaceID, nil
}

// EnsureDefaultWorkspace makes sure a default workspace is stored for apiUrl and
// returns it: the one already stored, else the oldest workspace of the account,
// else the workspace the token itself belongs to (an account whose workspace
// list is empty, such as a staff user or an API key, still has that one). It
// returns "" when there is none. It changes cfg but does not save it.
func (cfg *CliConfig) EnsureDefaultWorkspace(apiUrl, token string) (string, error) {
	apiUrl = utils.NormalizeApiUrl(apiUrl)
	if id := cfg.DefaultWorkspaceFor(apiUrl); id != "" {
		return id, nil
	}
	id := ""
	workspaces, listErr := listWorkspaces(apiUrl, token)
	if len(workspaces) > 0 {
		id = workspaces[0].ID
	}
	var tokenErr error
	if id == "" {
		id, tokenErr = tokenWorkspace(apiUrl, token)
	}
	if id == "" {
		if listErr != nil {
			return "", listErr
		}
		return "", tokenErr
	}
	cfg.SetDefaultWorkspace(apiUrl, id)
	return id, nil
}

// RevokeToken asks the API to destroy token (RFC 7009), so a copy of it that
// leaked is worthless. A token the API no longer knows (401/404) is already gone.
func RevokeToken(apiUrl, token string) error {
	body, _ := json.Marshal(map[string]string{"token": token, "token_type_hint": "access_token"})
	req, err := http.NewRequest("POST", utils.NormalizeApiUrl(apiUrl)+"/v2/auth/oauth2/revoke", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := oauthClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("revoking the token failed with status %d", resp.StatusCode)
	}
	return nil
}

// Logout destroys the stored token at the API, then removes it and the default
// workspace for apiUrl from cfg and saves cfg. The login is removed locally even
// when the API could not be asked (offline): revokeErr then says the token was
// not destroyed. wasLoggedIn reports whether a token was present beforehand.
func (cfg *CliConfig) Logout(apiUrl string) (wasLoggedIn bool, revokeErr error, err error) {
	apiUrl = utils.NormalizeApiUrl(apiUrl)
	token := cfg.TokenFor(apiUrl)
	wasLoggedIn = token != ""
	if wasLoggedIn {
		revokeErr = RevokeToken(apiUrl, token)
	}
	cfg.RemoveApiUrl(apiUrl)
	if err := Save(cfg); err != nil {
		return wasLoggedIn, revokeErr, err
	}
	return wasLoggedIn, revokeErr, nil
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
	apiUrl = utils.NormalizeApiUrl(apiUrl)

	if tokenOverride != "" {
		return tokenOverride, nil
	}
	if stored := cfg.TokenFor(apiUrl); stored != "" {
		return stored, nil
	}
	if os.Getenv("CI") == "true" {
		return "", ErrCIRequiresToken
	}

	// workspaceFlag is intentionally empty here: EnsureLoggedIn is the
	// auto-login path invoked from arbitrary commands via requireAuth, most
	// of which have no --workspace flag of their own to thread through. If
	// a workspace default needs setting, the user runs `futrou login
	// --workspace <x>` explicitly instead.
	_, _, err = cfg.Login(apiUrl, "", selectWorkspace, openBrowser, onAuthURL, loginTimeout)
	if err != nil {
		return "", err
	}
	return cfg.TokenFor(apiUrl), nil
}
