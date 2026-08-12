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
	return constants.ApiUrl
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

// appLevelFlags are the flags declared only on the root App, matching prior
// behavior: they must be given before the subcommand name (e.g.
// `futrou --api-url http://x serverlets list`). They aren't cloned onto
// subcommands because several helpers (globalApiUrl, globalApiKey, isJSON)
// resolve them by walking the context lineage, which requires that only one
// flagSet in that lineage ever defines them — a leaf-level clone with its
// own default `Value` would shadow a value set higher up.
var appLevelFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "log-format",
		Usage:   "Output format: text or json",
		Value:   constants.DefaultLogFormat,
		EnvVars: []string{constants.EnvLogFormat},
		Hidden:  true,
	},
	&cli.StringFlag{
		Name:    "api-url",
		Usage:   "Futrou API URL",
		Value:   constants.ApiUrl,
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
}

// perCommandFlags are cloned onto every command and subcommand (see
// withGlobalFlags) so they're accepted after the command name too (e.g.
// `futrou deploy --log-level debug`, not just
// `futrou --log-level debug deploy`). Unlike appLevelFlags, nothing reads
// these via lineage-walking helpers, so per-level shadowing isn't a concern.
var perCommandFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "log-level",
		Usage:   "Log level: debug, info, warn, error",
		Value:   constants.DefaultLogLevel,
		EnvVars: []string{constants.EnvLogLevel},
		Hidden:  true,
	},
	&cli.BoolFlag{
		Name:   "local",
		Usage:  "Shorthand for --api-url " + constants.LocalApiUrl,
		Hidden: true,
	},
	&cli.BoolFlag{
		Name:   "dev",
		Usage:  "Shorthand for --api-url " + constants.DevApiUrl,
		Hidden: true,
	},
	&cli.BoolFlag{
		Name:   "debug",
		Usage:  "Shorthand for --log-level debug",
		Hidden: true,
	},
}

// applyGlobalFlagShorthands resolves the --local, --dev and --debug
// shorthand flags into their underlying --api-url / --log-level effects,
// and applies the resulting log level and format. Safe to call at multiple
// levels of the command tree: --local/--dev/--debug/--log-level are only
// ever declared on the context's own flagSet (see perCommandFlags), so
// c.Bool/c.String here always resolve to values set at or below this exact
// context, never a shadowed ancestor value.
func applyGlobalFlagShorthands(c *cli.Context) error {
	if c.Bool("local") && !c.IsSet("api-url") {
		if err := c.Set("api-url", constants.LocalApiUrl); err != nil {
			return err
		}
	}
	if c.Bool("dev") && !c.IsSet("api-url") {
		if err := c.Set("api-url", constants.DevApiUrl); err != nil {
			return err
		}
	}

	logLevel := c.String("log-level")
	if c.Bool("debug") {
		logLevel = "debug"
	}
	logger.SetLogLevel(logLevel)
	logger.SetLogFormat(c.String("log-format"))
	return nil
}

// withGlobalFlags clones perCommandFlags onto command and, recursively, onto
// all of its subcommands, and ensures applyGlobalFlagShorthands runs
// against the leaf command's own context. This is necessary because
// urfave/cli only parses flags positioned before the first non-flag
// argument on each flag set, so e.g. `futrou deploy --local` places
// --local on the deploy subcommand's flag set, invisible to the root
// App's Before hook.
func withGlobalFlags(command *cli.Command) *cli.Command {
	if command == nil {
		return nil
	}

	cloned := *command
	cloned.Flags = append(append([]cli.Flag{}, perCommandFlags...), cloned.Flags...)

	if len(command.Subcommands) > 0 {
		cloned.Subcommands = make([]*cli.Command, 0, len(command.Subcommands))
		for _, subcommand := range command.Subcommands {
			cloned.Subcommands = append(cloned.Subcommands, withGlobalFlags(subcommand))
		}
	} else {
		innerBefore := command.Before
		cloned.Before = func(c *cli.Context) error {
			if err := applyGlobalFlagShorthands(c); err != nil {
				return err
			}
			if innerBefore != nil {
				return innerBefore(c)
			}
			return nil
		}
	}

	return &cloned
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
		Flags: append(append(append([]cli.Flag{}, appLevelFlags...), perCommandFlags...), &cli.BoolFlag{
			Name:   "version",
			Hidden: true,
		}),
		Action: func(c *cli.Context) error {
			if c.Bool("version") {
				return versionCommand.Action(c)
			}
			return cli.ShowAppHelp(c)
		},
		Commands: []*cli.Command{
			withGlobalFlags(configCommand),
			withGlobalFlags(loginCommand),
			withGlobalFlags(logoutCommand),
			withGlobalFlags(whoamiCommand),
			withGlobalFlags(setupCommand),
			withGlobalFlags(initCommand),
			withGlobalFlags(deployCommand),
			withGlobalFlags(upgradeCommand),
			withGlobalFlags(serverletsCommand),
			withGlobalFlags(proxiesCommand),
			withGlobalFlags(dnsCommand),
			withGlobalFlags(projectsCommand),
			withGlobalFlags(workspacesCommand),
			withGlobalFlags(volumesCommand),
			withGlobalFlags(licenseCommand),
			withGlobalFlags(versionCommand),
			withGlobalFlags(schemaCommand),
		},
		Before: applyGlobalFlagShorthands,
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
