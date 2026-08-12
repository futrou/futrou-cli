package commands

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/manifoldco/promptui"
	"github.com/urfave/cli/v2"
)

// loginTimeout is how long the CLI waits for the OAuth callback before
// giving up. A var (not const) so tests can shrink it instead of waiting
// out the real duration.
var loginTimeout = 5 * time.Minute

var loginCommand = &cli.Command{
	Name:  "login",
	Usage: "Log in to Futrou Cloud on this machine",
	Flags: []cli.Flag{workspaceFlag},
	Action: func(c *cli.Context) error {
		apiUrl := services.NormalizeApiUrl(globalApiUrl(c))

		// If a token is already stored for this API URL, don't start a new flow.
		if cfg, err := cliconfig.Load(); err == nil && cfg.TokenFor(apiUrl) != "" {
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
		shortURL := buildShortAuthURL(apiUrl, challenge, port)
		if verifyShortAuthURL(shortURL, clientID, redirectURI, challenge) {
			authURL = shortURL
		}

		expiresAt := time.Now().Add(loginTimeout)

		fmt.Printf("Please visit the below link in your browser and follow the instructions:\n\n  %s\n\n", authURL)
		openBrowserFunc(authURL)

		// Tick a countdown in interactive terminals; non-interactive gets no counter.
		interactive := isInteractiveTerminal()
		stopCountdown := make(chan struct{})
		var countdownDone <-chan struct{}
		if interactive {
			countdownDone = startCountdown(stopCountdown, expiresAt, time.Second)
		}
		stopCountdownAndLoader := func() {
			close(stopCountdown)
			if interactive {
				<-countdownDone
				logger.StopLoader()
			}
		}

		var code string
		select {
		case code = <-codeCh:
			stopCountdownAndLoader()
		case err = <-errCh:
			stopCountdownAndLoader()
			return err
		case <-time.After(loginTimeout):
			stopCountdownAndLoader()
			fmt.Println("Login link expired. Run 'futrou login' to try again.")
			return fmt.Errorf("login timed out")
		}

		token, userEmail, err := exchangeCode(discovery.TokenEndpoint, clientID, code, verifier, redirectURI)
		if err != nil {
			return fmt.Errorf("exchanging code for token: %w", err)
		}

		cfg, err := cliconfig.Load()
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

		if err := cliconfig.Save(cfg); err != nil {
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

// startCountdown starts a goroutine that updates the loader with the time
// remaining until expiresAt, once per interval, until stopCountdown is
// closed. It returns a channel that is closed once the goroutine has fully
// exited and will make no further calls into logger. Callers must wait on
// this channel before calling logger.StopLoader, otherwise a call to
// UpdateLoader (which can restart the spinner via StartLoader) can race the
// spinner's WaitGroup with the Wait inside StopLoader and panic.
func startCountdown(stopCountdown <-chan struct{}, expiresAt time.Time, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCountdown:
				return
			default:
			}
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
	return done
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

	names := make([]string, len(workspaces))
	for i, w := range workspaces {
		names[i] = w.Name
	}
	prompt := promptui.Select{
		Label: "Select a default workspace",
		Items: names,
	}
	choice, _, err := prompt.Run()
	if err != nil {
		return "", "", fmt.Errorf("selecting workspace: %w", err)
	}
	w := workspaces[choice]
	return w.Id, w.Name, nil
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
