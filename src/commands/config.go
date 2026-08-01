package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"futrou-cli/src/cliconfig"
	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/constants"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var configCommand = &cli.Command{
	Name:  "config",
	Usage: "Print the loaded Futrou project configuration as JSON",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Config file path (default: auto-detect)",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, _, err := projectconfig.LoadConfig(".", c.String("file"))
		if err != nil {
			if errors.Is(err, projectconfig.ErrNoFutrouConfig) {
				cfg = &projectconfig.Config{}
			} else {
				return err
			}
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding config: %w", err)
		}
		_, err = fmt.Fprintln(os.Stdout, string(data))
		return err
	},
	Subcommands: []*cli.Command{configInitCommand, configPullCommand, configSchemaCommand},
}

var configInitCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a new Futrou project config with the default project name",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Output config file path",
			Value:   "futrou.json",
		},
		&cli.StringFlag{
			Name:  "project",
			Usage: "Project name (default: current directory name)",
		},
	},
	Action: func(c *cli.Context) error {
		file := c.String("file")
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(".", path)
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", file)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", file, err)
		}

		project := c.String("project")
		if project == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}
			project = filepath.Base(cwd)
		}
		writtenPath, err := projectconfig.SaveConfig(".", file, &projectconfig.Config{Schema: constants.ProjectConfigSchemaURL, Project: project})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "Created %s\n", writtenPath)
		return err
	},
}

var configPullCommand = &cli.Command{
	Name:  "pull",
	Usage: "Fetch a cloud project and write it as a Futrou project config",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "project",
			Aliases: []string{"p"},
			Usage:   "Cloud project name or ID (defaults to local config or default)",
		},
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Output config file path",
			Value:   "futrou.json",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Overwrite an existing output file",
		},
	},
	Action: func(c *cli.Context) error {
		file := c.String("file")
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(".", path)
		}
		cfg := &projectconfig.Config{}
		loadedLocalConfig := false
		if loaded, loadedPath, err := projectconfig.LoadConfig(".", ""); err == nil {
			cfg = loaded
			loadedLocalConfig = filepath.Clean(loadedPath) == filepath.Clean(path)
		} else if !errors.Is(err, projectconfig.ErrNoFutrouConfig) {
			return err
		}
		if _, err := os.Stat(path); err == nil && !c.Bool("force") && !loadedLocalConfig {
			return fmt.Errorf("%s already exists (use --force to overwrite it)", file)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", file, err)
		}

		client, err := services.NewApiClient(globalApiUrl(c), globalApiKey(c))
		if err != nil {
			return fmt.Errorf("loading credentials: %w", err)
		}
		project := c.String("project")
		if cfg.Workspace == "" {
			if cliConfig, err := cliconfig.Load(); err == nil {
				cfg.Workspace = cliConfig.DefaultWorkspaceFor(globalApiUrl(c))
			}
		}
		if project == "" {
			project = cfg.Project
		}
		if project == "" {
			project = "default"
		}
		if err := cfg.Pull(client, project); err != nil {
			return err
		}
		writtenPath, err := projectconfig.SaveConfig(".", file, cfg)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "Pulled project %s into %s\n", cfg.Project, writtenPath)
		return err
	},
}

var configSchemaCommand = &cli.Command{
	Name:  "schema",
	Usage: "Print the Futrou project config JSON Schema",
	Action: func(c *cli.Context) error {
		schema := (&projectconfig.Config{}).ToJSONSchema()
		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding config schema: %w", err)
		}
		_, err = fmt.Fprintln(os.Stdout, string(data))
		return err
	},
}
