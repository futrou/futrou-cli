package commands

import (
	"fmt"

	"futrou-cli/src/config"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var logoutCommand = &cli.Command{
	Name:  "logout",
	Usage: "Log out from Futrou Cloud on this machine",
	Action: func(c *cli.Context) error {
		apiUrl := services.NormalizeApiUrl(globalApiUrl(c))
		cfg, err := config.Load()
		loggedIn := err == nil && cfg.TokenFor(apiUrl) != ""

		if err := config.Delete(); err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}

		if !loggedIn {
			if isJSON(c) {
				return printJSON(map[string]string{"status": "not logged in"})
			}
			fmt.Println("Not logged in")
			return nil
		}

		if isJSON(c) {
			return printJSON(map[string]string{"status": "logged out"})
		}
		fmt.Println("✓ Logged out")
		return nil
	},
}
