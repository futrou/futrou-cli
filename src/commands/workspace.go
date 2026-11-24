package commands

import (
	"fmt"
	"net/url"

	"futrou-cli/src/cliconfig"
	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

// firstSet is the value of the first of the string flags that is set: the
// current name of a flag, then its older, hidden spellings.
func firstSet(c *cli.Context, names ...string) string {
	for _, name := range names {
		if v := c.String(name); v != "" {
			return v
		}
	}
	return ""
}

// workspaceFlag and projectFlag are shared by every command that needs to
// resolve a workspace (and, where relevant, a project within it).
var workspaceFlag = &cli.StringFlag{Name: "workspace", Usage: "Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)"}
var projectFlag = &cli.StringFlag{Name: "project", Usage: "Project name or ID within the workspace (defaults to futrou.json|yaml, then \"default\")"}

// resolveWorkspaceID resolves --workspace to a workspace ID. A UUID is used
// directly; a name is looked up. If the flag is empty, it falls back to the
// workspace declared in the project's futrou.json (if one exists in the
// current directory), then to the default workspace stored at login.
func resolveWorkspaceID(c *cli.Context) (string, error) {
	name := c.String("workspace")
	if name == "" {
		if cfg, _, err := projectconfig.LoadConfig(".", ""); err == nil {
			name = cfg.Workspace
		}
	}
	if name == "" {
		apiUrl := utils.NormalizeApiUrl(globalApiUrl(c))
		if cfg, err := cliconfig.Load(); err == nil {
			name = cfg.DefaultWorkspaceFor(apiUrl)
		}
		if name == "" {
			return defaultWorkspaceFromAccount(c)
		}
	}
	if utils.IsUuid(name) {
		return name, nil
	}

	client, err := requireAuth(c)
	if err != nil {
		return "", err
	}

	path := "/v2/workspaces?" + url.Values{"name": {name}}.Encode()
	var workspaces []map[string]interface{}
	_, err = client.RequestInto("GET", path, nil, &workspaces)
	if err != nil {
		return "", err
	}
	if len(workspaces) == 0 {
		return "", fmt.Errorf("no workspace named %q found", name)
	}

	id, _ := workspaces[0]["id"].(string)
	if id == "" {
		return "", fmt.Errorf("workspace response missing id")
	}
	return id, nil
}

// resolveProjectID resolves --project (within the given workspace) to a
// project ID. A UUID is used directly; a name is looked up via an exact
// match. If the flag is empty, it falls back to the project declared in the
// current directory's futrou.json, then to a project named "default".
func resolveProjectID(c *cli.Context, workspaceID string) (string, error) {
	name := c.String("project")
	if name == "" {
		if cfg, _, err := projectconfig.LoadConfig(".", ""); err == nil {
			name = cfg.Project
		}
	}
	if name == "" {
		name = "default"
	}
	if utils.IsUuid(name) {
		return name, nil
	}

	client, err := requireAuth(c)
	if err != nil {
		return "", err
	}

	path := "/v2/workspaces/" + workspaceID + "/projects?" + url.Values{"name": {name}}.Encode()
	var projects []map[string]interface{}
	_, err = client.RequestInto("GET", path, nil, &projects)
	if err != nil {
		return "", err
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

var workspaceCommand = &cli.Command{
	Name:  "workspace",
	Usage: "Manage workspaces",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List all workspaces",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				_, err = client.RequestInto("GET", "/v2/workspaces", nil, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "displayName", "currency", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a workspace by ID",
			ArgsUsage: "<id>",
			Action:    showAction("workspace", "/v2/workspaces"),
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
				_, err = client.RequestInto("PATCH", "/v2/workspaces/"+id, body, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Workspace updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a workspace",
			ArgsUsage: "<id>",
			Action:    rmAction("workspace", "/v2/workspaces"),
		},
		{
			Name:  "limit",
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
				_, err = client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/limits", nil, &result)
				if err != nil {
					return err
				}
				return printJSON(result)
			},
		},
		{
			Name:  "contact",
			Usage: "Manage workspace billing contact",
			Subcommands: []*cli.Command{
				{
					Name:  "show",
					Usage: "Show the workspace billing contact",
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
						_, err = client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/contact", nil, &result)
						if err != nil {
							return err
						}
						return printJSON(result)
					},
				},
				{
					Name:  "update",
					Usage: "Update the workspace billing contact",
					Flags: []cli.Flag{
						workspaceFlag,
						&cli.StringFlag{Name: "first-name", Usage: "First name"},
						&cli.StringFlag{Name: "last-name", Usage: "Last name"},
						&cli.StringFlag{Name: "firstname", Hidden: true, Usage: "Same as --first-name"},
						&cli.StringFlag{Name: "lastname", Hidden: true, Usage: "Same as --last-name"},
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
						if v := firstSet(c, "first-name", "firstname"); v != "" {
							body["firstname"] = v
						}
						if v := firstSet(c, "last-name", "lastname"); v != "" {
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
						_, err = client.RequestInto("PATCH", "/v2/workspaces/"+workspaceID+"/contact", body, &result)
						if err != nil {
							return err
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
			Name:  "user",
			Usage: "Manage users in a workspace",
			Subcommands: []*cli.Command{
				{
					Name:  "ls",
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
						_, err = client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/users", nil, &result)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(result)
						}
						printTable(result, []string{"id", "userId", "role", "createdAt"})
						return nil
					},
				},
				{
					Name:      "show",
					Usage:     "Show a user in a workspace",
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
						_, err = client.RequestInto("GET", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, nil, &result)
						if err != nil {
							return err
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
						_, err = client.RequestInto("POST", "/v2/workspaces/"+workspaceID+"/users", body, &result)
						if err != nil {
							return err
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
						_, err = client.RequestInto("PATCH", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, body, &result)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ User role updated")
						return nil
					},
				},
				{
					Name:      "rm",
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
						_, err = client.RequestInto("DELETE", "/v2/workspaces/"+workspaceID+"/users/"+workspaceUserID, nil, nil)
						if err != nil {
							return err
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

// defaultWorkspaceFromAccount is the workspace to use when nothing names one and
// no default is stored (a login from before defaults, or an API token): the
// oldest workspace of the account, else the one the token belongs to. It is
// remembered as the default for the next command.
func defaultWorkspaceFromAccount(c *cli.Context) (string, error) {
	client, err := requireAuth(c)
	if err != nil {
		return "", err
	}
	cfg, err := cliconfig.Load()
	if err != nil {
		return "", err
	}
	apiUrl := utils.NormalizeApiUrl(globalApiUrl(c))
	id, err := cfg.EnsureDefaultWorkspace(apiUrl, client.ApiToken())
	if id == "" {
		if err != nil {
			return "", fmt.Errorf("no workspace specified and none could be found (%w) — pass --workspace or add \"workspace\" to futrou.json", err)
		}
		return "", fmt.Errorf("no workspace specified — pass --workspace, add \"workspace\" to futrou.json, or run 'futrou login'")
	}
	if globalApiToken(c) == "" { // a login's default is kept; a token given for one command is not
		_ = cliconfig.Save(cfg)
	}
	return id, nil
}

// projectQuery is the query string that limits a listing to the project selected by
// --workspace/--project, else futrou.json, else the login's default workspace and
// its "default" project.
func projectQuery(c *cli.Context) (string, error) {
	workspaceID, projectID, err := resolveScope(c)
	if err != nil {
		return "", err
	}
	return "?" + url.Values{"workspaceId": {workspaceID}, "projectId": {projectID}}.Encode(), nil
}

// workspaceQuery is projectQuery for resources that belong to a workspace, not a
// project (servers, projects): only the workspace is selected.
func workspaceQuery(c *cli.Context) (string, error) {
	workspaceID, err := resolveWorkspaceID(c)
	if err != nil {
		return "", err
	}
	return "?" + url.Values{"workspaceId": {workspaceID}}.Encode(), nil
}

// resolveScope is the workspace and project a project-scoped command works in:
// --workspace/--project, else futrou.json, else the login's default workspace and
// its "default" project.
func resolveScope(c *cli.Context) (workspaceID, projectID string, err error) {
	if workspaceID, err = resolveWorkspaceID(c); err != nil {
		return "", "", err
	}
	if projectID, err = resolveProjectID(c, workspaceID); err != nil {
		return "", "", err
	}
	return workspaceID, projectID, nil
}
