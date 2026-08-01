package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCommandPrintsEmptyObjectWhenNoConfigExists(t *testing.T) {
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	out, err := captureRun([]string{"futrou", "config"})
	assertNoError(t, err)
	if strings.TrimSpace(out) != "{}" {
		t.Fatalf("expected {}, got %q", out)
	}
}

func TestConfigCommandPrintsLoadedConfig(t *testing.T) {
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"project":"demo"}`), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun([]string{"futrou", "config"})
	assertNoError(t, err)
	assertContains(t, out, `"project"`)
	assertContains(t, out, `"demo"`)
}

func TestConfigInitCreatesProjectConfig(t *testing.T) {
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	out, err := captureRun([]string{"futrou", "config", "init"})
	assertNoError(t, err)
	assertContains(t, out, "Created")
	data, err := os.ReadFile(filepath.Join(dir, "futrou.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"project": "`+filepath.Base(dir)+`"`) {
		t.Fatalf("expected default project name in %s", data)
	}
	if !strings.Contains(string(data), `"$schema": "https://futrou.com/futrou.schema.json"`) {
		t.Fatalf("expected schema URL in %s", data)
	}

	_, err = captureRun([]string{"futrou", "config", "init"})
	if err == nil {
		t.Fatal("expected init not to overwrite config")
	}
}

func TestConfigSchemaPrintsJSONSchema(t *testing.T) {
	out, err := captureRun([]string{"futrou", "config", "schema"})
	assertNoError(t, err)
	assertContains(t, out, `"$schema": "https://json-schema.org/draft/2020-12/schema"`)
	assertContains(t, out, `"$id": "https://futrou.com/futrou.schema.json"`)
	assertContains(t, out, `"serverlets"`)
}

func TestRootInitIncludesProjectAndSchema(t *testing.T) {
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	_, err = captureRun([]string{"futrou", "init", "--name", "web", "--image", "nginx:latest"})
	assertNoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "futrou.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"$schema": "https://futrou.com/futrou.schema.json"`) || !strings.Contains(string(data), `"project": "`+filepath.Base(dir)+`"`) {
		t.Fatalf("expected schema and default project in %s", data)
	}
}

func TestConfigPullWritesCloudProjectResources(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace":"acme"}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", respond(http.StatusOK, []any{map[string]any{"id": "ws-1", "name": "acme"}}))
	ts.on("GET", "/v2/projects", respond(http.StatusOK, []any{
		map[string]any{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"},
	}))
	ts.on("GET", "/v2/workspaces/ws-1", respond(http.StatusOK, map[string]any{"id": "ws-1", "name": "acme"}))
	ts.on("GET", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("projectId") != "proj-1" {
			t.Errorf("expected project scope, got %s", r.URL.RawQuery)
		}
		writeJSON(w, []any{map[string]any{"id": "sl-1", "name": "web", "image": "nginx:latest", "ram": 128, "cpu": 100, "minInstances": 1, "maxInstances": 1, "state": "running"}})
	})
	ts.on("GET", "/v2/dns", respond(http.StatusOK, []any{map[string]any{"id": "dns-1", "name": "example.com"}}))
	ts.on("GET", "/v2/dns/dns-1/records", respond(http.StatusOK, []any{map[string]any{"id": "record-1", "name": "www", "type": "A", "value": "203.0.113.10", "ttl": 300}}))
	ts.on("GET", "/v2/proxies", respond(http.StatusOK, []any{map[string]any{"id": "proxy-1", "domain": "app.example.com", "type": "http", "target": "web"}}))
	ts.on("GET", "/v2/volumes", respond(http.StatusOK, []any{map[string]any{"id": "vol-1", "name": "data", "sizeGb": 10, "type": "ssd"}}))
	ts.on("GET", "/v2/crons", respond(http.StatusOK, []any{map[string]any{"id": "cron-1", "name": "hourly", "schedule": "0 * * * *", "url": "https://example.com/job"}}))

	out, err := runArgs(t, ts, "config", "pull", "--project", "demo")
	assertNoError(t, err)
	assertContains(t, out, "Pulled project demo")

	data, err := os.ReadFile(filepath.Join(dir, "futrou.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pulled map[string]any
	if err := json.Unmarshal(data, &pulled); err != nil {
		t.Fatal(err)
	}
	if pulled["$schema"] != "https://futrou.com/futrou.schema.json" || pulled["workspace"] != "acme" || pulled["project"] != "demo" {
		t.Fatalf("unexpected project config: %#v", pulled)
	}
	serverlets := pulled["serverlets"].([]any)
	if _, exists := serverlets[0].(map[string]any)["state"]; exists {
		t.Fatalf("API-managed serverlet state must not be persisted: %#v", serverlets[0])
	}
	dns := pulled["dns"].([]any)
	if len(dns[0].(map[string]any)["records"].([]any)) != 1 {
		t.Fatalf("expected pulled DNS records: %#v", dns)
	}
}

func TestConfigPullUsesLocalProjectWithoutProjectFlag(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace":"acme","project":"existing"}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", respond(http.StatusOK, []any{map[string]any{"id": "workspace-1", "name": "acme"}}))
	ts.on("GET", "/v2/projects", respond(http.StatusOK, []any{map[string]any{"id": "project-1", "name": "existing", "workspaceId": "workspace-1"}}))
	ts.on("GET", "/v2/workspaces/workspace-1", respond(http.StatusOK, map[string]any{"id": "workspace-1", "name": "acme"}))
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/dns", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/proxies", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/volumes", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/crons", respond(http.StatusOK, []any{}))
	_, err := runArgs(t, ts, "config", "pull")
	assertNoError(t, err)
}
