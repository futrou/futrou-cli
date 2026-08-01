package commands

import (
	"errors"
	"fmt"
	"path/filepath"

	"futrou-cli/src/cliconfig"
	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// requireAuth returns an error with a clear message when no API key is configured.
func requireAuth(c *cli.Context) (*services.ApiClient, error) {
	apiUrl := globalApiUrl(c)
	apiKey := globalApiKey(c)

	client, err := services.NewApiClient(apiUrl, apiKey)
	if err != nil {
		return nil, err
	}

	if client.ApiToken() == "" {
		return nil, fmt.Errorf("not logged in — run 'futrou login' or set FUTROU_API_TOKEN")
	}
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
