package commands

import (
	"fmt"

	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

var versionCommand = &cli.Command{
	Name:  "version",
	Usage: "Display the Futrou CLI version",
	Action: func(c *cli.Context) error {
		if isJSON(c) {
			return printJSON(map[string]string{"version": constants.Version})
		}
		fmt.Println(constants.Version)
		return nil
	},
}
