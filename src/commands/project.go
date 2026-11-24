package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var projectCommand = &cli.Command{
	Name:  "project",
	Usage: "Manage projects",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List the projects of a workspace (defaults to futrou.json|yaml, then the login workspace)",
			Flags: []cli.Flag{workspaceFlag},
			Action: func(c *cli.Context) error {
				query, err := workspaceQuery(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				_, err = client.RequestInto("GET", "/v2/projects"+query, nil, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "displayName", "workspaceId", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a project by ID",
			ArgsUsage: "<id>",
			Action:    showAction("project", "/v2/projects"),
		},
		{
			Name:  "create",
			Usage: "Create a project",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Project name (slug)"},
				&cli.StringFlag{Name: "display-name", Usage: "Display name"},
				workspaceFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, err := resolveWorkspaceID(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":        c.String("name"),
					"workspaceId": workspaceID,
				}
				if v := c.String("display-name"); v != "" {
					body["displayName"] = v
				}
				var result interface{}
				_, err = client.RequestInto("POST", "/v2/projects", body, &result)
				if err != nil {
					return err
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
				_, err = client.RequestInto("PATCH", "/v2/projects/"+id, body, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Project updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a project",
			ArgsUsage: "<id>",
			Action:    rmAction("project", "/v2/projects"),
		},
	},
}
