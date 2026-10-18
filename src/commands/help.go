package commands

import (
	"fmt"
	"io"
	"strings"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"

	"github.com/urfave/cli/v2"
)

const defaultHelpUsage = "Shows a list of commands or help for one command"
const helpUsage = "Display the list of commands or help for one command"

const (
	// A single yellow stripe, a bold-white-on-dark-gray title, and a
	// muted-off-white-on-light-gray version.
	helpBgYellow      = "\033[43m"
	helpBold          = "\033[1m"
	helpWhiteBright   = "\033[97m"
	helpWhiteMuted    = "\033[37m"
	helpBgGrayLight   = "\033[48;5;240m"
	helpBgBlackBright = "\033[100m"
	helpFgReset       = "\033[39m"
	helpBgReset       = "\033[49m"
	helpBoldReset     = "\033[22m"
)

// appBadge renders the "  Futrou CLI  v0.0.0  " banner, colored when the
// terminal supports it and plain otherwise.
func appBadge(label string) string {
	title := fmt.Sprintf(" %s ", constants.Name)
	subtitle := fmt.Sprintf(" v%s ", constants.Version)
	if !logger.UseColors() {
		if label == "" {
			return title + subtitle
		}
		return title + subtitle + " " + label
	}
	badge := helpBgYellow + " " + helpBgReset +
		helpBgBlackBright + helpBold + helpWhiteBright + title + helpFgReset + helpBoldReset + helpBgReset +
		helpWhiteMuted + helpBgGrayLight + subtitle + helpBgReset + helpFgReset
	if label != "" {
		badge += " " + label
	}
	return badge
}

func customAppHelpTemplate() string {
	tpl := appBadge("help") + `

{{.Usage}}
{{if .VisibleFlagCategories}}
Global options:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}
Global options:{{template "visibleFlagTemplate" .}}{{end}}
{{if .VisibleCommands}}
Commands:{{commandLines .VisibleCommands}}
{{end}}
Documentation:
   For more information and detailed guides, visit ` + constants.DocsUrl + `
`
	return tpl
}

// expandedGroups lists command names whose subcommands should be listed
// inline in top-level help (e.g. "init config", "init mcp") instead of
// collapsing to a single "init" line, since unlike proxies/dns/serverlets/
// etc. their subcommands are the whole point of the command.
var expandedGroups = map[string]bool{
	"init": true,
}

// helpGroupBreaks names the top-level commands that start a new group in the
// top-level help; a blank line is printed before each. The order of the
// groups is the order of appCommands in root.go.
var helpGroupBreaks = map[string]bool{
	"login":     true, // account and config
	"init":      true, // project setup
	"workspace": true, // workspaces, projects and servers
	"serverlet": true, // services
	"upgrade":   true, // everything else
}

// helpDisplayNames overrides how a command is named in the top-level help;
// the aliases are not listed after the name as usual.
var helpDisplayNames = map[string]string{
	"deploy": "up (deploy)",
}

// commandLines renders the top-level command list, one line per command,
// expanding the groups named in expandedGroups into one line per subcommand.
func commandLines(commands []*cli.Command) string {
	var b strings.Builder
	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}
		if helpGroupBreaks[cmd.Name] {
			// A tab-terminated empty cell keeps the blank line inside the
			// aligned column block instead of restarting the alignment.
			b.WriteString("\n   \t")
		}
		if expandedGroups[cmd.Name] {
			for _, sub := range cmd.Subcommands {
				if sub.Hidden || sub.Name == "help" {
					continue
				}
				fmt.Fprintf(&b, "\n   %s %s\t%s", cmd.Name, sub.Name, sub.Usage)
			}
			continue
		}
		display, ok := helpDisplayNames[cmd.Name]
		if !ok {
			display = strings.Join(cmd.Names(), ", ")
		}
		fmt.Fprintf(&b, "\n   %s\t%s", display, cmd.Usage)
	}
	return b.String()
}

// commandHelpTemplate is cli.CommandHelpTemplate with the same badge header
// as the app-level help, and no redundant Name/Usage boilerplate.
const commandHelpTemplate = commandBadgeHeader + `{{if .Description}}
Description:
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleFlagCategories}}
Options:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}
Options:{{template "visibleFlagTemplate" .}}{{end}}
`

// subcommandHelpTemplate is cli.SubcommandHelpTemplate with the same badge
// header as the app-level help, and no redundant Name/Usage boilerplate.
const subcommandHelpTemplate = commandBadgeHeader + `{{if .Description}}
Description:
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}
Commands:{{template "visibleCommandCategoryTemplate" .}}
{{end}}{{if .VisibleFlagCategories}}
Options:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}
Options:{{template "visibleFlagTemplate" .}}{{end}}
`

// commandBadgeHeader renders the badge in place of the default "NAME:"/
// "USAGE:" block, with the command's one-line usage text directly beneath.
const commandBadgeHeader = `{{commandBadge .HelpName}}

{{.Usage}}
`

// setHelpTemplate assigns the colored help template to the app and rewrites
// urfave/cli's built-in "help, h" command usage text, which can't be
// mutated directly since it's an unexported package var.
func setHelpTemplate(app *cli.App) {
	app.CustomAppHelpTemplate = customAppHelpTemplate()
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.SubcommandHelpTemplate = subcommandHelpTemplate
	if bf, ok := cli.HelpFlag.(*cli.BoolFlag); ok {
		bf.Usage = "Display help"
	}

	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		var buf strings.Builder
		cli.HelpPrinterCustom(&buf, templ, data, map[string]interface{}{
			"commandBadge": commandBadge,
			"commandLines": commandLines,
		})
		io.WriteString(w, hideGlobalHelpFlag(clearBlankLines(strings.ReplaceAll(buf.String(), defaultHelpUsage, helpUsage))))
	}
}

// commandBadge renders the badge for a command/subcommand help screen. The
// leading "Futrou CLI" is stripped from helpName (e.g. "Futrou CLI proxies
// logs") since the badge title already shows it, leaving just "proxies logs".
func commandBadge(helpName string) string {
	label := strings.TrimSpace(strings.TrimPrefix(helpName, constants.Name))
	return appBadge(label)
}

// clearBlankLines removes the padding the column alignment leaves on the
// separator lines of the command list.
func clearBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// hideGlobalHelpFlag drops the "--help, -h" line from the "Global options:"
// section of the top-level help; every command still accepts the flag, and
// the per-command help keeps listing it.
func hideGlobalHelpFlag(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inGlobal := false
	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == "Global options:":
			inGlobal = true
		case strings.TrimSpace(line) == "":
			inGlobal = false
		case inGlobal && strings.HasPrefix(strings.TrimSpace(line), "--help, -h"):
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
