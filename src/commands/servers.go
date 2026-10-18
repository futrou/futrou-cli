package commands

import (
	"fmt"
	"net/url"

	"github.com/urfave/cli/v2"
)

// serverCommand manages the servers that belong to a workspace. Servers are
// infrastructure rather than project resources, so they are intentionally not
// part of futrou.json and never touched by "futrou deploy".
var serverCommand = &cli.Command{
	Name:  "server",
	Usage: "Manage servers",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List servers",
			Flags: []cli.Flag{&cli.StringFlag{Name: "region", Usage: "Only list servers in this region ID"}},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				path := "/v2/servers"
				if v := c.String("region"); v != "" {
					path += "?regionId=" + v
				}
				var result interface{}
				status, err := client.RequestInto("GET", path, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "hostname", "ipv4", "regionId", "arch", "seenAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a server by ID",
			ArgsUsage: "<id>",
			Action:    serverAction("GET", "", ""),
		},
		{
			Name:  "create",
			Usage: "Create a server in the workspace",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Server name"},
				&cli.StringFlag{Name: "region", Usage: "Region ID"},
				&cli.StringFlag{Name: "workspace", Usage: "Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{"name": c.String("name")}
				if v := c.String("region"); v != "" {
					body["regionId"] = v
				}
				if c.IsSet("workspace") {
					workspaceID, err := resolveWorkspaceID(c)
					if err != nil {
						return err
					}
					body["workspaceId"] = workspaceID
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/servers", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Server created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a server",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "New name"},
				&cli.StringFlag{Name: "ipv4", Usage: "IPv4 address"},
				&cli.StringFlag{Name: "ipv6", Usage: "IPv6 address"},
				&cli.StringFlag{Name: "api-port", Usage: "Server API port"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("server ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				for flag, key := range map[string]string{"name": "name", "ipv4": "ipv4", "ipv6": "ipv6", "api-port": "apiPort"} {
					if v := c.String(flag); v != "" {
						body[key] = v
					}
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/servers/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Server updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a server",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("server ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/servers/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Server deleted")
				return nil
			},
		},
		{
			Name:      "metrics",
			Usage:     "Show server metrics",
			ArgsUsage: "<id>",
			Action:    serverAction("GET", "/metrics", ""),
		},
		{
			Name:      "logs",
			Usage:     "View server logs",
			ArgsUsage: "<id>",
			Flags: append([]cli.Flag{
				&cli.StringFlag{Name: "type", Usage: "Only logs of this resource type (e.g. serverlet, storage)"},
				&cli.StringFlag{Name: "id", Usage: "Only logs of the resource with this ID"},
				&cli.StringFlag{Name: "instance-id", Usage: "Only logs of the instance with this ID"},
			}, logFlags...),
			Action: func(c *cli.Context) error {
				extra := url.Values{}
				for flag, key := range map[string]string{"type": "type", "id": "id", "instance-id": "instanceId"} {
					if v := c.String(flag); v != "" {
						extra.Set(key, v)
					}
				}
				return resourceLogsAction("server", "/v2/servers", logOptions{extra: extra})(c)
			},
		},
	},
}

// serverAction runs a request against /v2/servers/<id><suffix> and prints the
// JSON response, or message when set and the response is not requested as JSON.
func serverAction(method, suffix, message string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("server ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		path := "/v2/servers/" + id + suffix
		var result interface{}
		status, err := client.RequestInto(method, path, nil, &result)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if message != "" && !isJSON(c) {
			fmt.Println(message)
			return nil
		}
		return printJSON(result)
	}
}
