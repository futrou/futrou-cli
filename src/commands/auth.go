package commands

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"futrou-cli/src/cliconfig"
	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// requireAuth returns an authenticated ApiClient, transparently running an
// interactive login if no token is available (CI=true short-circuits to
// projectconfig.ErrCIRequiresToken instead of attempting one).
func requireAuth(c *cli.Context) (*services.ApiClient, error) {
	apiUrl := globalApiUrl(c)
	cfg, err := cliconfig.Load()
	if err != nil {
		return nil, err
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

	token, err := cfg.EnsureLoggedIn(apiUrl, globalApiKey(c), promptSelectWorkspace, openBrowserFunc, onAuthURL, loginTimeout)

	if interactive && countdownDone != nil {
		close(stopCountdown)
		<-countdownDone
		logger.StopLoader()
	}

	if err != nil {
		return nil, err
	}

	client := services.NewApiClientWithToken(apiUrl, token)
	client.SetAfterMutation(func() error {
		if err := syncLocalProjectConfig(client); err != nil {
			logger.Warn("resource changed but local Futrou config was not refreshed: %v", err)
		}
		return nil
	})

	return client, nil
}

func ensureConfigWorkspace(c *cli.Context, client *services.ApiClient, cfg *projectconfig.Config) error {
	if cfg.Workspace != "" {
		return nil
	}
	cliConfig, err := cliconfig.Load()
	if err != nil {
		return err
	}
	id := cliConfig.DefaultWorkspaceFor(globalApiUrl(c))
	if id == "" {
		return fmt.Errorf("workspace is required")
	}
	var workspace map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/workspaces/"+id, nil, &workspace); err != nil {
		return fmt.Errorf("fetching default workspace: %w", err)
	}
	name, _ := workspace["name"].(string)
	if name == "" {
		return fmt.Errorf("default workspace %q has no name", id)
	}
	cfg.Workspace = name
	return nil
}

// syncLocalProjectConfig refreshes an existing JSON project configuration. It
// intentionally skips projects without a local config and executable configs,
// which cannot safely be rewritten as JSON.
func syncLocalProjectConfig(client *services.ApiClient) error {
	cfg, path, err := projectconfig.LoadConfig(".", "")
	if errors.Is(err, projectconfig.ErrNoFutrouConfig) {
		return nil
	}
	if err != nil {
		return err
	}
	if filepath.Ext(path) != ".json" || cfg.Project == "" {
		return nil
	}
	if err := cfg.Pull(client, ""); err != nil {
		return err
	}
	_, err = projectconfig.SaveConfig(".", path, cfg)
	return err
}
