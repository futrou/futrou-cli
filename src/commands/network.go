package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var networkCommand = &cli.Command{
	Name:  "network",
	Usage: "Manage networks",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List all networks",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/networks", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "ipv4", "ipv6", "internal", "regionId", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a network by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				return networkRequest(c, "GET", "/v2/networks/%s", nil)
			},
		},
		{
			Name:  "create",
			Usage: "Create a network (defaults to futrou.json|yaml, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Network name"},
				&cli.StringFlag{Name: "ipv4", Usage: "IPv4 CIDR"},
				&cli.StringFlag{Name: "ipv6", Usage: "IPv6 CIDR"},
				&cli.BoolFlag{Name: "internal", Usage: "Internal-only network (no outbound access)"},
				&cli.StringFlag{Name: "region", Usage: "Region ID"},
				workspaceFlag,
				projectFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, err := resolveWorkspaceID(c)
				if err != nil {
					return err
				}
				projectID, err := resolveProjectID(c, workspaceID)
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
					"projectId":   projectID,
				}
				if v := c.String("ipv4"); v != "" {
					body["ipv4"] = v
				}
				if v := c.String("ipv6"); v != "" {
					body["ipv6"] = v
				}
				if c.IsSet("internal") {
					body["internal"] = fmt.Sprint(c.Bool("internal"))
				}
				if v := c.String("region"); v != "" {
					body["regionId"] = v
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/networks", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Network created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a network",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "New name"},
				&cli.StringFlag{Name: "ipv4", Usage: "IPv4 CIDR"},
				&cli.StringFlag{Name: "ipv6", Usage: "IPv6 CIDR"},
				&cli.BoolFlag{Name: "internal", Usage: "Internal-only network"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("network ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				for _, k := range []string{"name", "ipv4", "ipv6"} {
					if v := c.String(k); v != "" {
						body[k] = v
					}
				}
				if c.IsSet("internal") {
					body["internal"] = fmt.Sprint(c.Bool("internal"))
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/networks/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Network updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a network",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("network ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/networks/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Network deleted")
				return nil
			},
		},
		{
			Name:      "devices",
			Usage:     "List devices attached to a network",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				return networkRequest(c, "GET", "/v2/networks/%s/devices", nil)
			},
		},
		{
			Name:      "metrics",
			Usage:     "Show network metrics",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				return networkRequest(c, "GET", "/v2/networks/%s/metrics", nil)
			},
		},
	},
}

// networkRequest runs a body-less request against a path template taking the
// network ID as its first argument and prints the JSON response.
func networkRequest(c *cli.Context, method, pathFmt string, body interface{}) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("network ID required")
	}
	client, err := requireAuth(c)
	if err != nil {
		return err
	}
	var result interface{}
	status, err := client.RequestInto(method, fmt.Sprintf(pathFmt, id), body, &result)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("request failed with status %d", status)
	}
	return printJSON(result)
}
