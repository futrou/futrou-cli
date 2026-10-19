package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrou-cli/src/services"
)

func initDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	return dir
}

func TestInitConfigCreatesTheFile(t *testing.T) {
	for _, args := range [][]string{
		{"init", "config", "--name", "web", "--image", "nginx:latest"},
		{"init", "--name", "web", "--image", "nginx:latest"},
	} {
		dir := initDir(t)
		out, err := captureRun(append([]string{"futrou"}, args...))
		assertNoError(t, err)
		assertContains(t, out, "Created futrou.json")
		data, err := os.ReadFile(filepath.Join(dir, "futrou.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"$schema"`, `"name": "web"`, `"image": "nginx:latest"`, `"project": "` + filepath.Base(dir) + `"`} {
			if !strings.Contains(string(data), want) {
				t.Errorf("%v: %s missing from %s", args, want, data)
			}
		}
	}
}

func TestInitConfigWritesYAMLForYAMLFile(t *testing.T) {
	dir := initDir(t)
	_, err := captureRun([]string{"futrou", "init", "config", "-f", "futrou.yaml", "--name", "web", "--image", "nginx:latest"})
	assertNoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "futrou.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(data), "{") || !strings.Contains(string(data), "- name: web") {
		t.Fatalf("expected block-style YAML, got:\n%s", data)
	}
	// And the generated file must load again.
	out, err := captureRun([]string{"futrou", "config", "-f", "futrou.yaml"})
	assertNoError(t, err)
	assertContains(t, out, "nginx:latest")
}

func TestInitConfigRefusesToOverwriteAndJSFiles(t *testing.T) {
	dir := initDir(t)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := captureRun([]string{"futrou", "init", "config", "--name", "web", "--image", "nginx"})
	assertError(t, err)
	assertContains(t, err.Error(), "already exists")
	if data, _ := os.ReadFile(filepath.Join(dir, "futrou.json")); string(data) != "{}" {
		t.Fatalf("the existing file must not change: %s", data)
	}
	_, err = captureRun([]string{"futrou", "init", "config", "-f", "futrou.mjs", "--name", "web", "--image", "nginx"})
	assertError(t, err)
	if _, statErr := os.Stat(filepath.Join(dir, "futrou.mjs")); statErr == nil {
		t.Fatal("a JavaScript config must not be generated")
	}
}

func TestInitConfigJSONOutput(t *testing.T) {
	initDir(t)
	out, err := captureRun([]string{"futrou", "--log-format", "json", "init", "config", "--name", "web", "--image", "nginx"})
	assertNoError(t, err)
	assertContains(t, out, "web")
}

func TestInitMcpWritesBothConfigs(t *testing.T) {
	dir := initDir(t)
	out, err := captureRun([]string{"futrou", "init", "mcp"})
	assertNoError(t, err)
	assertContains(t, out, "Configured .mcp.json")
	assertContains(t, out, "Configured .cursor/mcp.json")

	claude, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	cursor, _ := os.ReadFile(filepath.Join(dir, ".cursor", "mcp.json"))
	if !strings.Contains(string(claude), `"type": "http"`) || !strings.Contains(string(claude), "https://mcp.futrou.com") {
		t.Fatalf("unexpected .mcp.json: %s", claude)
	}
	if strings.Contains(string(cursor), `"type"`) || !strings.Contains(string(cursor), "https://mcp.futrou.com") {
		t.Fatalf("unexpected .cursor/mcp.json: %s", cursor)
	}

	out, err = captureRun([]string{"futrou", "init", "mcp"})
	assertNoError(t, err)
	assertContains(t, out, "already configured")
}

func TestInitMcpKeepsOtherServers(t *testing.T) {
	dir := initDir(t)
	existing := `{"mcpServers":{"other":{"url":"https://other.example.com"}},"extra":true}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := captureRun([]string{"futrou", "init", "mcp"})
	assertNoError(t, err)
	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	for _, want := range []string{"other.example.com", `"extra": true`, "mcp.futrou.com"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing from %s", want, data)
		}
	}
}

func TestInitMcpErrorsAndJSON(t *testing.T) {
	dir := initDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := captureRun([]string{"futrou", "init", "mcp"})
	assertError(t, err)
	assertContains(t, err.Error(), ".mcp.json")

	clean := initDir(t)
	out, err := captureRun([]string{"futrou", "--log-format", "json", "init", "mcp"})
	assertNoError(t, err)
	assertContains(t, out, "written")
	if _, statErr := os.Stat(filepath.Join(clean, ".mcp.json")); statErr != nil {
		t.Fatal(statErr)
	}
}

func TestInitSkillsRequiresNpx(t *testing.T) {
	initDir(t)
	t.Setenv("PATH", t.TempDir())
	_, err := captureRun([]string{"futrou", "init", "skills"})
	assertError(t, err)
	assertContains(t, err.Error(), "npx is required")
}

func TestInitSkillsRunsNpx(t *testing.T) {
	initDir(t)
	bin := t.TempDir()
	log := filepath.Join(bin, "args.txt")
	script := "#!/bin/sh\necho \"$@\" > " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := captureRun([]string{"futrou", "init", "skills"})
	assertNoError(t, err)
	got, _ := os.ReadFile(log)
	if strings.TrimSpace(string(got)) != "-y skills add futrou/futrou-cli -y" {
		t.Fatalf("unexpected npx arguments: %q", got)
	}
}

func TestSetupIsAHiddenAliasOfInit(t *testing.T) {
	dir := initDir(t)
	_, err := captureRun([]string{"futrou", "setup", "mcp"})
	assertNoError(t, err)
	if _, statErr := os.Stat(filepath.Join(dir, ".mcp.json")); statErr != nil {
		t.Fatalf("setup mcp should still work: %v", statErr)
	}
	out, _ := captureRun([]string{"futrou", "--help"})
	for _, want := range []string{"init config", "init mcp", "init skills"} {
		assertContains(t, out, want)
	}
	if strings.Contains(out, "setup") {
		t.Fatalf("setup should be hidden:\n%s", out)
	}
}

func TestCheckServerletNameSuggestsAFreeName(t *testing.T) {
	for name, tc := range map[string]struct {
		response any
		status   int
		name     string
		want     string
	}{
		"free name":             {[]any{map[string]any{"name": "other"}}, 200, "web", ""},
		"taken":                 {[]any{map[string]any{"name": "web"}}, 200, "web", "web-2"},
		"taken with suffixes":   {[]any{map[string]any{"name": "web"}, map[string]any{"name": "web-2"}, map[string]any{"name": "web-3"}}, 200, "web", "web-4"},
		"wrapped in data":       {map[string]any{"data": []any{map[string]any{"name": "web"}}}, 200, "web", "web-2"},
		"unknown shape":         {"oops", 200, "web", ""},
		"api error":             {map[string]string{"message": "x"}, 500, "web", ""},
		"rows without names":    {[]any{map[string]any{"id": "1"}}, 200, "web", ""},
		"every suffix is taken": {nil, 200, "web", ""},
	} {
		t.Run(name, func(t *testing.T) {
			response := tc.response
			if name == "every suffix is taken" {
				rows := []any{map[string]any{"name": "web"}}
				for i := 2; i <= 99; i++ {
					rows = append(rows, map[string]any{"name": fmt.Sprintf("web-%d", i)})
				}
				response = rows
			}
			ts := newTestServer(t)
			ts.on("GET", "/v2/serverlets", respond(tc.status, response))
			client := services.NewApiClientWithToken(ts.URL, "token")
			if got := checkServerletName(client, tc.name); got != tc.want {
				t.Fatalf("suggestion = %q, want %q", got, tc.want)
			}
		})
	}
}
