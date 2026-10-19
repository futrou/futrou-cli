package commands

import (
	"regexp"
	"strings"
	"testing"

	"futrou-cli/src/constants"
)

// commandList returns the lines of the "Commands:" section of the top-level
// help, with a blank line for every group separator.
func commandList(t *testing.T) []string {
	t.Helper()
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	var lines []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "Commands:":
			in = true
		case in && strings.HasPrefix(line, "Documentation:"):
			return lines
		case in:
			lines = append(lines, line)
		}
	}
	t.Fatalf("no Commands section in:\n%s", out)
	return nil
}

func TestTopLevelHelpGroupsCommands(t *testing.T) {
	var groups [][]string
	current := []string{}
	for _, line := range commandList(t) {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				groups = append(groups, current)
				current = []string{}
			}
			continue
		}
		current = append(current, strings.Fields(line)[0]+" "+strings.Fields(line)[1])
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	// Reduce each line to its command name (the first word, plus the subcommand for init).
	names := make([][]string, len(groups))
	for i, group := range groups {
		for _, entry := range group {
			fields := strings.Fields(entry)
			if fields[0] == "init" {
				names[i] = append(names[i], "init "+fields[1])
			} else {
				names[i] = append(names[i], strings.TrimSuffix(fields[0], ","))
			}
		}
	}
	want := [][]string{
		{"up", "down"}, // "up (deploy)" in the help
		{"login", "logout", "whoami", "config"},
		{"init mcp", "init skills", "init config"},
		{"workspace", "project", "server"},
		{"serverlet", "network", "proxy", "dns", "storage", "logs"},
		{"upgrade", "schema", "version", "license", "help"},
	}
	if len(names) != len(want) {
		t.Fatalf("groups = %v\nwant %v", names, want)
	}
	for i := range want {
		if strings.Join(names[i], ",") != strings.Join(want[i], ",") {
			t.Errorf("group %d = %v, want %v", i, names[i], want[i])
		}
	}
}

func TestTopLevelHelpSeparatorsHaveNoTrailingWhitespace(t *testing.T) {
	blanks := 0
	for _, line := range commandList(t) {
		if line == "" {
			blanks++
		}
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("trailing whitespace in %q", line)
		}
	}
	// Five separators between the six groups; the final empty line is the one before "Documentation:".
	if blanks != 6 {
		t.Errorf("expected 5 separators plus the closing blank line, got %d blank lines", blanks)
	}
}

func TestTopLevelHelpKeepsTheDescriptionsAligned(t *testing.T) {
	descriptionStart := regexp.MustCompile(`^ {3}\S+(?: \S+)*? {2,}(\S)`)
	column := -1
	for _, line := range commandList(t) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		match := descriptionStart.FindStringSubmatchIndex(line)
		if match == nil {
			t.Errorf("cannot find the description in %q", line)
			continue
		}
		if column == -1 {
			column = match[2]
		} else if match[2] != column {
			t.Errorf("description of %q starts at column %d, want %d", line, match[2], column)
		}
	}
	if column == -1 {
		t.Fatal("no commands found")
	}
}

func TestUpIsListedFirstWithDeployInBrackets(t *testing.T) {
	var found bool
	for _, line := range commandList(t) {
		if strings.HasPrefix(strings.TrimSpace(line), "up (deploy)") {
			found = true
		}
	}
	if !found {
		t.Fatal(`the help should list "up (deploy)"`)
	}
	out, err := captureRun([]string{"futrou", "up", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "--dry-run")
}

func TestHelpMentionsJSONAndYAMLConfig(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "Deploy or upgrade resources based on futrou.json|yaml config")
	assertContains(t, out, "Configure a futrou.json|yaml config file for this project")
	for _, args := range [][]string{{"deploy", "--help"}, {"down", "--help"}} {
		out, err := captureRun(append([]string{"futrou"}, args...))
		assertNoError(t, err)
		assertContains(t, out, "auto-detect futrou.json|yaml, then docker-compose.yml")
	}
}

func TestConfigUsageDisplaysTheConfigAsJSON(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "config       Display the loaded Futrou project configuration as JSON\n")
}

func TestGlobalOptionsDoNotListHelp(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	global := out[strings.Index(out, "Global options:"):strings.Index(out, "Commands:")]
	assertContains(t, global, "--api-url")
	assertContains(t, global, "--api-token")
	if strings.Contains(global, "--help") {
		t.Fatalf("--help must not be listed in the global options:\n%s", global)
	}

	// The flag still works, and the other help screens still list it.
	sub, err := captureRun([]string{"futrou", "serverlet", "ls", "--help"})
	assertNoError(t, err)
	assertContains(t, sub, "--help, -h")
	if short, err := captureRun([]string{"futrou", "-h"}); err != nil || !strings.Contains(short, "Commands:") {
		t.Fatalf("-h must still show the help: %v\n%s", err, short)
	}
}

func TestHideGlobalHelpFlagOnlyTouchesTheGlobalSection(t *testing.T) {
	in := "Global options:\n   --api-url value  x\n   --help, -h       Display help\n\nOptions:\n   --help, -h  Display help\n"
	got := hideGlobalHelpFlag(in)
	want := "Global options:\n   --api-url value  x\n\nOptions:\n   --help, -h  Display help\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if hideGlobalHelpFlag("no sections here") != "no sections here" {
		t.Fatal("text without a global section must be unchanged")
	}
}

func TestLogsUsageIsGeneric(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "logs         Browse logs of all services\n")
}

func TestInitSubcommandsAllStartWithConfigure(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	for _, name := range []string{"init mcp", "init skills", "init config"} {
		var line string
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), name+" ") {
				line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), name))
			}
		}
		if !strings.HasPrefix(line, "Configure ") {
			t.Errorf("%s description %q should start with Configure", name, line)
		}
	}
}

func TestServerUsageIsManageServers(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "server       Manage servers\n")
}

func TestAppBadgeIsPlainWithoutAColorTerminal(t *testing.T) {
	// the tests do not run on a terminal, so no escape sequences are used
	for label, want := range map[string]string{"": "v", "help": "help"} {
		badge := appBadge(label)
		if strings.Contains(badge, "\033[") || !strings.Contains(badge, want) || !strings.Contains(badge, constants.Name) {
			t.Errorf("appBadge(%q) = %q", label, badge)
		}
	}
}
