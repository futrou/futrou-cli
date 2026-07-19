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
			Name:  "list",
			Usage: "List all DNS zones",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/dns", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a DNS zone by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("DNS zone ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/dns/"+id, nil, &result)
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
			Usage: "Create a DNS zone",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Zone domain name"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name": c.String("name"),
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/dns", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
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
				status, err := client.RequestInto("PATCH", "/v2/dns/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ DNS zone updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a DNS zone",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("DNS zone ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/dns/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ DNS zone deleted")
				return nil
			},
		},
		{
			Name:      "logs",
			Usage:     "View logs for a DNS zone",
			ArgsUsage: "<id>",
			Flags:     logFlags,
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("DNS zone ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/dns/"+id+"/logs"+logQueryString(c), nil, &result)
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
					Usage:     "View recent logs for a DNS zone",
					ArgsUsage: "<id>",
					Flags:     logFlags,
					Action: func(c *cli.Context) error {
						id := c.Args().First()
						if id == "" {
							return fmt.Errorf("DNS zone ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						var result interface{}
						status, err := client.RequestInto("GET", "/v2/dns/"+id+"/logs/tail"+logQueryString(c), nil, &result)
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
		{
			Name:  "records",
			Usage: "Manage DNS records for a zone",
			Subcommands: []*cli.Command{
				{
					Name:      "list",
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
						status, err := client.RequestInto("GET", "/v2/dns/"+zoneId+"/records", nil, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						printTable(result, []string{"id", "name", "type", "value", "ttl", "priority"})
						return nil
					},
				},
				{
					Name:      "get",
					Usage:     "Get a DNS record",
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
						status, err := client.RequestInto("GET", "/v2/dns/"+zoneId+"/records/"+recordId, nil, &result)
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
						status, err := client.RequestInto("POST", "/v2/dns/"+zoneId+"/records", body, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
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
						status, err := client.RequestInto("PATCH", "/v2/dns/"+zoneId+"/records/"+recordId, body, &result)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
						}
						if isJSON(c) {
							return printJSON(result)
						}
						fmt.Println("✓ DNS record updated")
						return nil
					},
				},
				{
					Name:      "delete",
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
						status, err := client.RequestInto("DELETE", "/v2/dns/"+zoneId+"/records/"+recordId, nil, nil)
						if err != nil {
							return err
						}
						if status >= 400 {
							return fmt.Errorf("request failed with status %d", status)
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
