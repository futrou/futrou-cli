package commands

import (
	"fmt"

	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

var licenseCommand = &cli.Command{
	Name:  "license",
	Usage: "Display the Futrou CLI license",
	Action: func(c *cli.Context) error {
		if isJSON(c) {
			return printJSON(map[string]string{"license": constants.License})
		}
		fmt.Println(constants.License)
		return nil
	},
}
