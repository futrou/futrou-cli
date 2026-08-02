package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"futrou-cli/src/services"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

var schemaCommand = &cli.Command{
	Name:  "schema",
	Usage: "Display the Futrou API v2 OpenAPI schema",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "format",
			Usage: "Output format: json or go",
			Value: "json",
		},
	},
	Action: func(c *cli.Context) error {
		client := services.NewApiClientWithToken(globalApiUrl(c), globalApiKey(c))
		switch c.String("format") {
		case "go":
			schema, err := client.ToJSONSchema()
			if err != nil {
				return err
			}
			types, err := utils.JSONSchemaToGo(schema, client.ApiUrl()+"/v2/openapi.json")
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(types)
			return err
		case "json":
			data, err := client.ToJSONSchema()
			if err != nil {
				return err
			}

			if isJSON(c) {
				// Already JSON — print raw.
				fmt.Println(string(data))
				return nil
			}

			// Pretty-print JSON schema for terminals.
			var pretty interface{}
			if err := json.Unmarshal(data, &pretty); err != nil {
				return fmt.Errorf("parsing schema: %w", err)
			}
			return printJSON(pretty)
		default:
			return fmt.Errorf("unsupported schema format %q (supported: json, go)", c.String("format"))
		}
	},
}
