package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var proxiesCommand = &cli.Command{
	Name:  "proxies",
	Usage: "Manage HTTPs/TCP/UDP proxies",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all proxies",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/proxies", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "domain", "type", "target", "port", "status", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a proxy by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/proxies/"+id, nil, &result)
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
			Usage: "Create a new proxy",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "domain", Required: true, Usage: "Domain name"},
				&cli.StringFlag{Name: "target", Required: true, Usage: "Target host"},
				&cli.IntFlag{Name: "port", Value: 80, Usage: "Target port"},
				&cli.StringFlag{Name: "type", Value: "http", Usage: "Proxy type (http, tcp, udp)"},
				&cli.StringFlag{Name: "strategy", Usage: "Load balancing strategy"},
				&cli.BoolFlag{Name: "https", Usage: "Enforce HTTPS"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"domain": c.String("domain"),
					"target": c.String("target"),
					"port":   c.Int("port"),
					"type":   c.String("type"),
				}
				if c.Bool("https") {
					body["enforceHttps"] = true
				}
				if s := c.String("strategy"); s != "" {
					body["strategy"] = s
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/proxies", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Proxy created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a proxy",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "domain", Usage: "Domain name"},
				&cli.StringFlag{Name: "target", Usage: "Target host"},
				&cli.IntFlag{Name: "port", Usage: "Target port"},
				&cli.StringFlag{Name: "type", Usage: "Proxy type"},
				&cli.BoolFlag{Name: "https", Usage: "Enforce HTTPS"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("domain"); v != "" {
					body["domain"] = v
				}
				if v := c.String("target"); v != "" {
					body["target"] = v
				}
				if c.IsSet("port") {
					body["port"] = c.Int("port")
				}
				if v := c.String("type"); v != "" {
					body["type"] = v
				}
				if c.IsSet("https") {
					body["enforceHttps"] = c.Bool("https")
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/proxies/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Proxy updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a proxy",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/proxies/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Proxy deleted")
				return nil
			},
		},
		{
			Name:      "purge",
			Usage:     "Purge cached responses for a proxy",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("POST", "/v2/proxies/"+id+"/purge", nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "purged"})
				}
				fmt.Println("✓ Proxy cache purged")
				return nil
			},
		},
		{
			Name:      "metrics",
			Usage:     "Get metrics for a proxy",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/proxies/"+id+"/metrics", nil, &result)
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
			Name:      "logs",
			Usage:     "View logs for a proxy",
			ArgsUsage: "<id>",
			Flags:     logFlags,
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("proxy ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/proxies/"+id+"/logs"+logQueryString(c), nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
			Subcommands: []*cli.Command{
				{
					Name:      "tail",
					Usage:     "View recent logs for a proxy",
					ArgsUsage: "<id>",
					Flags:     logFlags,
					Action: func(c *cli.Context) error {
						id := c.Args().First()
						if id == "" {
							return fmt.Errorf("proxy ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						status, err := client.RequestInto("GET", "/v2/proxies/"+id+"/logs/tail"+logQueryString(c), nil, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						return printJSON(result)
					},
				},
			},
		},
	},
}
