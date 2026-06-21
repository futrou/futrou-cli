package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var volumesCommand = &cli.Command{
	Name:  "volumes",
	Usage: "Manage persistent volumes",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all volumes",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/volumes", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "type", "sizeGb", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a volume by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("volume ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/volumes/"+id, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		{
			Name:  "create",
			Usage: "Create a volume",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Volume name"},
				&cli.IntFlag{Name: "size", Value: 10, Usage: "Size in GB"},
				&cli.StringFlag{Name: "type", Value: "ssd", Usage: "Volume type"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":   c.String("name"),
					"sizeGb": c.Int("size"),
					"type":   c.String("type"),
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/volumes", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Volume created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a volume",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "Volume name"},
				&cli.IntFlag{Name: "size", Usage: "Size in GB"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("volume ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if c.IsSet("size") {
					body["sizeGb"] = c.Int("size")
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/volumes/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Volume updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a volume",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("volume ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/volumes/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Volume deleted")
				return nil
			},
		},
	},
}
