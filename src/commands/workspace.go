package commands

import (
	"fmt"
	"net/url"

	"github.com/urfave/cli/v2"
)

// workspaceFlag and projectFlag are shared by every command that needs to
// resolve a workspace (and, where relevant, a project within it).
var workspaceFlag = &cli.StringFlag{Name: "workspace", Usage: "Workspace name (defaults to the first workspace)"}
var projectFlag = &cli.StringFlag{Name: "project", Usage: "Project name within the workspace"}

// resolveWorkspaceID resolves --workspace to a workspace ID. If the flag is
// empty, it lists all workspaces and picks the first one.
func resolveWorkspaceID(c *cli.Context) (string, error) {
	client, err := requireAuth(c)
	if err != nil {
		return "", err
	}

	name := c.String("workspace")
	path := "/v2/workspaces"
	if name != "" {
		path += "?" + url.Values{"name": {name}}.Encode()
	}

	var workspaces []map[string]interface{}
	status, err := client.RequestInto("GET", path, nil, &workspaces)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("listing workspaces failed with status %d", status)
	}
	if len(workspaces) == 0 {
		if name != "" {
			return "", fmt.Errorf("no workspace named %q found", name)
		}
		return "", fmt.Errorf("no workspaces found")
	}

	id, _ := workspaces[0]["id"].(string)
	if id == "" {
		return "", fmt.Errorf("workspace response missing id")
	}
	return id, nil
}

// resolveProjectID resolves --project (within the given workspace) to a
// project ID via an exact name match.
func resolveProjectID(c *cli.Context, workspaceID string) (string, error) {
	name := c.String("project")
	if name == "" {
		return "", fmt.Errorf("--project is required")
	}

	client, err := requireAuth(c)
	if err != nil {
		return "", err
	}

	path := "/v2/workspaces/" + workspaceID + "/projects?" + url.Values{"name": {name}}.Encode()
	var projects []map[string]interface{}
	status, err := client.RequestInto("GET", path, nil, &projects)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("listing projects failed with status %d", status)
	}
	if len(projects) == 0 {
		return "", fmt.Errorf("no project named %q found in this workspace", name)
	}

	id, _ := projects[0]["id"].(string)
	if id == "" {
		return "", fmt.Errorf("project response missing id")
	}
	return id, nil
}

