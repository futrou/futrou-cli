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

// globalApiKey returns the --api-key value walking up to the root context.
func globalApiKey(c *cli.Context) string {
	if v := c.String("api-key"); v != "" {
		return v
	}
	if c.Lineage() != nil {
		for _, parent := range c.Lineage() {
			if v := parent.String("api-key"); v != "" {
				return v
			}
		}
	}
	return ""
}

func newApp() *cli.App {
	return &cli.App{
		Name:                 constants.Name,
		Version:              constants.Version,
		Usage:                constants.Description,
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "Output format: text or json",
				Value:   constants.DefaultLogFormat,
				EnvVars: []string{constants.EnvLogFormat},
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Log level: debug, info, warn, error",
				Value:   constants.DefaultLogLevel,
				EnvVars: []string{constants.EnvLogLevel},
			},
			&cli.StringFlag{
				Name:    "api-url",
				Usage:   "Futrou API URL",
				Value:   constants.DefaultApiUrl,
				EnvVars: []string{constants.EnvApiUrl},
			},
			&cli.StringFlag{
				Name:    "api-key",
				Aliases: []string{"api-token"},
				Usage:   "Futrou API key (overrides stored credentials)",
				EnvVars: []string{constants.EnvApiToken},
			},
		},
		Commands: []*cli.Command{
			loginCommand,
			logoutCommand,
			initCommand,
			deployCommand,
			upgradeCommand,
			serverletsCommand,
			proxiesCommand,
			dnsCommand,
			projectsCommand,
			volumesCommand,
			licenseCommand,
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
