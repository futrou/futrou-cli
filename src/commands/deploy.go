package commands

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/deployer"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var deployCommand = &cli.Command{
	Name:  "deploy",
	Usage: "Deploy or upgrade resources based on futrou.json config",
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
			Usage: "Destroy the resources declared in the project configuration",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, cfgFile, err := projectconfig.LoadConfig(".", c.String("file"))
		if err != nil {
			return err
		}
		fmt.Printf("Using config: %s\n", cfgFile)

		client, err := services.NewApiClient(globalApiUrl(c), globalApiKey(c))
		if err != nil {
			return fmt.Errorf("loading credentials: %w", err)
		}
		if cfg.Project != "" {
			if err := ensureConfigWorkspace(c, client, cfg); err != nil {
				return err
			}
		}

		plan, err := deployer.BuildPlan(client, cfg, c.Bool("destroy"))
		if err != nil {
			return err
		}
		if len(plan.Actions) == 0 {
			refreshDeploymentConfig(client, cfg, cfgFile)
			fmt.Printf("%s✓ No changes. Infrastructure is up to date.%s\n", colorGreen, colorReset)
			return nil
		}
		printDeployPlan(plan)
		if !c.Bool("yes") && !promptConfirm("Apply these changes?") {
			fmt.Println("Cancelled.")
			return nil
		}
		if err := deployer.Apply(client, plan); err != nil {
			return err
		}
		// Refresh successful deployments so the file records the API IDs needed
		// to recognize future resource renames as updates.
		refreshDeploymentConfig(client, cfg, cfgFile)
		if isJSON(c) {
			return printJSON(plan)
		}
		fmt.Printf("%s✓ Applied %d change(s).%s\n", colorGreen, len(plan.Actions), colorReset)
		return nil
	},
}

func refreshDeploymentConfig(client *services.ApiClient, cfg *projectconfig.Config, path string) {
	if cfg.Project == "" || !strings.HasSuffix(path, ".json") {
		return
	}
	if err := cfg.Pull(client, ""); err != nil {
		logger.Warn("deployment succeeded but config locks were not refreshed: %v", err)
		return
	}
	if _, err := cfg.Save(".", path); err != nil {
		logger.Warn("deployment succeeded but config locks were not refreshed: %v", err)
	}
}

func printDeployPlan(plan *deployer.Plan) {
	fmt.Println("\nChanges:")
	actions := deployer.SortedActions(plan)
	printPlanSection("Create", colorGreen, actions, deployer.Create)
	printPlanSection("Update", colorYellow, actions, deployer.Update)
	printPlanSection("Delete", colorRed, actions, deployer.Delete)
	fmt.Println()
}

func printPlanSection(title, color string, actions []deployer.Action, kind deployer.ActionType) {
	count := 0
	for _, action := range actions {
		if action.Type == kind {
			count++
		}
	}
	if count == 0 {
		return
	}
	fmt.Printf("\n%s%s (%d)%s\n", color, title, count, colorReset)
	printedZones := map[string]bool{}
	for _, action := range actions {
		if action.Type != kind {
			continue
		}
		if action.Resource == "dns record" {
			printDNSRecordAction(action, printedZones)
			continue
		}
		switch kind {
		case deployer.Create:
			printAdded(action.Resource, action.Name)
			printActionPayload(action)
		case deployer.Delete:
			printRemoved(action.Resource, action.Name)
		case deployer.Update:
			printUpdatedResource(action.Resource, action.Name)
			printActionFieldChanges(action)
		}
	}
}

func printDNSRecordAction(action deployer.Action, printedZones map[string]bool) {
	parts := strings.SplitN(action.Name, " / ", 2)
	zone, record := action.Name, action.Name
	if len(parts) == 2 {
		zone, record = parts[0], parts[1]
	}
	if !printedZones[zone] {
		switch action.Type {
		case deployer.Create:
			printAdded("dns zone", zone)
		case deployer.Delete:
			printRemoved("dns zone", zone)
		default:
			printUpdatedResource("dns zone", zone)
		}
		printedZones[zone] = true
	}
	label := "  dns record"
	switch action.Type {
	case deployer.Create:
		printAdded(label, record)
		printActionPayload(action)
	case deployer.Update:
		printUpdatedResource(label, record)
		printActionFieldChanges(action)
	case deployer.Delete:
		printRemoved(label, record)
	}
}

func printActionPayload(action deployer.Action) {
	keys := make([]string, 0, len(action.Payload))
	for key := range action.Payload {
		if key != "name" && key != "domain" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		printAdded("  "+key, fmt.Sprint(action.Payload[key]))
	}
}

func printUpdatedResource(resource, name string) {
	fmt.Printf("  %s~ %s: %q%s\n", colorYellow, resource, name, colorReset)
}

func printActionFieldChanges(action deployer.Action) {
	keys := make([]string, 0, len(action.Changes))
	for key := range action.Changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		printChanged("  "+key, fmt.Sprint(action.Previous[key]), fmt.Sprint(action.Changes[key]))
	}
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
	colorYellow = "\033[34m"
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
