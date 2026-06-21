package commands

import (
	"fmt"

	"futrou-cli/src/config"

	"github.com/urfave/cli/v2"
)

var logoutCommand = &cli.Command{
	Name:  "logout",
	Usage: "Remove stored credentials",
	Action: func(c *cli.Context) error {
		if err := config.Delete(); err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}
		if isJSON(c) {
			return printJSON(map[string]string{"status": "logged out"})
		}
		fmt.Println("✓ Logged out")
		return nil
	},
}
