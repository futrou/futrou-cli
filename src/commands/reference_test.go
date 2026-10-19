package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// TestReferenceUpToDate fails when SKILL.md or README.md no longer contain
// the current generated command reference. Fix with `make build-docs`.
func TestReferenceUpToDate(t *testing.T) {
	reference := ReferenceMarkdown()
	for _, file := range []string{"SKILL.md", "README.md"} {
		data, err := os.ReadFile("../../" + file)
		if err != nil {
			t.Fatal(err)
		}
		want, err := ReplaceReference(string(data), reference)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, example := range ExampleFiles {
			content, err := os.ReadFile("../../examples/" + example.Name)
			if err != nil {
				t.Fatal(err)
			}
			if start, _ := ExampleMarkers(example.Name); file == "SKILL.md" && !strings.Contains(want, start) {
				t.Errorf("%s is missing the %s example block", file, example.Name)
			}
			want = ReplaceExample(want, example.Name, example.Language, string(content))
		}
		if want != string(data) {
			t.Errorf("%s has an outdated command reference or example; run `make build-docs`", file)
		}
	}
}

func TestReferenceCoversCommandsAndFlags(t *testing.T) {
	reference := ReferenceMarkdown()
	for _, want := range []string{
		"futrou deploy|up ", "futrou serverlet ls ", "futrou network create --name <string> ", "futrou server logs <id> ",
		"[--live]", "[LOG-OPTIONS]", "[--file|-f <string>]", "[--yes|-y]", "--instance-id <string>",
		"futrou dns metrics <id> ", "futrou storage metrics <id> ", "futrou serverlet instance logs <id> <instance-id> [LOG-OPTIONS]",
		"# Deploy or upgrade resources based on futrou.json|yaml config",
	} {
		if !strings.Contains(reference, want) {
			t.Errorf("reference is missing %s", want)
		}
	}
	for _, hidden := range []string{"futrou serverlets", "futrou proxies", "futrou setup", "futrou teardown"} {
		if strings.Contains(reference, hidden) {
			t.Errorf("reference should not contain %s", hidden)
		}
	}
	if strings.Contains(reference, "--offset") && strings.Count(reference, "--offset") != 1 {
		t.Errorf("the log flags should be spelled out once, in the LOG-OPTIONS summary")
	}
}

func TestReferenceIsOneCodeBlockWithAlignedComments(t *testing.T) {
	reference := ReferenceMarkdown()
	if strings.Count(reference, "```") != 2 {
		t.Fatalf("the reference should be a single code block:\n%s", reference)
	}
	// Within a group the comments start in the same column.
	var column int
	for _, line := range strings.Split(reference, "\n") {
		if strings.TrimSpace(line) == "" {
			column = 0
			continue
		}
		if !strings.HasPrefix(line, "futrou ") {
			continue
		}
		i := strings.Index(line, "  # ")
		if i < 0 {
			continue
		}
		if len(line[:i]) > referenceMaxWidth {
			continue
		}
		if column == 0 {
			column = i
		} else if i != column {
			t.Errorf("comment of %q starts at column %d, want %d", line, i, column)
		}
	}
}

