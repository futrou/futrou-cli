package commands

import "github.com/urfave/cli/v2"

// legacyNames maps each canonical (singular) command name to the older names
// that must keep working. Those are registered as hidden duplicates rather
// than cli aliases so they stay out of help output.
var legacyNames = map[string][]string{
	"serverlet": {"serverlets"},
	"proxy":     {"proxies"},
	"project":   {"projects"},
	"workspace": {"workspaces"},
	"storage":   {"storages"},
	"network":   {"networks"},
	"server":    {"servers"},
	"init":      {"setup"},
	"down":      {"teardown"},
	"plan":      {"plans"},
	"record":    {"records"},
	"user":      {"users"},
	"file":      {"files"},
	"limit":     {"limits"},
	"instance":  {"instances"},
	"ls":        {"list"},
	"rm":        {"delete", "remove"},
	"show":      {"get", "inspect"},
}

// withLegacyNames returns command with hidden duplicates of itself and of
// every subcommand for each older name in legacyNames, skipping names a
// sibling already uses.
func withLegacyNames(command *cli.Command) *cli.Command {
	cloned := *command
	cloned.Subcommands = expandLegacy(command.Subcommands)
	return &cloned
}

func expandLegacy(commands []*cli.Command) []*cli.Command {
	if len(commands) == 0 {
		return commands
	}
	taken := map[string]bool{}
	for _, c := range commands {
		for _, n := range c.Names() {
			taken[n] = true
		}
	}
	out := make([]*cli.Command, 0, len(commands))
	for _, c := range commands {
		canonical := withLegacyNames(c)
		out = append(out, canonical)
		for _, old := range legacyNames[c.Name] {
			if taken[old] {
				continue
			}
			taken[old] = true
			hidden := *canonical
			hidden.Name = old
			hidden.Hidden = true
			out = append(out, &hidden)
		}
	}
	return out
}

// withLegacyTopLevel is withLegacyNames for a top-level command, returning
// the command followed by its hidden legacy duplicates.
func withLegacyTopLevel(command *cli.Command) []*cli.Command {
	return expandLegacy([]*cli.Command{command})
}
