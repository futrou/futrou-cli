package commands

import (
	"os"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"

	"github.com/urfave/cli/v2"
)

func Execute() {
	app := newApp()
	if err := app.Run(os.Args); err != nil {
		logger.Fatal("%v", err)
	}
}

// globalApiUrl returns the --api-url value walking up to the root context.
func globalApiUrl(c *cli.Context) string {
	if v := c.String("api-url"); v != "" {
		return v
	}
	if c.Lineage() != nil {
		for _, parent := range c.Lineage() {
			if v := parent.String("api-url"); v != "" {
				return v
			}
		}
	}
	return constants.DefaultApiUrl
}

// globalApiKey returns the --api-token value (or its hidden --api-key
// alias) walking up to the root context.
func globalApiKey(c *cli.Context) string {
	for _, ctx := range append([]*cli.Context{c}, c.Lineage()...) {
		if v := ctx.String("api-token"); v != "" {
			return v
		}
		if v := ctx.String("api-key"); v != "" {
			return v
		}
	}
	return ""
}

func newApp() *cli.App {
	app := buildApp()
	setHelpTemplate(app)
	return app
}

func buildApp() *cli.App {
	return &cli.App{
		Name:                 constants.Name,
		Version:              constants.Version,
		Usage:                constants.Description,
		EnableBashCompletion: true,
		HideVersion:          true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "Output format: text or json",
				Value:   constants.DefaultLogFormat,
				EnvVars: []string{constants.EnvLogFormat},
				Hidden:  true,
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Log level: debug, info, warn, error",
				Value:   constants.DefaultLogLevel,
				EnvVars: []string{constants.EnvLogLevel},
				Hidden:  true,
			},
			&cli.StringFlag{
				Name:    "api-url",
				Usage:   "Futrou API URL",
				Value:   constants.DefaultApiUrl,
				EnvVars: []string{constants.EnvApiUrl},
			},
			&cli.StringFlag{
				Name:    "api-token",
				Usage:   "Futrou API token (overrides stored credentials)",
				EnvVars: []string{constants.EnvApiToken},
			},
			&cli.StringFlag{
				Name:    "api-key",
				Usage:   "Futrou API token (overrides stored credentials)",
				EnvVars: []string{constants.EnvApiToken},
				Hidden:  true,
			},
			&cli.BoolFlag{
				Name:   "version",
				Hidden: true,
			},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("version") {
				return versionCommand.Action(c)
			}
			return cli.ShowAppHelp(c)
		},
		Commands: []*cli.Command{
			loginCommand,
			logoutCommand,
			whoamiCommand,
			setupCommand,
			initCommand,
			deployCommand,
			upgradeCommand,
			serverletsCommand,
			proxiesCommand,
			dnsCommand,
			projectsCommand,
			workspacesCommand,
			volumesCommand,
			licenseCommand,
			versionCommand,
			schemaCommand,
		},
		Before: func(c *cli.Context) error {
			logger.SetLogLevel(c.String("log-level"))
			logger.SetLogFormat(c.String("log-format"))
			return nil
		},
		ExitErrHandler: func(c *cli.Context, err error) {
			if err == nil {
				return
			}
			if isJSON(c) {
				if je, ok := err.(*jsonError); ok {
					writeJSONError(os.Stderr, je.Status, je.Body)
					os.Exit(1)
					return
				}
				writeJSONError(os.Stderr, 1, err.Error())
				os.Exit(1)
				return
			}
			cli.HandleExitCoder(err)
		},
	}
}
