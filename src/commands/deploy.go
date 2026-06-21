package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"futrou-cli/src/api"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// DeployConfig is the schema for futrou.json / futrou.config.json etc.
type DeployConfig struct {
	// Serverlet identification — one of id or name is required
	Id   string `json:"id"`
	Name string `json:"name"`

	// Serverlet fields
	Image           string            `json:"image"`
	ServerletPlanId string            `json:"serverletPlanId"`
	WorkspaceId     string            `json:"workspaceId"`
	ProjectId       string            `json:"projectId"`
	MinInstances    *int              `json:"minInstances"`
	MaxInstances    *int              `json:"maxInstances"`
	Env             map[string]string `json:"env"`
}

var deployCommand = &cli.Command{
	Name:  "deploy",
	Usage: "Deploy resources defined in futrou.json",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Config file (default: auto-detect futrou.json etc.)",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Auto-approve without prompting",
		},
		&cli.BoolFlag{
			Name:  "destroy",
			Usage: "Destroy the resource instead of creating/updating",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, cfgFile, err := loadDeployConfig(c.String("file"))
		if err != nil {
			return err
		}
		fmt.Printf("Using config: %s\n", cfgFile)

		client, err := services.NewApiClient(globalApiUrl(c), globalApiKey(c))
		if err != nil {
			return fmt.Errorf("loading credentials: %w", err)
		}

		if c.Bool("destroy") {
			return runDestroy(c, client, cfg)
		}
		return runDeploy(c, client, cfg)
	},
}

// loadDeployConfig finds and reads the config file in priority order.
func loadDeployConfig(override string) (*DeployConfig, string, error) {
	candidates := []string{
		"futrou.json",
		"futrou.js",
		"futrou.config.json",
		"futrou.config.js",
	}
	if override != "" {
		candidates = []string{override}
	}

	for _, name := range candidates {
		data, err := readConfigFile(name)
		if err != nil {
			continue
		}
		var cfg DeployConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, "", fmt.Errorf("parsing %s: %w", name, err)
		}
		return &cfg, name, nil
	}
	return nil, "", fmt.Errorf("no config file found (looked for: %s)", strings.Join(candidates, ", "))
}

// readConfigFile reads a file; for .js files it uses node to evaluate and return JSON.
func readConfigFile(name string) ([]byte, error) {
	if _, err := os.Stat(name); err != nil {
		return nil, err
	}
	if strings.HasSuffix(name, ".js") {
		node, err := exec.LookPath("node")
		if err != nil {
			return nil, fmt.Errorf("%s requires node (not found in PATH)", name)
		}
		out, err := exec.Command(node, "-e",
			fmt.Sprintf("process.stdout.write(JSON.stringify(require('./%s')))", name),
		).Output()
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", name, err)
		}
		return out, nil
	}
	return os.ReadFile(name)
}

func runDeploy(c *cli.Context, client *services.ApiClient, cfg *DeployConfig) error {
	autoApprove := c.Bool("yes")

	// Fetch current remote state if serverlet id/name is known
	var remote *api.Serverlet
	if cfg.Id != "" {
		var s api.Serverlet
		if _, err := client.RequestInto("GET", "/v2/serverlets/"+cfg.Id, nil, &s); err == nil {
			remote = &s
		}
	} else if cfg.Name != "" {
		// Search by name
		var list []api.Serverlet
		if _, err := client.RequestInto("GET", "/v2/serverlets?limit=100", nil, &list); err == nil {
			for i, s := range list {
				if strings.EqualFold(s.Name, cfg.Name) {
					remote = &list[i]
					break
				}
			}
		}
	}

	if remote == nil {
		return runCreate(c, client, cfg, autoApprove)
	}
	return runUpdate(c, client, cfg, remote, autoApprove)
}

