package commands

import (
	"fmt"

	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/deployer"

	"github.com/urfave/cli/v2"
)

var downCommand = &cli.Command{
	Name:  "down",
	Usage: "Deprovision every resource in the project and delete the project",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Config file (default: auto-detect futrou.json|yaml, then docker-compose.yml)",
		},
		&cli.StringFlag{
			Name:    "project",
			Aliases: []string{"p"},
			Usage:   "Project to use: reads futrou.<project>.json|yaml first (e.g. futrou.prod.json), then the default config",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Auto-approve without prompting",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Show what would be deleted and exit without deleting anything",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, cfgFile, err := projectconfig.LoadProjectConfig(".", c.String("file"), c.String("project"))
		if err != nil {
			return err
		}
		fmt.Printf("Using config: %s\n", cfgFile)
		if cfg.Project == "" {
			return fmt.Errorf("%s does not name a project; set \"project\" to tear it down", cfgFile)
		}
		if err := cfg.Validate(); err != nil {
			return err
		}

		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		// The project is going away, so there is no local config to keep in sync.
		client.SetAfterMutation(nil)
		if err := ensureConfigWorkspace(c, client, cfg); err != nil {
			return err
		}

		plan, err := deployer.BuildTeardownPlan(client, cfg)
		if err != nil {
			return err
		}
		printDeployPlan(plan)
		if c.Bool("dry-run") {
			return nil
		}
		if !confirmApply(c, fmt.Sprintf("Delete everything in project %q, including the project itself?", cfg.Project)) {
			fmt.Println("Cancelled.")
			return nil
		}
		if err := deployer.Apply(client, plan); err != nil {
			return err
		}
		if isJSON(c) {
			return printJSON(plan)
		}
		fmt.Printf("%s✓ Deleted %d resource(s) and project %q.%s\n", colorGreen, len(plan.Actions)-1, cfg.Project, colorReset)
		return nil
	},
}
