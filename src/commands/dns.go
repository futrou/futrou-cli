package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var dnsCommand = &cli.Command{
	Name:  "dns",
	Usage: "Manage DNS zones and records",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List DNS zones in a project (defaults to futrou.json|yaml, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{workspaceFlag, projectFlag},
			Action: func(c *cli.Context) error {
				query, err := projectQuery(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				_, err = client.RequestInto("GET", "/v2/dns"+query, nil, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "domain", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a DNS zone by ID",
			ArgsUsage: "<id>",
			Action:    showAction("DNS zone", "/v2/dns"),
		},
		{
			Name:  "create",
			Usage: "Create a DNS zone (defaults to futrou.json|yaml, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Zone domain name"},
				workspaceFlag,
				projectFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, projectID, err := resolveScope(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"domain":      c.String("name"),
					"workspaceId": workspaceID,
					"projectId":   projectID,
				}
				var result interface{}
				_, err = client.RequestInto("POST", "/v2/dns", body, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ DNS zone created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a DNS zone",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "Zone domain name"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("DNS zone ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				_, err = client.RequestInto("PATCH", "/v2/dns/"+id, body, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ DNS zone updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a DNS zone",
			ArgsUsage: "<id>",
			Action:    rmAction("DNS zone", "/v2/dns"),
		},
		{
			Name:      "metrics",
			Usage:     "Get metrics for a DNS zone",
			ArgsUsage: "<id>",
			Action:    metricsAction("DNS zone", "/v2/dns"),
		},
		{
			Name:      "logs",
			Usage:     "View logs for a DNS zone",
			ArgsUsage: "<id>",
			Flags:     logFlags,
			Action:    resourceLogsAction("DNS zone", "/v2/dns", logOptions{hasTail: true}),
			Subcommands: []*cli.Command{
				{
					Name:      "tail",
					Usage:     "View recent logs for a DNS zone",
					ArgsUsage: "<id>",
					Flags:     logFlags,
					Action:    resourceLogsAction("DNS zone", "/v2/dns", logOptions{hasTail: true, forceTail: true}),
				},
			},
		},
		{
			Name:  "record",
			Usage: "Manage DNS records for a zone",
			Subcommands: []*cli.Command{
				{
					Name:      "ls",
					Usage:     "List records in a DNS zone",
					ArgsUsage: "<zone-id>",
					Action: func(c *cli.Context) error {
						zoneId := c.Args().First()
						if zoneId == "" {
							return fmt.Errorf("DNS zone ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						_, err = client.RequestInto("GET", "/v2/dns/"+zoneId+"/records", nil, &result)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(result)
						}
						printTable(result, []string{"id", "name", "type", "value", "ttl", "priority"})
						return nil
					},
				},
				{
					Name:      "show",
					Usage:     "Show a DNS record",
					ArgsUsage: "<zone-id> <record-id>",
					Action: func(c *cli.Context) error {
						zoneId := c.Args().Get(0)
						recordId := c.Args().Get(1)
						if zoneId == "" || recordId == "" {
							return fmt.Errorf("zone ID and record ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						_, err = client.RequestInto("GET", "/v2/dns/"+zoneId+"/records/"+recordId, nil, &result)
						if err != nil {
							return err
						}
						return printJSON(result)
					},
				},
				{
					Name:      "create",
					Usage:     "Create a DNS record",
					ArgsUsage: "<zone-id>",
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "name", Required: true, Usage: "Record name (e.g. www or @)"},
						&cli.StringFlag{Name: "type", Required: true, Usage: "Record type (A, AAAA, CNAME, MX, TXT, ...)"},
						&cli.StringFlag{Name: "value", Required: true, Usage: "Record value"},
						&cli.IntFlag{Name: "ttl", Value: 300, Usage: "TTL in seconds"},
						&cli.IntFlag{Name: "priority", Usage: "Priority (MX, SRV records)"},
					},
					Action: func(c *cli.Context) error {
						zoneId := c.Args().First()
						if zoneId == "" {
							return fmt.Errorf("DNS zone ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						body := map[string]interface{}{
							"name":  c.String("name"),
							"type":  c.String("type"),
							"value": c.String("value"),
							"ttl":   c.Int("ttl"),
						}
						if c.IsSet("priority") {
							body["priority"] = c.Int("priority")
						}
						var result interface{}
						_, err = client.RequestInto("POST", "/v2/dns/"+zoneId+"/records", body, &result)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ DNS record created")
						printJSON(result)
						return nil
					},
				},
				{
					Name:      "update",
					Usage:     "Update a DNS record",
					ArgsUsage: "<zone-id> <record-id>",
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "name", Usage: "Record name"},
						&cli.StringFlag{Name: "type", Usage: "Record type"},
						&cli.StringFlag{Name: "value", Usage: "Record value"},
						&cli.IntFlag{Name: "ttl", Usage: "TTL in seconds"},
						&cli.IntFlag{Name: "priority", Usage: "Priority"},
					},
					Action: func(c *cli.Context) error {
						zoneId := c.Args().Get(0)
						recordId := c.Args().Get(1)
						if zoneId == "" || recordId == "" {
							return fmt.Errorf("zone ID and record ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						body := map[string]interface{}{}
						if v := c.String("name"); v != "" {
							body["name"] = v
						}
						if v := c.String("type"); v != "" {
							body["type"] = v
						}
						if v := c.String("value"); v != "" {
							body["value"] = v
						}
						if c.IsSet("ttl") {
							body["ttl"] = c.Int("ttl")
						}
						if c.IsSet("priority") {
							body["priority"] = c.Int("priority")
						}
						if len(body) == 0 {
							return fmt.Errorf("no fields to update")
						}
						var result interface{}
						_, err = client.RequestInto("PATCH", "/v2/dns/"+zoneId+"/records/"+recordId, body, &result)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ DNS record updated")
						return nil
					},
				},
				{
					Name:      "rm",
					Usage:     "Delete a DNS record",
					ArgsUsage: "<zone-id> <record-id>",
					Action: func(c *cli.Context) error {
						zoneId := c.Args().Get(0)
						recordId := c.Args().Get(1)
						if zoneId == "" || recordId == "" {
							return fmt.Errorf("zone ID and record ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						_, err = client.RequestInto("DELETE", "/v2/dns/"+zoneId+"/records/"+recordId, nil, nil)
						if err != nil {
							return err
						}
						if isJSON(c) {
							return printJSON(map[string]string{"status": "deleted"})
						}
						fmt.Println("✓ DNS record deleted")
						return nil
					},
				},
			},
		},
	},
}
