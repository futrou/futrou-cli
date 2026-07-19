package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

var serverletsCommand = &cli.Command{
	Name:  "serverlets",
	Usage: "Manage serverlets",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all serverlets",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "image", "state", "instances", "minInstances", "maxInstances", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a serverlet by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id, nil, &result)
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
			Usage: "Create a new serverlet",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Serverlet name"},
				&cli.StringFlag{Name: "image", Required: true, Usage: "Container image"},
				&cli.StringFlag{Name: "plan", Usage: "Serverlet plan ID"},
				&cli.IntFlag{Name: "min", Value: 1, Usage: "Minimum instances"},
				&cli.IntFlag{Name: "max", Value: 1, Usage: "Maximum instances"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":         c.String("name"),
					"image":        c.String("image"),
					"minInstances": c.Int("min"),
					"maxInstances": c.Int("max"),
				}
				if p := c.String("plan"); p != "" {
					body["serverletPlanId"] = p
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/serverlets", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Serverlet created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a serverlet",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "New name"},
				&cli.StringFlag{Name: "image", Usage: "New image"},
				&cli.IntFlag{Name: "min", Usage: "Minimum instances"},
				&cli.IntFlag{Name: "max", Usage: "Maximum instances"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if v := c.String("image"); v != "" {
					body["image"] = v
				}
				if c.IsSet("min") {
					body["minInstances"] = c.Int("min")
				}
				if c.IsSet("max") {
					body["maxInstances"] = c.Int("max")
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/serverlets/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Serverlet updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a serverlet",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/serverlets/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Serverlet deleted")
				return nil
			},
		},
		{
			Name:      "start",
			Usage:     "Start a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("start"),
		},
		{
			Name:      "stop",
			Usage:     "Stop a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("stop"),
		},
		{
			Name:      "restart",
			Usage:     "Restart a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("restart"),
		},
		{
			Name:      "logs",
			Usage:     "View serverlet logs",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id+"/logs", nil, &result)
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
			Name:      "instances",
			Usage:     "List serverlet instances",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id+"/instances", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "state", "cpu", "ram", "createdAt"})
				return nil
			},
		},
	},
}

func serverletAction(action string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("serverlet ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		status, err := client.RequestInto("POST", "/v2/serverlets/"+id+"/"+action, nil, nil)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if isJSON(c) {
			return printJSON(map[string]string{"status": action})
		}
		fmt.Printf("✓ Serverlet %s\n", action)
		return nil
	}
}

// printTable prints a slice of maps as an aligned table with the given columns.
func printTable(data interface{}, cols []string) {
	raw, err := json.Marshal(data)
	if err != nil {
		fmt.Println(data)
		return
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		// maybe it's wrapped in a data field
		var wrapped struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil || len(wrapped.Data) == 0 {
			fmt.Println(string(raw))
			return
		}
		rows = wrapped.Data
	}

	if len(rows) == 0 {
		fmt.Println("No results.")
		return
	}

	// calculate column widths
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, col := range cols {
			v := fmt.Sprintf("%v", row[col])
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}

	// header
	for i, col := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.ToUpper(col))
		fmt.Print(strings.Repeat(" ", widths[i]-len(col)))
	}
	fmt.Println()

	// separator
	for i, col := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.Repeat("-", widths[i]+len(col)-len(col)))
		_ = col
		fmt.Print(strings.Repeat("-", widths[i]))
	}
	fmt.Println()

	// rows
	for _, row := range rows {
		for i, col := range cols {
			if i > 0 {
				fmt.Print("  ")
			}
			v := fmt.Sprintf("%v", row[col])
			fmt.Print(v)
			fmt.Print(strings.Repeat(" ", widths[i]-len(v)))
		}
		fmt.Println()
	}
}