func TestFlagSyntax(t *testing.T) {
	for name, tc := range map[string]struct {
		flag     cli.Flag
		optional bool
		want     string
	}{
		"optional string": {&cli.StringFlag{Name: "name"}, true, "[--name <string>]"},
		"required string": {&cli.StringFlag{Name: "name", Required: true}, false, "--name <string>"},
		"alias":           {&cli.StringFlag{Name: "file", Aliases: []string{"f"}}, true, "[--file|-f <string>]"},
		"bool":            {&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}}, true, "[--yes|-y]"},
		"bool default on": {&cli.BoolFlag{Name: "live", Value: true}, true, "[--live=false]"},
		"int":             {&cli.IntFlag{Name: "limit"}, true, "[--limit <int>]"},
		"slice":           {&cli.StringSliceFlag{Name: "env"}, true, "[--env <string>...]"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := flagSyntax(tc.flag, tc.optional); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCommandFlagsCollapsesLogOptionsAndSkipsHidden(t *testing.T) {
	flags := append([]cli.Flag{&cli.StringFlag{Name: "type"}, &cli.BoolFlag{Name: "secret", Hidden: true}, &cli.BoolFlag{Name: "debug"}}, logFlags...)
	got := commandFlags(flags, map[string]bool{"debug": true})
	if got != " [--type <string>] [LOG-OPTIONS]" {
		t.Fatalf("got %q", got)
	}
	if got := commandFlags(logFlags[:2], nil); strings.Contains(got, "LOG-OPTIONS") || !strings.Contains(got, "--offset") {
		t.Fatalf("a command with only some log flags lists them: %q", got)
	}
}

func TestReplaceReference_missingMarkers(t *testing.T) {
	if _, err := ReplaceReference("no markers", "x"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReplaceReference_replacesOnlyBetweenMarkers(t *testing.T) {
	doc := "before\n" + ReferenceStart + "\nold\n" + ReferenceEnd + "\nafter\n"
	got, err := ReplaceReference(doc, "NEW")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "before\n") || !strings.HasSuffix(got, "\nafter\n") || strings.Contains(got, "old") || !strings.Contains(got, "NEW") {
		t.Fatalf("unexpected result: %q", got)
	}
	again, _ := ReplaceReference(got, "NEW")
	if again != got {
		t.Fatal("replacing twice must be idempotent")
	}
}

func TestReplaceExample(t *testing.T) {
	start, end := ExampleMarkers("futrou.yaml")
	doc := "intro\n" + start + "\nold\n" + end + "\noutro\n"
	got := ReplaceExample(doc, "futrou.yaml", "yaml", "a: 1\n")
	if !strings.Contains(got, "```yaml\na: 1\n```") || strings.Contains(got, "old") || !strings.HasPrefix(got, "intro\n") || !strings.HasSuffix(got, "outro\n") {
		t.Fatalf("unexpected result: %q", got)
	}
	if again := ReplaceExample(got, "futrou.yaml", "yaml", "a: 1\n"); again != got {
		t.Fatal("replacing twice must be idempotent")
	}
	if unchanged := ReplaceExample("no markers", "futrou.yaml", "yaml", "x"); unchanged != "no markers" {
		t.Fatalf("a doc without markers must be unchanged, got %q", unchanged)
	}
}

func TestExampleFilesExist(t *testing.T) {
	for _, example := range ExampleFiles {
		if _, err := os.Stat("../../examples/" + example.Name); err != nil {
			t.Errorf("missing example %s: %v", example.Name, err)
		}
	}
}

// findCommand resolves the longest chain of command names at the start of
// words (hidden aliases included) and returns the command and the rest.
func findCommand(commands []*cli.Command, words []string) (*cli.Command, []string) {
	var current *cli.Command
	for len(words) > 0 {
		var next *cli.Command
		for _, c := range commands {
			if c.HasName(words[0]) {
				next = c
				break
			}
		}
		if next == nil {
			break
		}
		current, commands, words = next, next.Subcommands, words[1:]
	}
	return current, words
}

func hasFlag(command *cli.Command, name string) bool {
	for _, f := range command.Flags {
		for _, n := range f.Names() {
			if n == name {
				return true
			}
		}
	}
	return false
}

// TestDocExamplesUseRealCommandsAndFlags checks every `futrou ...` line in the
// code blocks of SKILL.md and README.md against the real command tree.
func TestDocExamplesUseRealCommandsAndFlags(t *testing.T) {
	app := buildApp()
	global := map[string]bool{"help": true, "h": true, "api-url": true, "api-token": true, "api-key": true, "log-format": true, "log-level": true, "local": true, "dev": true, "debug": true}
	for _, file := range []string{"SKILL.md", "README.md"} {
		data, err := os.ReadFile("../../" + file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if start := strings.Index(text, ReferenceStart); start >= 0 {
			end := strings.Index(text, ReferenceEnd)
			text = text[:start] + text[end:] // the generated reference is checked elsewhere
		}
		inBlock := false
		for n, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inBlock = !inBlock
				continue
			}
			if !inBlock || !strings.HasPrefix(trimmed, "futrou ") {
				continue
			}
			if i := strings.Index(trimmed, " #"); i >= 0 {
				trimmed = trimmed[:i]
			}
			words := strings.Fields(trimmed)[1:]
			// Skip global flags in front of the command.
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				words = words[1:]
				if len(words) > 0 && !strings.HasPrefix(words[0], "-") {
					words = words[1:]
				}
			}
			if len(words) == 0 {
				continue
			}
			command, rest := findCommand(app.Commands, words)
			if command == nil {
				t.Errorf("%s:%d: unknown command in %q", file, n+1, trimmed)
				continue
			}
			for _, word := range rest {
				if !strings.HasPrefix(word, "-") || word == "--" {
					if word == "--" {
						break
					}
					continue
				}
				name := strings.TrimLeft(word, "-")
				if i := strings.Index(name, "="); i >= 0 {
					name = name[:i]
				}
				if !hasFlag(command, name) && !global[name] {
					t.Errorf("%s:%d: %q has no flag %q", file, n+1, command.Name, word)
				}
			}
		}
	}
}

// visibleCommandPaths lists "serverlet instance logs"-style paths of every
// visible command (including groups that have their own action).
func visibleCommandPaths(prefix string, commands []*cli.Command) []string {
	var paths []string
	for _, c := range commands {
		if c.Hidden || c.Name == "help" {
			continue
		}
		path := strings.TrimSpace(prefix + " " + c.Name)
		paths = append(paths, path)
		paths = append(paths, visibleCommandPaths(path, c.Subcommands)...)
	}
	return paths
}

// TestSkillExamplesCoverEveryCommand makes sure the hand-written examples in
// SKILL.md show every command, not only the generated reference.
func TestSkillExamplesCoverEveryCommand(t *testing.T) {
	data, err := os.ReadFile("../../SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start, end := strings.Index(text, ReferenceStart), strings.Index(text, ReferenceEnd)
	handWritten := text[:start] + text[end:]
	var examples []string
	inBlock := false
	for _, line := range strings.Split(handWritten, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
		} else if inBlock {
			examples = append(examples, strings.TrimSpace(line))
		}
	}
	joined := "\n" + strings.Join(examples, "\n") + "\n"
	for _, path := range visibleCommandPaths("futrou", buildApp().Commands) {
		if path == "futrou" {
			continue
		}
		if !strings.Contains(joined, "\n"+path+"\n") && !strings.Contains(joined, "\n"+path+" ") {
			t.Errorf("SKILL.md has no example for `%s`", path)
		}
	}
}