var workspacesCommand = &cli.Command{
	Name:  "workspaces",
	Usage: "Manage workspaces",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all workspaces",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/workspaces", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "displayName", "currency", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a workspace by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("workspace ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/workspaces/"+id, nil, &result)
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
			Name:      "update",
			Usage:     "Update a workspace",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "Workspace name"},
				&cli.StringFlag{Name: "display-name", Usage: "Display name"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("workspace ID required")
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
				status, err := client.RequestInto("PATCH", "/v2/workspaces/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Workspace updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a workspace",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("workspace ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/workspaces/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Workspace deleted")
				return nil
			},
		},
		{
			Name:  "limits",
			Usage: "Get resource limits for a workspace",
			Flags: []cli.Flag{workspaceFlag},
			Action: func(c *cli.Context) error {
				workspaceID, err := resolveWorkspaceID(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/limits", nil, &result)
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
			Name:  "contact",
			Usage: "Manage workspace billing contact",
			Subcommands: []*cli.Command{
				{
					Name:  "get",
					Usage: "Get the workspace billing contact",
					Flags: []cli.Flag{workspaceFlag},
					Action: func(c *cli.Context) error {
						workspaceID, err := resolveWorkspaceID(c)
						if err != nil {
							return err
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						status, err := client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/contact", nil, &result)
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
					Name:  "update",
					Usage: "Update the workspace billing contact",
					Flags: []cli.Flag{
						workspaceFlag,
						&cli.StringFlag{Name: "firstname", Usage: "First name"},
						&cli.StringFlag{Name: "lastname", Usage: "Last name"},
						&cli.StringFlag{Name: "company", Usage: "Company name"},
						&cli.StringFlag{Name: "email", Usage: "Contact email"},
						&cli.StringFlag{Name: "phone", Usage: "Phone number"},
						&cli.StringFlag{Name: "street-address", Usage: "Street address"},
						&cli.StringFlag{Name: "city", Usage: "City"},
						&cli.StringFlag{Name: "postal-code", Usage: "Postal code"},
						&cli.StringFlag{Name: "country", Usage: "Country"},
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
						body := map[string]interface{}{}
						if v := c.String("firstname"); v != "" {
							body["firstname"] = v
						}
						if v := c.String("lastname"); v != "" {
							body["lastname"] = v
						}
						if v := c.String("company"); v != "" {
							body["company"] = v
						}
						if v := c.String("email"); v != "" {
							body["email"] = v
						}
						if v := c.String("phone"); v != "" {
							body["phone"] = v
						}
						if v := c.String("street-address"); v != "" {
							body["streetAddress"] = v
						}
						if v := c.String("city"); v != "" {
							body["city"] = v
						}
						if v := c.String("postal-code"); v != "" {
							body["postalCode"] = v
						}
						if v := c.String("country"); v != "" {
							body["country"] = v
						}
						if len(body) == 0 {
							return fmt.Errorf("no fields to update")
						}
						var result interface{}
						status, err := client.RequestInto("PATCH", "/v2/workspaces/"+workspaceID+"/contact", body, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ Contact updated")
						return nil
					},
				},
			},
		},
		{
			Name:  "users",
			Usage: "Manage users in a workspace",
			Subcommands: []*cli.Command{
				{
					Name:  "list",
					Usage: "List users in a workspace",
					Flags: []cli.Flag{workspaceFlag},
					Action: func(c *cli.Context) error {
						workspaceID, err := resolveWorkspaceID(c)
						if err != nil {
							return err
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						status, err := client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/users", nil, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						printTable(result, []string{"id", "userId", "role", "createdAt"})
						return nil
					},
				},
				{
					Name:      "get",
					Usage:     "Get a user in a workspace",
					ArgsUsage: "<workspace-user-id>",
					Flags:     []cli.Flag{workspaceFlag},
					Action: func(c *cli.Context) error {
						workspaceUserID := c.Args().First()
						if workspaceUserID == "" {
							return fmt.Errorf("workspace user ID required")
						}
						workspaceID, err := resolveWorkspaceID(c)
						if err != nil {
							return err
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						status, err := client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, nil, &result)
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
					Name:  "add",
					Usage: "Add a user to a workspace",
					Flags: []cli.Flag{
						workspaceFlag,
						&cli.StringFlag{Name: "user-id", Required: true, Usage: "User ID to add"},
						&cli.StringFlag{Name: "role", Value: "viewer", Usage: "Role: none, viewer, developer, billing_manager, administrator, owner"},
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
							"userId": c.String("user-id"),
							"role":   c.String("role"),
						}
						var result interface{}
						status, err := client.RequestInto("POST", "/v2/workspaces/"+workspaceID+"/users", body, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ User added to workspace")
						printJSON(result)
						return nil
					},
				},
				{
					Name:      "update",
					Usage:     "Update a user's role in a workspace",
					ArgsUsage: "<workspace-user-id>",
					Flags: []cli.Flag{
						workspaceFlag,
						&cli.StringFlag{Name: "role", Required: true, Usage: "Role: none, viewer, developer, billing_manager, administrator, owner"},
					},
					Action: func(c *cli.Context) error {
						workspaceUserID := c.Args().First()
						if workspaceUserID == "" {
							return fmt.Errorf("workspace user ID required")
						}
						workspaceID, err := resolveWorkspaceID(c)
						if err != nil {
							return err
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						body := map[string]interface{}{"role": c.String("role")}
						var result interface{}
						status, err := client.RequestInto("PATCH", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, body, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ User role updated")
						return nil
					},
				},
				{
					Name:      "remove",
					Usage:     "Remove a user from a workspace",
					ArgsUsage: "<workspace-user-id>",
					Flags:     []cli.Flag{workspaceFlag},
					Action: func(c *cli.Context) error {
						workspaceUserID := c.Args().First()
						if workspaceUserID == "" {
							return fmt.Errorf("workspace user ID required")
						}
						workspaceID, err := resolveWorkspaceID(c)
						if err != nil {
							return err
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						status, err := client.RequestInto("DELETE", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, nil, nil)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(map[string]string{"status": "removed"})
						}
						fmt.Println("✓ User removed from workspace")
						return nil
					},
				},
			},
		},
	},
}
