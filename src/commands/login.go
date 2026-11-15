package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/logger"
	"futrou-cli/src/utils"

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
		apiUrl := utils.NormalizeApiUrl(globalApiUrl(c))

		cfg, err := cliconfig.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// A stored token proves nothing: when the API does not accept it, the login
		// is done again instead of telling the user they are logged in.
		var current *sessionInfo // who is logged in, when the stored login was checked and works
		if stored := cfg.TokenFor(apiUrl); stored != "" && globalApiToken(c) == "" {
			stopLoader := startLoader(c, "Checking the stored login...")
			state, context, _ := checkSession(apiUrl, stored)
			if state == sessionValid {
				// a login made before defaults existed, or by a client that skipped
				// the choice, gets its default workspace now
				if _, err := cfg.EnsureDefaultWorkspace(apiUrl, stored); err == nil {
					_ = cliconfig.Save(cfg)
				}
				info := newSessionInfo(apiUrl, stored, cfg, *context, false)
				current = &info
			}
			stopLoader()
			if state == sessionRejected {
				if !isJSON(c) {
					fmt.Printf("The stored login for %s is no longer valid. Logging in again.\n", apiUrl)
				}
				cfg.ClearToken(apiUrl)
			}
		}

		expiresAt := time.Now().Add(loginTimeout)
		interactive := isInteractiveTerminal()
		stopCountdown := make(chan struct{})
		var countdownDone <-chan struct{}

		onAuthURL := func(authURL string) {
			fmt.Printf("Please visit the below link in your browser and follow the instructions:\n\n  %s\n\n", authURL)
			if interactive {
				countdownDone = startCountdown(stopCountdown, expiresAt, time.Second)
			}
		}

		email, alreadyLoggedIn, err := cfg.Login(apiUrl, c.String("workspace"), nil, openBrowserFunc, onAuthURL, loginTimeout)

		if interactive && countdownDone != nil {
			close(stopCountdown)
			<-countdownDone
			logger.StopLoader()
		}

		if err != nil {
			return err
		}

		if alreadyLoggedIn {
			if isJSON(c) {
				output := map[string]interface{}{"status": "already logged in"}
				if current != nil {
					for key, value := range current.json() {
						output[key] = value
					}
				}
				return printJSON(output)
			}
			if current == nil { // not checked: --api-token given, or the API could not be asked
				fmt.Printf("Already logged in to %s.\nRun 'futrou logout' to log out.\n", apiUrl)
				return nil
			}
			fmt.Printf("Already logged in.\n%sRun 'futrou logout' to log out.\n", current.text())
			return nil
		}

		workspaceID := cfg.DefaultWorkspaceFor(apiUrl)
		workspaceName := ""
		if workspaceID != "" {
			token := cfg.TokenFor(apiUrl)
			if _, context, _ := checkSession(apiUrl, token); context != nil {
				workspaceName = context.workspaceName(workspaceID)
			}
			if workspaceName == "" {
				workspaceName = lookupWorkspaceName(apiUrl, token, workspaceID)
			}
		}

		if isJSON(c) {
			return printJSON(map[string]string{
				"email":         email,
				"status":        "logged in",
				"workspace":     workspaceID,
				"workspaceName": workspaceName,
			})
		}
		fmt.Print(loggedInText(email, workspaceID, workspaceName))
		return nil
	},
}

// loggedInText is what a finished login prints: who, and the default workspace by
// its name (its ID only when the name could not be read).
func loggedInText(email, workspaceID, workspaceName string) string {
	text := "✓ Logged in successfully\n"
	if email != "" {
		text = "✓ Logged in as " + email + "\n"
	}
	switch {
	case workspaceName != "":
		text += "✓ Default workspace set to " + workspaceName + "\n"
	case workspaceID != "":
		text += "✓ Default workspace set to " + workspaceID + "\n"
	}
	return text
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
