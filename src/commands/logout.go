package commands

import (
	"fmt"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/logger"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

var logoutCommand = &cli.Command{
	Name:  "logout",
	Usage: "Log out from Futrou Cloud: destroys the token at the API and removes it from this machine",
	Action: func(c *cli.Context) error {
		apiUrl := utils.NormalizeApiUrl(globalApiUrl(c))
		cfg, err := cliconfig.Load()
		if err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}

		wasLoggedIn, revokeErr, err := cfg.Logout(apiUrl)
		if err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}

		if !wasLoggedIn {
			if isJSON(c) {
				return printJSON(map[string]string{"status": "not logged in"})
			}
			fmt.Println("Not logged in")
			return nil
		}

		if revokeErr != nil {
			logger.Warn("the token could not be destroyed at %s (%v): it was removed from this machine only", apiUrl, revokeErr)
		}
		if isJSON(c) {
			return printJSON(map[string]interface{}{"status": "logged out", "revoked": revokeErr == nil})
		}
		fmt.Println("✓ Logged out")
		return nil
	},
}
