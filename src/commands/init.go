package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"futrou-cli/src/api"
	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/constants"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// initConfigCommand creates a futrou.json for the current directory.
var initConfigCommand = &cli.Command{
	Name:  "config",
	Usage: "Configure a futrou.json|yaml config file for this project",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Output file path; .yaml and .yml files are written as YAML",
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

		// Default serverlet and project name from current directory.
		cwd, _ := os.Getwd()
		defaultName := filepath.Base(cwd)
		projectName := defaultName

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

		cfg := &projectconfig.Config{
			Schema:     constants.ProjectConfigSchemaURL,
			Project:    projectName,
			Serverlets: []projectconfig.ServerletConfig{{Serverlet: api.Serverlet{Name: name, Image: image}}},
		}
		// Save writes the format of the file name: JSON for .json, YAML for
		// .yaml/.yml.
		if _, err := cfg.Save(".", outFile); err != nil {
			return err
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

// initCommand sets Futrou up in the current project: "init config" writes
// futrou.json, "init mcp" and "init skills" configure AI coding agents. A bare
// "init" still creates the config file.
var initCommand = &cli.Command{
	Name:   "init",
	Usage:  "Set up Futrou in this project (config, mcp, skills)",
	Flags:  initConfigCommand.Flags,
	Action: initConfigCommand.Action,
	Subcommands: []*cli.Command{
		initMcpCommand,
		initSkillsCommand,
		initConfigCommand,
	},
}

// initMcpCommand configures the Futrou MCP server for AI coding agents.
var initMcpCommand = &cli.Command{
	Name:  "mcp",
	Usage: "Configure Futrou MCP for AI coding agents",
	Action: func(c *cli.Context) error {
		written := []string{}

		if path, err := writeMcpConfig(".mcp.json", "type"); err != nil {
			return err
		} else if path != "" {
			written = append(written, path)
		}

		if path, err := writeMcpConfig(filepath.Join(".cursor", "mcp.json"), ""); err != nil {
			return err
		} else if path != "" {
			written = append(written, path)
		}

		if isJSON(c) {
			return printJSON(map[string]interface{}{"written": written})
		}
		for _, p := range written {
			fmt.Printf("✓ Configured %s\n", p)
		}
		if len(written) == 0 {
			fmt.Println("Futrou MCP is already configured.")
		}
		return nil
	},
}

// initSkillsCommand installs the Futrou agent skill.
var initSkillsCommand = &cli.Command{
	Name:  "skills",
	Usage: "Configure Futrou Skills for AI coding agents",
	Action: func(c *cli.Context) error {
		if _, err := exec.LookPath("npx"); err != nil {
			return fmt.Errorf("npx is required to install skills — install Node.js and try again")
		}
		cmd := exec.Command("npx", "-y", "skills", "add", "futrou/futrou-cli", "-y")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installing skills: %w", err)
		}
		return nil
	},
}

// writeMcpConfig merges a "futrou" MCP server entry into the JSON file at
// path, creating it if missing. When typeField is non-empty, it's set to
// "http" alongside the url (Claude Code's .mcp.json shape); Cursor's
// .cursor/mcp.json omits it. Returns "" if the entry was already present.
func writeMcpConfig(path, typeField string) (string, error) {
	config := map[string]interface{}{}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	servers, _ := config["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}

	if existing, ok := servers["futrou"].(map[string]interface{}); ok {
		if existing["url"] == constants.McpUrl {
			return "", nil
		}
	}

	entry := map[string]interface{}{"url": constants.McpUrl}
	if typeField != "" {
		entry[typeField] = "http"
	}
	servers["futrou"] = entry
	config["mcpServers"] = servers

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
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
