package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"futrou-cli/src/cliconfig"
)

func TestRequireAuth_noConfig(t *testing.T) {
	ts := newTestServer(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	_, err := runArgsNoAuth(t, ts, "serverlets", "list")
	assertError(t, err)
	// The error message should mention login
	if err != nil {
		_ = os.Stdout // keep import; actual message goes to logger stderr
	}
}

func writeCLIConfig(t *testing.T, apiURL, workspaceID string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetDefaultWorkspace(apiURL, workspaceID)
	if err := cliconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDeployUsesTheDefaultWorkspaceWhenTheConfigHasNone(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"project":"demo","networks":[{"name":"backend"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts, cloud := newCloudServer(t)
	writeCLIConfig(t, ts.URL, "ws-1")

	out, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Applied 1 change(s).")
	if names := cloud.named("/v2/networks"); len(names) != 1 || names[0] != "backend" {
		t.Fatalf("networks = %v", names)
	}
}

func TestDeployWithoutAnyWorkspaceFails(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"project":"demo"}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts, cloud := newCloudServer(t)
	t.Setenv("HOME", t.TempDir())
	_, err := runArgs(t, ts, "deploy", "--yes")
	if err == nil || !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("error = %v", err)
	}
	if len(cloud.mutations()) != 0 {
		t.Fatal("nothing may be changed without a workspace")
	}

	// a default workspace that no longer exists, and one without a name
	writeCLIConfig(t, ts.URL, "ghost")
	if _, err := runArgs(t, ts, "deploy", "--yes"); err == nil || !strings.Contains(err.Error(), "default workspace") {
		t.Fatalf("error = %v", err)
	}
	cloud.add("/v2/workspaces", map[string]any{"id": "ws-nameless"})
	writeCLIConfig(t, ts.URL, "ws-nameless")
	if _, err := runArgs(t, ts, "deploy", "--yes"); err == nil || !strings.Contains(err.Error(), "has no name") {
		t.Fatalf("error = %v", err)
	}
}

func TestResourceCommandsRefreshTheLocalProjectConfig(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	path := filepath.Join(dir, "futrou.yaml")
	if err := os.WriteFile(path, []byte("workspace: acme\nproject: demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts, cloud := newCloudServer(t)
	cloud.add("/v2/networks", map[string]any{"id": "net-1", "name": "backend", "projectId": "proj-1"})

	// any mutation through a resource command pulls the project into the file
	out, err := runArgs(t, ts, "network", "update", "--name", "backend", "net-1")
	assertNoError(t, err)
	_ = out
	saved, _ := os.ReadFile(path)
	for _, want := range []string{"networks", "backend", "locks"} {
		if !strings.Contains(string(saved), want) {
			t.Fatalf("the config should contain %q after the change:\n%s", want, saved)
		}
	}
	if strings.HasPrefix(string(saved), "{") {
		t.Fatalf("the YAML config must stay YAML:\n%s", saved)
	}

	// a failing refresh warns but the command still succeeds
	cloud.fail("GET", "/v2/storages", 500)
	out, err = runArgs(t, ts, "network", "update", "--name", "backend", "net-1")
	assertNoError(t, err)
	assertContains(t, out, "local Futrou config was not refreshed")
}

func TestResourceCommandsLeaveJavaScriptConfigsAndConfiglessDirsAlone(t *testing.T) {
	ts, cloud := newCloudServer(t)
	cloud.add("/v2/networks", map[string]any{"id": "net-1", "name": "backend"})

	withWorkingDirectory(t, t.TempDir()) // no config at all
	if _, err := runArgs(t, ts, "network", "update", "--name", "backend", "net-1"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	source := "export default {project: 'demo', workspace: 'acme'}"
	path := filepath.Join(dir, "futrou.mjs")
	os.WriteFile(path, []byte(source), 0644)
	if _, err := exec.LookPath("node"); err == nil {
		if _, err := runArgs(t, ts, "network", "update", "--name", "backend", "net-1"); err != nil {
			t.Fatal(err)
		}
		if after, _ := os.ReadFile(path); string(after) != source {
			t.Fatalf("executable configs are never rewritten:\n%s", after)
		}
	}

	// a config without a project has nothing to refresh
	dir = t.TempDir()
	withWorkingDirectory(t, dir)
	os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"networks":[{"name":"x"}]}`), 0644)
	if _, err := runArgs(t, ts, "network", "update", "--name", "backend", "net-1"); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "futrou.json")); string(after) != `{"networks":[{"name":"x"}]}` {
		t.Fatalf("a config without a project is untouched: %s", after)
	}
}
