package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var initCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a futrou.json config file for this project",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Output file path",
			Value:   "futrou.json",
		},
		&cli.StringFlag{Name: "name", Usage: "Serverlet name"},
		&cli.StringFlag{Name: "image", Usage: "Container image"},
	},
	Action: func(c *cli.Context) error {
		outFile := c.String("file")

		if _, err := os.Stat(outFile); err == nil {
			return fmt.Errorf("%s already exists — delete it first or use -f to specify a different path", outFile)
		}

		reader := bufio.NewReader(os.Stdin)

		// default name from current directory
		cwd, _ := os.Getwd()
		defaultName := filepath.Base(cwd)

		name := c.String("name")
		if name == "" {
			fmt.Printf("Serverlet name [%s]: ", defaultName)
			input, _ := reader.ReadString('\n')
			name = strings.TrimSpace(input)
			if name == "" {
				name = defaultName
			}
		}

		// validate or suggest: check if name is taken (if logged in)
		apiUrl := globalApiUrl(c)
		apiKey := globalApiKey(c)
		var suggestedName string
		if client, err := services.NewApiClient(apiUrl, apiKey); err == nil && client.ApiToken() != "" {
			suggestedName = checkServerletName(client, name)
		}
		if suggestedName != "" && suggestedName != name {
			fmt.Printf("Name '%s' may already be taken, suggested: %s\n", name, suggestedName)
			fmt.Printf("Use name [%s]: ", suggestedName)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "" {
				name = input
			} else {
				name = suggestedName
			}
		}

		image := c.String("image")
		if image == "" {
			fmt.Print("Container image [e.g. nginx:latest]: ")
			input, _ := reader.ReadString('\n')
			image = strings.TrimSpace(input)
		}

		cfg := map[string]interface{}{
			"name":  name,
			"image": image,
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding config: %w", err)
		}

		if err := os.WriteFile(outFile, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outFile, err)
		}

		if isJSON(c) {
			return printJSON(cfg)
		}

		fmt.Printf("✓ Created %s\n", outFile)
		fmt.Printf("\nNext steps:\n")
		fmt.Printf("  futrou deploy        — deploy this serverlet\n")
		fmt.Printf("  futrou deploy -y     — deploy without confirmation\n")
		return nil
	},
}

// checkServerletName looks up existing serverlets and returns an alternative name if taken.
func checkServerletName(client *services.ApiClient, name string) string {
	var result interface{}
	status, err := client.RequestInto("GET", "/v2/serverlets", nil, &result)
	if err != nil || status >= 400 {
		return ""
	}

	raw, _ := json.Marshal(result)
	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		var wrapped struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil {
			return ""
		}
		rows = wrapped.Data
	}

	taken := make(map[string]bool)
	for _, row := range rows {
		if n, ok := row["name"].(string); ok {
			taken[n] = true
		}
	}

	if !taken[name] {
		return ""
	}

	// suggest name-2, name-3, ...
	for i := 2; i <= 99; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !taken[candidate] {
			return candidate
		}
	}
	return ""
}