func runCreate(c *cli.Context, client *services.ApiClient, cfg *DeployConfig, autoApprove bool) error {
	fmt.Println("\nPlan: create serverlet")
	fmt.Println()
	printAdded("name", cfg.Name)
	printAdded("image", cfg.Image)
	if cfg.ServerletPlanId != "" {
		printAdded("serverletPlanId", cfg.ServerletPlanId)
	}
	if cfg.WorkspaceId != "" {
		printAdded("workspaceId", cfg.WorkspaceId)
	}
	if cfg.ProjectId != "" {
		printAdded("projectId", cfg.ProjectId)
	}
	fmt.Println()

	if !autoApprove && !promptConfirm("Create serverlet?") {
		fmt.Println("Cancelled.")
		return nil
	}

	payload := map[string]interface{}{
		"name":  cfg.Name,
		"image": cfg.Image,
	}
	if cfg.ServerletPlanId != "" {
		payload["serverletPlanId"] = cfg.ServerletPlanId
	}
	if cfg.WorkspaceId != "" {
		payload["workspaceId"] = cfg.WorkspaceId
	}
	if cfg.ProjectId != "" {
		payload["projectId"] = cfg.ProjectId
	}

	var created api.Serverlet
	if _, err := client.RequestInto("POST", "/v2/serverlets", payload, &created); err != nil {
		return fmt.Errorf("create failed: %w", err)
	}

	if isJSON(c) {
		return printJSON(created)
	}
	fmt.Printf("✓ Created serverlet %s (%s)\n", created.Name, created.Id)
	return nil
}

func runUpdate(c *cli.Context, client *services.ApiClient, cfg *DeployConfig, remote *api.Serverlet, autoApprove bool) error {
	changes := map[string]interface{}{}
	hasDiff := false

	fmt.Printf("\nPlan: update serverlet %s (%s)\n\n", remote.Name, remote.Id)

	if cfg.Image != "" && cfg.Image != remote.Image {
		printChanged("image", remote.Image, cfg.Image)
		changes["image"] = cfg.Image
		hasDiff = true
	}
	if cfg.Name != "" && cfg.Name != remote.Name {
		printChanged("name", remote.Name, cfg.Name)
		changes["name"] = cfg.Name
		hasDiff = true
	}
	if cfg.ServerletPlanId != "" {
		printAdded("serverletPlanId", cfg.ServerletPlanId)
		changes["serverletPlanId"] = cfg.ServerletPlanId
		hasDiff = true
	}

	fmt.Println()

	if !hasDiff {
		fmt.Println("✓ No changes. Serverlet is up to date.")
		return nil
	}

	if !autoApprove && !promptConfirm("Apply changes?") {
		fmt.Println("Cancelled.")
		return nil
	}

	var updated api.Serverlet
	if _, err := client.RequestInto("PATCH", "/v2/serverlets/"+remote.Id, changes, &updated); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if isJSON(c) {
		return printJSON(updated)
	}
	fmt.Printf("✓ Updated serverlet %s (%s)\n", updated.Name, updated.Id)
	return nil
}

func runDestroy(c *cli.Context, client *services.ApiClient, cfg *DeployConfig) error {
	id := cfg.Id
	name := cfg.Name

	// Resolve name to id if needed
	if id == "" && name != "" {
		var list []api.Serverlet
		if _, err := client.RequestInto("GET", "/v2/serverlets?limit=100", nil, &list); err != nil {
			return fmt.Errorf("fetching serverlets: %w", err)
		}
		for _, s := range list {
			if strings.EqualFold(s.Name, name) {
				id = s.Id
				break
			}
		}
		if id == "" {
			return fmt.Errorf("serverlet %q not found", name)
		}
	}
	if id == "" {
		return fmt.Errorf("serverlet id or name required in config")
	}

	fmt.Printf("\nPlan: destroy serverlet %s\n\n", id)
	printRemoved("id", id)
	fmt.Println()

	autoApprove := c.Bool("yes")
	if !autoApprove && !promptConfirm("Destroy serverlet?") {
		fmt.Println("Cancelled.")
		return nil
	}

	if _, err := client.RequestInto("DELETE", "/v2/serverlets/"+id, nil, nil); err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	if isJSON(c) {
		return printJSON(map[string]string{"status": "destroyed", "id": id})
	}
	fmt.Printf("✓ Destroyed serverlet %s\n", id)
	return nil
}

// promptConfirm asks the user yes/no and returns true for yes.
func promptConfirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

const (
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorReset  = "\033[0m"
)

func printAdded(key, val string) {
	fmt.Printf("  %s+ %s: %q%s\n", colorGreen, key, val, colorReset)
}

func printRemoved(key, val string) {
	fmt.Printf("  %s- %s: %q%s\n", colorRed, key, val, colorReset)
}

func printChanged(key, from, to string) {
	fmt.Printf("  %s~ %s: %q → %q%s\n", colorYellow, key, from, to, colorReset)
}
