package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"futrou-cli/src/cliconfig"
	projectconfig "futrou-cli/src/config"
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

		cfg, err := cliconfig.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
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

		email, alreadyLoggedIn, err := cfg.Login(apiUrl, c.String("workspace"), promptSelectWorkspace, openBrowserFunc, onAuthURL, loginTimeout)

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
				return printJSON(map[string]string{"status": "already logged in"})
			}
			fmt.Printf("Already logged in to %s.\nRun 'futrou logout' to log out.\n", apiUrl)
			return nil
		}

		workspaceName := ""
		if id := cfg.DefaultWorkspaceFor(apiUrl); id != "" {
			workspaceName = id
		}

		if isJSON(c) {
			return printJSON(map[string]string{
				"email":     email,
				"status":    "logged in",
				"workspace": workspaceName,
			})
		}

		if email != "" {
			fmt.Printf("✓ Logged in as %s\n", email)
		} else {
			fmt.Println("✓ Logged in successfully")
		}
		if workspaceName != "" {
			fmt.Printf("✓ Default workspace set to %s\n", workspaceName)
		}
		return nil
	},
}

// promptSelectWorkspace is the interactive workspace picker injected into
// CliConfig.Login. It's the CLI-layer counterpart to the non-interactive
// --workspace flag resolution CliConfig.Login handles internally.
func promptSelectWorkspace(workspaces []projectconfig.Workspace) (id, name string, err error) {
	if !isInteractiveTerminal() {
		return "", "", nil
	}
	if len(workspaces) == 1 {
		return workspaces[0].ID, workspaces[0].Name, nil
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
	return w.ID, w.Name, nil
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
