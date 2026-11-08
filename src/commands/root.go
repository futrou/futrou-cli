package commands

import (
	"os"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

func Execute() {
	app := newApp()
	if err := app.Run(os.Args); err != nil {
		logger.Fatal("%v", err)
	}
}

// globalApiUrl returns the --api-url value walking up to the root context.
// A missing scheme is added and a trailing /v2 dropped (services.NormalizeApiUrl),
// so "localhost:4000", "api.futrou.com" and "https://api.futrou.com/v2" all work.
func globalApiUrl(c *cli.Context) string {
	if v := lineageString(c, "api-url"); v != "" {
		return services.NormalizeApiUrl(v)
	}
	return services.NormalizeApiUrl(constants.ApiUrl)
}

// globalApiKey returns the --api-token value (or its hidden --api-key
// alias), taking the nearest one up the context lineage.
func globalApiKey(c *cli.Context) string {
	for _, ctx := range c.Lineage() {
		if v := ctx.String("api-token"); v != "" {
			return v
		}
		if v := ctx.String("api-key"); v != "" {
			return v
		}
	}
	return ""
}

// appLevelFlags are the root App's flags; they carry the defaults and the
// environment variables. withGlobalFlags also adds default-less hidden copies
// (leafOverrideFlags) to every command, so they work before or after the
// command name.
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

// leafOverrideFlags returns hidden copies of the app-level flags for every
// command, so `futrou login --api-url x` works like `futrou --api-url x login`.
// They have no default and no environment variable: the root flag keeps both,
// and the helpers below take the nearest non-empty value walking up the
// context lineage, so an unset copy never shadows a value given earlier.
func leafOverrideFlags() []cli.Flag {
	var flags []cli.Flag
	for _, name := range []string{"api-url", "api-token", "api-key", "log-format"} {
		flags = append(flags, &cli.StringFlag{Name: name, Hidden: true})
	}
	return flags
}

// lineageString returns the nearest non-empty value of a string flag, from c
// up to the root, or "" when none is set.
func lineageString(c *cli.Context, name string) string {
	for _, ctx := range c.Lineage() {
		if v := ctx.String(name); v != "" {
			return v
		}
	}
	return ""
}

// lineageBool reports whether a bool flag is true on c or any parent context.
func lineageBool(c *cli.Context, name string) bool {
	for _, ctx := range c.Lineage() {
		if ctx.Bool(name) {
			return true
		}
	}
	return false
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
	// An empty FUTROU_API_URL counts as set to urfave, but it chooses nothing.
	urlChosen := func() bool {
		for _, ctx := range c.Lineage() {
			if ctx.IsSet("api-url") && ctx.String("api-url") != "" {
				return true
			}
		}
		return false
	}
	if lineageBool(c, "local") && !urlChosen() {
		if err := c.Set("api-url", constants.LocalApiUrl); err != nil {
			return err
		}
	}
	if lineageBool(c, "dev") && !urlChosen() {
		if err := c.Set("api-url", constants.DevApiUrl); err != nil {
			return err
		}
	}

	logLevel := lineageString(c, "log-level")
	if logLevel == "" {
		logLevel = constants.DefaultLogLevel
	}
	if lineageBool(c, "debug") {
		logLevel = "debug"
	}
	logFormat := lineageString(c, "log-format")
	if logFormat == "" {
		logFormat = constants.DefaultLogFormat
	}
	logger.SetLogLevel(logLevel)
	logger.SetLogFormat(logFormat)
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
	cloned.Flags = append(append(append([]cli.Flag{}, perCommandFlags...), leafOverrideFlags()...), cloned.Flags...)

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
		Commands: appCommands(),
		Before:   applyGlobalFlagShorthands,
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

// appCommands lists the top-level commands, each followed by its hidden
// legacy (plural) duplicates.
func appCommands() []*cli.Command {
	var commands []*cli.Command
	for _, command := range []*cli.Command{
		// Keep in sync with helpGroupBreaks in help.go, which separates the
		// groups with a blank line in the top-level help.
		deployCommand,
		downCommand,

		loginCommand,
		logoutCommand,
		whoamiCommand,
		configCommand,

		initCommand,

		workspacesCommand,
		projectsCommand,
		serverCommand,

		serverletsCommand,
		networkCommand,
		proxiesCommand,
		dnsCommand,
		storagesCommand,
		logsCommand,

		upgradeCommand,
		schemaCommand,
		versionCommand,
		licenseCommand,
	} {
		commands = append(commands, withLegacyTopLevel(withGlobalFlags(command))...)
	}
	return commands
}
