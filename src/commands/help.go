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
Commands:{{template "visibleCommandCategoryTemplate" .}}
{{end}}
Documentation:
   For more information and detailed guides, visit ` + constants.DocsUrl + `
`
	return tpl
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
		})
		io.WriteString(w, strings.ReplaceAll(buf.String(), defaultHelpUsage, helpUsage))
	}
}

// commandBadge renders the badge for a command/subcommand help screen. The
// leading "Futrou CLI" is stripped from helpName (e.g. "Futrou CLI proxies
// logs") since the badge title already shows it, leaving just "proxies logs".
func commandBadge(helpName string) string {
	label := strings.TrimSpace(strings.TrimPrefix(helpName, constants.Name))
	return appBadge(label)
}
