package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var projectsCommand = &cli.Command{
	Name:  "projects",
	Usage: "Manage projects",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all projects",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/projects", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "displayName", "workspaceId", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a project by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("project ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/projects/"+id, nil, &result)
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
			Usage: "Create a project",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Project name (slug)"},
				&cli.StringFlag{Name: "display-name", Usage: "Display name"},
				&cli.StringFlag{Name: "workspace", Usage: "Workspace ID"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name": c.String("name"),
				}
				if v := c.String("display-name"); v != "" {
					body["displayName"] = v
				}
				if v := c.String("workspace"); v != "" {
					body["workspaceId"] = v
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/projects", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Project created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a project",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "Project name"},
				&cli.StringFlag{Name: "display-name", Usage: "Display name"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("project ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if v := c.String("display-name"); v != "" {
					body["displayName"] = v
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/projects/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Project updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a project",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("project ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/projects/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Project deleted")
				return nil
			},
		},
	},
}
