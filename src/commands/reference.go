package commands

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// Markers delimiting the generated command reference in SKILL.md and
// README.md. scripts/build-docs.go rewrites everything between them.
const (
	ReferenceStart = "<!-- cli-reference:start -->"
	ReferenceEnd   = "<!-- cli-reference:end -->"
)

// ReferenceMarkdown renders every visible command and flag of the CLI as
// markdown, so the docs cannot drift from the code.
func ReferenceMarkdown() string {
	app := buildApp()
	skip := map[string]bool{"help": true}
	for _, f := range perCommandFlags {
		for _, n := range f.Names() {
			skip[n] = true
		}
	}

	var b strings.Builder
	b.WriteString("Generated from the command definitions by `make build-docs`; do not edit by hand.\n\n")
	b.WriteString("Global options: `--api-url <string>`, `--api-token <string>` (alias `--api-key`), `--log-level <string>`, `--local`, `--dev`, `--debug`.\n")
	for _, cmd := range app.Commands {
		if cmd.Hidden || skip[cmd.Name] {
			continue
		}
		b.WriteString("\n")
		writeReference(&b, "futrou", cmd, skip)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeReference(b *strings.Builder, parent string, cmd *cli.Command, skip map[string]bool) {
	path := parent + " " + cmd.Name
	if len(cmd.Subcommands) == 0 || cmd.Action != nil {
		line := "- `" + path
		if cmd.ArgsUsage != "" {
			line += " " + cmd.ArgsUsage
		}
		line += "`"
		if len(cmd.Aliases) > 0 {
			line += " (alias: `" + strings.Join(cmd.Aliases, "`, `") + "`)"
		}
		if cmd.Usage != "" {
			line += " — " + cmd.Usage
		}
		b.WriteString(line + "\n")
		for _, flag := range cmd.Flags {
			if described := describeFlag(flag, skip); described != "" {
				b.WriteString("  - " + described + "\n")
			}
		}
	}
	subs := cmd.Subcommands
	for _, sub := range subs {
		if sub.Hidden || skip[sub.Name] {
			continue
		}
		writeReference(b, path, sub, skip)
	}
}

func describeFlag(flag cli.Flag, skip map[string]bool) string {
	names := flag.Names()
	if len(names) == 0 || skip[names[0]] {
		return ""
	}
	if hidden, ok := flag.(interface{ IsVisible() bool }); ok && !hidden.IsVisible() {
		return ""
	}
	var forms []string
	for _, n := range names {
		if len(n) == 1 {
			forms = append(forms, "-"+n)
		} else {
			forms = append(forms, "--"+n)
		}
	}
	out := "`" + strings.Join(forms, ", ")
	switch flag.(type) {
	case *cli.StringFlag:
		out += " <string>"
	case *cli.IntFlag:
		out += " <int>"
	case *cli.Float64Flag:
		out += " <number>"
	case *cli.StringSliceFlag:
		out += " <string>..."
	}
	out += "`"
	doc, _ := flag.(cli.DocGenerationFlag)
	if doc != nil && doc.GetUsage() != "" {
		out += " " + doc.GetUsage()
	}
	if req, ok := flag.(cli.RequiredFlag); ok && req.IsRequired() {
		out += " (required)"
	}
	if boolFlag, ok := flag.(*cli.BoolFlag); ok && boolFlag.Value {
		out += " (default true)"
	} else if doc != nil {
		if def := doc.GetValue(); def != "" && def != "0" && def != "false" {
			out += fmt.Sprintf(" (default %s)", def)
		}
	}
	return out
}

// ExampleFiles are the example configs embedded in the docs, with the code
// fence language used for each.
var ExampleFiles = []struct{ Name, Language string }{
	{"futrou.json", "json"},
	{"futrou.yaml", "yaml"},
	{"futrou.mjs", "js"},
}

// ExampleMarkers returns the markers delimiting the embedded example file.
func ExampleMarkers(name string) (start, end string) {
	return "<!-- example:" + name + ":start -->", "<!-- example:" + name + ":end -->"
}

// ReplaceExample returns doc with the text between the markers of the named
// example replaced by a code block holding content. A doc without those
// markers is returned unchanged.
func ReplaceExample(doc, name, language, content string) string {
	startMarker, endMarker := ExampleMarkers(name)
	start := strings.Index(doc, startMarker)
	end := strings.Index(doc, endMarker)
	if start < 0 || end < start {
		return doc
	}
	block := "```" + language + "\n" + strings.TrimRight(content, "\n") + "\n```"
	return doc[:start+len(startMarker)] + "\n\n" + block + "\n\n" + doc[end:]
}

// ReplaceReference returns doc with the text between the reference markers
// replaced by reference.
func ReplaceReference(doc, reference string) (string, error) {
	start := strings.Index(doc, ReferenceStart)
	end := strings.Index(doc, ReferenceEnd)
	if start < 0 || end < start {
		return "", fmt.Errorf("missing %s ... %s markers", ReferenceStart, ReferenceEnd)
	}
	return doc[:start+len(ReferenceStart)] + "\n\n" + reference + "\n" + doc[end:], nil
}
