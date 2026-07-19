package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

var setupCommand = &cli.Command{
	Name:  "setup",
	Usage: "Configure AI coding agents to work with Futrou",
	Subcommands: []*cli.Command{
		{
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
		},
		{
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
		},
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
