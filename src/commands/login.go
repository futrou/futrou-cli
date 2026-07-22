package commands

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"futrou-cli/src/config"
	"futrou-cli/src/constants"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var loginCommand = &cli.Command{
	Name:  "login",
	Usage: "Log in to Futrou Cloud on this machine",
	Flags: []cli.Flag{workspaceFlag},
	Action: func(c *cli.Context) error {
		apiUrl := services.NormalizeApiUrl(globalApiUrl(c))

		// If a token is already stored for this API URL, don't start a new flow.
		if cfg, err := config.Load(); err == nil && cfg.TokenFor(apiUrl) != "" {
			if isJSON(c) {
				return printJSON(map[string]string{"status": "already logged in"})
			}
			fmt.Printf("Already logged in to %s.\nRun 'futrou logout' to log out.\n", apiUrl)
			return nil
		}

		discovery, err := fetchOAuthDiscovery(apiUrl)
		if err != nil {
			return fmt.Errorf("fetching OAuth config: %w", err)
		}

		clientID, err := registerClient(discovery.RegistrationEndpoint)
		if err != nil {
			return fmt.Errorf("registering OAuth client: %w", err)
		}

		verifier, challenge, err := pkce()
		if err != nil {
			return fmt.Errorf("generating PKCE: %w", err)
		}

		// Start local callback server on a random available port.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("starting local server: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		redirectURI := fmt.Sprintf("http://localhost:%d/", port)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		srv := &http.Server{}
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

		go srv.Serve(listener)
		defer srv.Shutdown(context.Background())

		authURL := buildAuthURL(discovery.AuthorizationEndpoint, clientID, redirectURI, challenge)
		shortURL := buildShortAuthURL(apiUrl, challenge, redirectURI)
		if verifyShortAuthURL(shortURL, clientID, redirectURI, challenge) {
			authURL = shortURL
		}

		const loginTimeout = 5 * time.Minute
		expiresAt := time.Now().Add(loginTimeout)

		fmt.Printf("Please visit the below link in your browser and follow the instructions:\n\n  %s\n\n", authURL)
		openBrowserFunc(authURL)

		// Tick a countdown in interactive terminals; non-interactive gets no counter.
		interactive := isInteractiveTerminal()
		stopCountdown := make(chan struct{})
		if interactive {
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					remaining := time.Until(expiresAt)
					if remaining < 0 {
						remaining = 0
					}
					logger.UpdateLoader(fmt.Sprintf("Waiting for authentication (%s remaining)...", formatDuration(remaining)))
					select {
					case <-stopCountdown:
						return
					case <-ticker.C:
					}
				}
			}()
		}

		var code string
		select {
		case code = <-codeCh:
			close(stopCountdown)
			if interactive {
				logger.StopLoader()
			}
		case err = <-errCh:
			close(stopCountdown)
			if interactive {
				logger.StopLoader()
			}
			return err
		case <-time.After(loginTimeout):
			close(stopCountdown)
			if interactive {
				logger.StopLoader()
			}
			fmt.Println("Login link expired. Run 'futrou login' to try again.")
			return fmt.Errorf("login timed out")
		}

		token, userEmail, err := exchangeCode(discovery.TokenEndpoint, clientID, code, verifier, redirectURI)
		if err != nil {
			return fmt.Errorf("exchanging code for token: %w", err)
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg.ApiUrl = apiUrl
		cfg.SetToken(apiUrl, token)

		workspaceID, workspaceName, err := selectDefaultWorkspace(apiUrl, token, c.String("workspace"))
		if err != nil {
			return fmt.Errorf("selecting default workspace: %w", err)
		}
		if workspaceID != "" {
			cfg.SetDefaultWorkspace(apiUrl, workspaceID)
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		if isJSON(c) {
			return printJSON(map[string]string{
				"email":     userEmail,
				"status":    "logged in",
				"workspace": workspaceName,
			})
		}

		if userEmail != "" {
			fmt.Printf("✓ Logged in as %s\n", userEmail)
		} else {
			fmt.Println("✓ Logged in successfully")
		}
		if workspaceName != "" {
			fmt.Printf("✓ Default workspace set to %s\n", workspaceName)
		}
		return nil
	},
}

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
// up its own client_id/response_type/code_challenge_method defaults and
// resolves this to the real authorize request.
func buildShortAuthURL(apiUrl, challenge, redirectURI string) string {
	return apiUrl + "/v2/auth/cli/" + url.QueryEscape(challenge) + "/" + url.QueryEscape(redirectURI)
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
		// The API may return the email nested under a user object or at top level.
		Email string `json:"email"`
		User  struct {
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

// selectDefaultWorkspace determines the workspace to store as the default
// for apiUrl after a successful login. If flagValue is set, it resolves that
// workspace (by UUID or name) directly. Otherwise, in an interactive
// terminal, it prompts the user to choose among their workspaces. It returns
// empty strings (no error) when there's nothing to select or store, e.g. a
// brand-new account with no workspaces yet, or a non-interactive shell with
// no --workspace flag.
func selectDefaultWorkspace(apiUrl, token, flagValue string) (id, name string, err error) {
	client := services.NewApiClientWithToken(apiUrl, token)

	var workspaces []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	status, err := client.RequestInto("GET", "/v2/workspaces", nil, &workspaces)
	if err != nil {
		return "", "", err
	}
	if status >= 400 {
		return "", "", fmt.Errorf("listing workspaces failed with status %d", status)
	}
	if len(workspaces) == 0 {
		return "", "", nil
	}

	if flagValue != "" {
		if looksLikeUUID(flagValue) {
			for _, w := range workspaces {
				if w.Id == flagValue {
					return w.Id, w.Name, nil
				}
			}
			return flagValue, flagValue, nil
		}
		for _, w := range workspaces {
			if w.Name == flagValue {
				return w.Id, w.Name, nil
			}
		}
		return "", "", fmt.Errorf("no workspace named %q found", flagValue)
	}

	if !isInteractiveTerminal() {
		return "", "", nil
	}

	if len(workspaces) == 1 {
		return workspaces[0].Id, workspaces[0].Name, nil
	}

	fmt.Println("\nSelect a default workspace:")
	for i, w := range workspaces {
		fmt.Printf("  %d) %s\n", i+1, w.Name)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter a number: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return "", "", fmt.Errorf("reading workspace selection: %w", readErr)
		}
		choice, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || choice < 1 || choice > len(workspaces) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(workspaces))
			continue
		}
		w := workspaces[choice-1]
		return w.Id, w.Name, nil
	}
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// formatDuration renders d as h:mm:ss, m:ss, or Ns, using the coarsest
// unit that fits.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	case m > 0:
		return fmt.Sprintf("%d:%02d", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// openBrowserFunc is the function used to open a URL in the default browser.
// Tests replace it with a no-op to avoid launching a real browser.
var openBrowserFunc = openBrowser

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Start()
}
