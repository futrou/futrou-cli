package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"futrou-cli/src/api"
	"futrou-cli/src/logger"
)

func TestLoadConfigPrefersJSONAndValidates(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "futrou.json")
	if err := os.WriteFile(jsonPath, []byte(`{"serverlets":[{"name":"api","image":"example/api","serverletPlanId":"plan-1"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "futrou.js"), []byte("throw new Error('must not run')"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, path, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if path != jsonPath || len(cfg.Serverlets) != 1 || cfg.Serverlets[0].Name != "api" {
		t.Fatalf("unexpected config: %#v (%s)", cfg, path)
	}

	if err := os.WriteFile(jsonPath, []byte(`{"serverlets":[{"name":"api","projectId":123}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadConfig(dir, "")
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("expected string validation error, got %v", err)
	}
}

func TestLoadConfigEndToEndFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "futrou.json")
	data := `{
  "$schema": "https://futrou.com/futrou.schema.json",
  "workspace": "acme",
  "project": "website",
  "serverlets": [{"name":"web","image":"nginx:latest","serverletPlanId":"plan-1"}],
  "dns": [{"name":"example.com","domain":"example.com","ttl":300,"priority":10}],
  "proxies": [{"domain":"example.com","type":"http","target":"web:8080","port":443}],
  "storages": [{"name":"uploads"}],
  "crons": [{"name":"cleanup"}]
}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, loadedPath, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("loading disk config: %v", err)
	}
	if loadedPath != path {
		t.Fatalf("loaded %s, want %s", loadedPath, path)
	}
	if cfg.Schema != "https://futrou.com/futrou.schema.json" || cfg.Workspace != "acme" || cfg.Project != "website" {
		t.Fatalf("unexpected selectors: %#v", cfg)
	}
	if len(cfg.Serverlets) != 1 || cfg.Serverlets[0].Name != "web" || cfg.Serverlets[0].ServerletPlanId != "plan-1" {
		t.Fatalf("serverlets were not decoded: %#v", cfg.Serverlets)
	}
	if len(cfg.DNS) != 1 || cfg.DNS[0].Domain != "example.com" || cfg.DNS[0].TTL != 300 || cfg.DNS[0].Priority != 10 {
		t.Fatalf("dns was not decoded: %#v", cfg.DNS)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].Port != 443 || len(cfg.Storages) != 1 || len(cfg.Crons) != 1 {
		t.Fatalf("resource collections were not decoded: %#v", cfg)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveConfig(dir, "", &Config{Proxies: []ProxyConfig{{Domain: "example.com", Port: 443}}})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "futrou.json") {
		t.Fatalf("saved to %s", path)
	}
	if _, _, err := LoadConfig(dir, ""); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSerializers(t *testing.T) {
	cfg := &Config{Schema: "https://futrou.com/futrou.schema.json", Project: "website"}
	jsonData, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"project": "website"`) {
		t.Fatalf("unexpected JSON: %s", jsonData)
	}
	js, err := cfg.ToJS()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(js, "export default {") || !strings.Contains(js, `"project": "website"`) {
		t.Fatalf("unexpected JS: %s", js)
	}
	schema := cfg.ToJSONSchema()
	if schema["$id"] != "https://futrou.com/futrou.schema.json" {
		t.Fatalf("unexpected schema ID: %v", schema["$id"])
	}
}

func TestLoadConfigExecutesDefaultExportFunction(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "futrou.mjs")
	source := `export default async () => ({ serverlets: [{ name: "api", image: "example/api", serverletPlanId: "plan-1" }] })`
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, loadedPath, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if loadedPath != path || len(cfg.Serverlets) != 1 || cfg.Serverlets[0].Image != "example/api" {
		t.Fatalf("unexpected config: %#v (%s)", cfg, loadedPath)
	}
}

func TestLoadConfigIncludesAllValidationErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "futrou.json")
	content := `{"workspace":123,"project":456,"serverlets":"invalid","proxies":[{"port":0}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadConfig(dir, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, field := range []string{"workspace:", "project:", "serverlets:", "proxies:"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("expected %q in validation output: %v", field, err)
		}
	}
}

func TestLoadConfigLogsUnexpectedKeysAsWarnings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"projec":"typo"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	logger.SetOutput(&output, &output)
	logger.SetLogFormat("text")
	logger.SetLogLevel("warn")
	t.Cleanup(func() {
		logger.SetOutput(os.Stdout, os.Stderr)
		logger.SetLogFormat("plain")
		logger.SetLogLevel("info")
	})

	if _, _, err := LoadConfig(dir, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "WARN") || !strings.Contains(output.String(), `found unexpected key "projec"`) {
		t.Fatalf("expected logger warning, got %q", output.String())
	}
}

func TestLoadConfigAcceptsServerletEntrypointAndTemplateMetadata(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "template": {"title": "Node.js", "category": "Starters"},
  "serverlets": [
    {"name": "web", "image": "node:22-alpine", "entrypoint": ["node", "-e", "console.log(1)"], "cwd": "/app", "ports": [{"port": 3000}]}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	logger.SetOutput(&output, &output)
	logger.SetLogFormat("text")
	logger.SetLogLevel("warn")
	t.Cleanup(func() {
		logger.SetOutput(os.Stdout, os.Stderr)
		logger.SetLogFormat("plain")
		logger.SetLogLevel("info")
	})

	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "unexpected key") {
		t.Fatalf("template metadata and entrypoint must not warn, got %q", output.String())
	}
	if len(cfg.Serverlets) != 1 || cfg.Serverlets[0].Entrypoint != "node -e console.log(1)" || cfg.Serverlets[0].Cwd != "/app" {
		t.Fatalf("entrypoint/cwd were not decoded: %#v", cfg.Serverlets)
	}
	if cfg.Template["title"] != "Node.js" {
		t.Fatalf("template metadata was not decoded: %#v", cfg.Template)
	}

	// Entrypoint must survive the round trip the deployer uses to build payloads.
	data, err := json.Marshal(cfg.Serverlets[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"entrypoint":"node -e console.log(1)"`) || !strings.Contains(string(data), `"cwd":"/app"`) {
		t.Fatalf("entrypoint/cwd missing from serialized serverlet: %s", data)
	}
}

func TestLoadConfigAcceptsNetworksAndServerletNetworks(t *testing.T) {
	dir := t.TempDir()
	data := `{
  "networks": [{"name":"backend","ipv4":"10.0.0.0/24","internal":true}],
  "serverlets": [{"name":"web","image":"nginx","networks":["backend"],"ports":[{"port":8080,"protocol":"tcp"}]}]
}`
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Networks) != 1 || cfg.Networks[0].Ipv4 != "10.0.0.0/24" || cfg.Networks[0].Internal == nil || !*cfg.Networks[0].Internal {
		t.Fatalf("unexpected networks: %#v", cfg.Networks)
	}
	if got := cfg.Serverlets[0].Networks; len(got) != 1 || got[0] != "backend" {
		t.Fatalf("unexpected serverlet networks: %#v", got)
	}
	out, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"networks"`) || !strings.Contains(string(out), `"backend"`) {
		t.Fatalf("networks must survive serialization: %s", out)
	}
}

func TestLoadConfigRejectsInvalidResources(t *testing.T) {
	for name, data := range map[string]string{
		"duplicate serverlet":    `{"serverlets":[{"name":"web"},{"name":"web"}]}`,
		"unnamed network":        `{"networks":[{"ipv4":"10.0.0.0/24"}]}`,
		"bad port":               `{"serverlets":[{"name":"web","ports":[{"port":0}]}]}`,
		"network list not names": `{"serverlets":[{"name":"web","networks":[7]}]}`,
		"bad env value":          `{"serverlets":[{"name":"web","env":{"A":1}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadConfig(dir, ""); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	valid := &Config{Serverlets: []ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := &Config{Serverlets: []ServerletConfig{{Serverlet: api.Serverlet{Image: "nginx"}}}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected an error for an unnamed serverlet")
	}
}

// fakeAPI answers GET requests from a path-to-response table.
type fakeAPI map[string]interface{}

func (f fakeAPI) RequestInto(method, path string, body interface{}, v interface{}) (int, error) {
	response, ok := f[strings.SplitN(path, "?", 2)[0]]
	if !ok {
		return 404, fmt.Errorf("not found: %s", path)
	}
	data, _ := json.Marshal(response)
	return 200, json.Unmarshal(data, v)
}

func TestResolveProject(t *testing.T) {
	client := fakeAPI{
		"/v2/workspaces": []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":   []map[string]interface{}{{"id": "proj-1", "name": "demo"}, {"id": "proj-2", "name": "other"}},
	}
	project, err := (&Config{Workspace: "acme", Project: "demo"}).ResolveProject(client)
	if err != nil || project.Id != "proj-1" {
		t.Fatalf("project = %#v, err = %v", project, err)
	}
	project, err = (&Config{Workspace: "acme", Project: "proj-2"}).ResolveProject(client)
	if err != nil || project.Name != "other" {
		t.Fatalf("lookup by ID: project = %#v, err = %v", project, err)
	}
	for name, cfg := range map[string]*Config{
		"no project":        {Workspace: "acme"},
		"no workspace":      {Project: "demo"},
		"unknown project":   {Workspace: "acme", Project: "missing"},
		"unknown workspace": {Workspace: "nope", Project: "demo"},
		"nil config":        nil,
	} {
		if _, err := cfg.ResolveProject(client); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestPullIncludesNetworksAndLocks(t *testing.T) {
	client := fakeAPI{
		"/v2/workspaces":      []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":        []map[string]interface{}{{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}},
		"/v2/workspaces/ws-1": map[string]interface{}{"id": "ws-1", "name": "acme"},
		"/v2/networks":        []map[string]interface{}{{"id": "net-1", "name": "backend", "ipv4": "10.0.0.0/24"}},
		"/v2/serverlets":      []map[string]interface{}{{"id": "sl-1", "name": "web", "image": "nginx", "networks": []string{"backend"}}},
		"/v2/dns":             []map[string]interface{}{},
		"/v2/proxies":         []map[string]interface{}{},
		"/v2/storages":        []map[string]interface{}{},
		"/v2/crons":           []map[string]interface{}{},
	}
	cfg := &Config{Workspace: "acme", Project: "demo"}
	if err := cfg.Pull(client, ""); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Networks) != 1 || cfg.Networks[0].Name != "backend" {
		t.Fatalf("unexpected networks: %#v", cfg.Networks)
	}
	if cfg.Networks[0].Id != "" {
		t.Fatalf("IDs must not be persisted in the config: %#v", cfg.Networks[0])
	}
	if cfg.Locks["networks.backend"] != LockHash("net-1") || cfg.Locks["serverlets.web"] != LockHash("sl-1") {
		t.Fatalf("unexpected locks: %#v", cfg.Locks)
	}
	if got := cfg.Serverlets[0].Networks; len(got) != 1 || got[0] != "backend" {
		t.Fatalf("serverlet networks must round-trip: %#v", got)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigReadsYAMLAndYML(t *testing.T) {
	const doc = `
workspace: acme
project: website
networks:
  - name: backend
    internal: true
serverlets:
  - name: web
    image: nginx:latest
    env:
      PORT: "8080"
    ports:
      - port: 8080
        protocol: tcp
    networks: [backend]
    entrypoint: [node, server.js] # an array is joined with spaces
dns:
  - domain: example.com
    records:
      - {name: "@", type: MX, value: mail.example.com, ttl: 3600, priority: 10}
`
	for _, name := range []string{"futrou.yaml", "futrou.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, name, doc)
			cfg, loaded, err := LoadConfig(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			if loaded != path || cfg.Workspace != "acme" || cfg.Project != "website" {
				t.Fatalf("unexpected config %#v from %s", cfg, loaded)
			}
			web := cfg.Serverlets[0]
			if web.Env["PORT"] != "8080" || web.Ports[0].Port != 8080 || web.Networks[0] != "backend" || web.Entrypoint != "node server.js" {
				t.Fatalf("unexpected serverlet: %#v", web)
			}
			if cfg.Networks[0].Internal == nil || !*cfg.Networks[0].Internal {
				t.Fatalf("unexpected network: %#v", cfg.Networks[0])
			}
			if record := cfg.DNS[0].Records[0]; record.Name != "@" || record.Priority != 10 || record.TTL != 3600 {
				t.Fatalf("unexpected record: %#v", record)
			}
		})
	}
}

func TestLoadConfigYAMLIsValidatedLikeJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "futrou.yaml", "serverlets:\n  - name: web\n    ports:\n      - port: 70000\n    env:\n      PORT: 8080\n")
	_, _, err := LoadConfig(dir, "")
	if err == nil || !strings.Contains(err.Error(), "serverlets") {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidYAML(t *testing.T) {
	for name, content := range map[string]string{
		"empty file":         "",
		"only a comment":     "# nothing here\n",
		"top-level list":     "- name: web\n",
		"top-level scalar":   "just text\n",
		"multiple documents": "workspace: a\n---\nworkspace: b\n",
		"syntax error":       "serverlets: [unclosed\n",
		"duplicate key":      "workspace: a\nworkspace: b\n",
		"non-string key":     "serverlets:\n  - name: web\n    env:\n      ? [a]\n      : b\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "futrou.yml", content)
			_, _, err := LoadConfig(dir, "")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "futrou.yml") {
				t.Fatalf("the error should name the file: %v", err)
			}
		})
	}
}

func TestLoadConfigYAMLTimestampsBecomeStrings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "futrou.yaml", "crons:\n  - name: nightly\n    createdAt: 2026-01-02T03:04:05Z\n")
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Crons[0].CreatedAt.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("createdAt = %s", got)
	}
}

func TestLoadConfigDiscoveryOrder(t *testing.T) {
	order := []string{"futrou.json", "futrou.js", "futrou.cjs", "futrou.mjs", "futrou.ts", "futrou.yaml", "futrou.yml"}
	dir := t.TempDir()
	for _, name := range order {
		writeFile(t, dir, name, "")
	}
	for _, want := range order {
		path, err := findConfig(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != want {
			t.Fatalf("found %s, want %s", filepath.Base(path), want)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := findConfig(dir, ""); !errors.Is(err, ErrNoFutrouConfig) {
		t.Fatalf("expected ErrNoFutrouConfig, got %v", err)
	}
}

func TestLoadConfigJSONWinsOverYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "futrou.yaml", "project: from-yaml\n")
	writeFile(t, dir, "futrou.json", `{"project":"from-json"}`)
	cfg, _, err := LoadConfig(dir, "")
	if err != nil || cfg.Project != "from-json" {
		t.Fatalf("project = %v, err = %v", cfg, err)
	}
	cfg, _, err = LoadConfig(dir, "futrou.yaml")
	if err != nil || cfg.Project != "from-yaml" {
		t.Fatalf("an explicit file must be honored: %v, err = %v", cfg, err)
	}
}

// loadExample loads one of the files in the examples folder.
func loadExample(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	cfg, _, err := LoadConfig("../../examples", name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	object, err := toObject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func TestExamplesAreValidAndEquivalent(t *testing.T) {
	jsonExample := loadExample(t, "futrou.json")
	for _, resource := range []string{"networks", "storages", "serverlets", "proxies", "dns", "crons"} {
		if items, _ := jsonExample[resource].([]interface{}); len(items) == 0 {
			t.Errorf("the example should demonstrate %s", resource)
		}
	}
	if !reflect.DeepEqual(loadExample(t, "futrou.yaml"), jsonExample) {
		t.Error("examples/futrou.yaml must be equivalent to examples/futrou.json")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Log("node not found; skipping examples/futrou.mjs")
		return
	}
	if !reflect.DeepEqual(loadExample(t, "futrou.mjs"), jsonExample) {
		t.Error("examples/futrou.mjs must be equivalent to examples/futrou.json")
	}
}

func TestExamplesUseEveryServerletOption(t *testing.T) {
	serverlet := loadExample(t, "futrou.json")["serverlets"].([]interface{})[0].(map[string]interface{})
	for _, key := range []string{"name", "image", "serverletPlanId", "regionId", "env", "mounts", "ports", "networks", "entrypoint", "cwd",
		"user", "needs", "healthcheck", "healthcheckInterval", "healthcheckTimeout", "healthcheckRetries", "startTimeout", "stopTimeout",
		"minInstances", "maxInstances", "scaling", "strategy"} {
		if _, ok := serverlet[key]; !ok {
			t.Errorf("examples/futrou.json should set serverlet option %q", key)
		}
	}
}

func TestEntrypointAcceptsStringOrArray(t *testing.T) {
	for name, tc := range map[string]struct{ json, want string }{
		"string":          {`"node server.js"`, "node server.js"},
		"path":            {`"/usr/local/bin/start"`, "/usr/local/bin/start"},
		"array is joined": {`["node", "-e", "console.log(1)"]`, "node -e console.log(1)"},
		"single item":     {`["/app/start.sh"]`, "/app/start.sh"},
		"empty array":     {`[]`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			var e Entrypoint
			if err := json.Unmarshal([]byte(tc.json), &e); err != nil {
				t.Fatal(err)
			}
			if string(e) != tc.want {
				t.Fatalf("got %q, want %q", e, tc.want)
			}
		})
	}
	var e Entrypoint
	for _, bad := range []string{`1`, `{"a":1}`, `[1]`, `true`} {
		if err := json.Unmarshal([]byte(bad), &e); err == nil {
			t.Errorf("%s should be rejected", bad)
		}
	}
}

func TestEntrypointFromAPIArrayIsStoredAsOneString(t *testing.T) {
	client := fakeAPI{
		"/v2/workspaces":      []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":        []map[string]interface{}{{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}},
		"/v2/workspaces/ws-1": map[string]interface{}{"id": "ws-1", "name": "acme"},
		"/v2/networks":        []map[string]interface{}{},
		"/v2/serverlets":      []map[string]interface{}{{"id": "sl-1", "name": "web", "entrypoint": []string{"node", "server.js"}}},
		"/v2/dns":             []map[string]interface{}{},
		"/v2/proxies":         []map[string]interface{}{},
		"/v2/storages":        []map[string]interface{}{},
		"/v2/crons":           []map[string]interface{}{},
	}
	cfg := &Config{Workspace: "acme", Project: "demo"}
	if err := cfg.Pull(client, ""); err != nil {
		t.Fatal(err)
	}
	if cfg.Serverlets[0].Entrypoint != "node server.js" {
		t.Fatalf("entrypoint = %q", cfg.Serverlets[0].Entrypoint)
	}
}

func TestConfigLoadDispatchesByExtension(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", `{"project":"json"}`)
	writeFile(t, dir, "b.yaml", "project: yaml\n")
	writeFile(t, dir, "c.yml", "project: yml\n")
	writeFile(t, dir, "d.txt", "project: x\n")
	for file, want := range map[string]string{"a.json": "json", "b.yaml": "yaml", "c.yml": "yml"} {
		cfg := &Config{}
		path, err := cfg.Load(dir, file)
		if err != nil || cfg.Project != want || filepath.Base(path) != file {
			t.Errorf("%s: project = %q, path = %s, err = %v", file, cfg.Project, path, err)
		}
	}
	if _, err := (&Config{}).Load(dir, "d.txt"); err == nil {
		t.Error("an unknown extension must be rejected")
	}
	if _, err := (&Config{}).Load(dir, "missing.json"); err == nil {
		t.Error("a missing file must be an error")
	}
}

func TestConfigLoadKeepsExistingConfigOnFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "futrou.json", `{"serverlets":[{"name":"web"},{"name":"web"}]}`)
	cfg := &Config{Project: "keep-me"}
	if _, err := cfg.Load(dir, ""); err == nil {
		t.Fatal("expected a validation error")
	}
	if cfg.Project != "keep-me" {
		t.Fatalf("a failed load must not modify the config: %#v", cfg)
	}
}

func TestConfigLoadFormats(t *testing.T) {
	dir := t.TempDir()
	jsonPath := writeFile(t, dir, "futrou.json", `{"project":"json"}`)
	yamlPath := writeFile(t, dir, "futrou.yaml", "project: yaml\n")
	cfg := &Config{}
	if err := cfg.LoadJSON(jsonPath); err != nil || cfg.Project != "json" {
		t.Fatalf("LoadJSON: %v, %v", cfg.Project, err)
	}
	if err := cfg.LoadYAML(yamlPath); err != nil || cfg.Project != "yaml" {
		t.Fatalf("LoadYAML: %v, %v", cfg.Project, err)
	}
	if err := cfg.LoadJSON(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("LoadJSON of a missing file must fail")
	}
	if err := cfg.LoadYAML(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("LoadYAML of a missing file must fail")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found; skipping LoadJS")
	}
	mjs := writeFile(t, dir, "futrou.mjs", `export default async () => ({ project: "mjs" });`)
	if err := cfg.LoadJS(mjs); err != nil || cfg.Project != "mjs" {
		t.Fatalf("LoadJS: %v, %v", cfg.Project, err)
	}
	if err := cfg.LoadJS(jsonPath); err == nil {
		t.Error("LoadJS must reject a non-JavaScript file")
	}
}

func sampleConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "futrou.json", `{
  "$schema": "https://futrou.com/futrou.schema.json",
  "workspace": "acme",
  "project": "demo",
  "networks": [{"name": "backend", "internal": true}],
  "serverlets": [{"name": "web", "image": "nginx:latest", "env": {"PORT": "8080", "DEBUG": "true"}, "ports": [{"port": 8080}], "networks": ["backend"], "entrypoint": "node server.js"}],
  "dns": [{"domain": "example.com", "name": "example.com", "records": [{"name": "@", "type": "MX", "value": "mail.example.com", "ttl": 3600, "priority": 10}]}]
}`)
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigSaveKeepsTheFormat(t *testing.T) {
	cfg := sampleConfig(t)
	dir := t.TempDir()
	for _, name := range []string{"futrou.json", "futrou.yaml", "futrou.yml", "nested/futrou.yaml"} {
		t.Run(name, func(t *testing.T) {
			path, err := cfg.Save(dir, name)
			if err != nil {
				t.Fatal(err)
			}
			if path != filepath.Join(dir, name) {
				t.Fatalf("path = %s", path)
			}
			reloaded := &Config{}
			if _, err := reloaded.Load(dir, name); err != nil {
				t.Fatalf("the saved file must load again: %v", err)
			}
			want, _ := toObject(cfg)
			got, _ := toObject(reloaded)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed the config:\n got %v\nwant %v", got, want)
			}
		})
	}
	data, _ := os.ReadFile(filepath.Join(dir, "futrou.json"))
	if !strings.HasPrefix(string(data), "{") {
		t.Fatalf("a .json file must hold JSON: %s", data)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "futrou.yaml"))
	if strings.HasPrefix(string(data), "{") || !strings.Contains(string(data), "\nserverlets:\n  - name: web\n") {
		t.Fatalf("a .yaml file must hold block-style YAML: %s", data)
	}
}

func TestConfigSaveDefaultsToJSON(t *testing.T) {
	dir := t.TempDir()
	path, err := sampleConfig(t).Save(dir, "")
	if err != nil || path != filepath.Join(dir, "futrou.json") {
		t.Fatalf("path = %s, err = %v", path, err)
	}
	if _, err := (*Config)(nil).Save(dir, ""); err == nil {
		t.Fatal("saving a nil config must fail")
	}
}

func TestConfigSaveYAMLQuotesAmbiguousStringsAndKeepsKeyOrder(t *testing.T) {
	data, err := sampleConfig(t).ToYAML()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`PORT: "8080"`, `DEBUG: "true"`, "ttl: 3600", "entrypoint: node server.js"} {
		if !strings.Contains(text, want) {
			t.Errorf("YAML should contain %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, `name: '@'`) && !strings.Contains(text, `name: "@"`) {
		t.Errorf("@ must be quoted:\n%s", text)
	}
	if strings.Index(text, "workspace:") > strings.Index(text, "serverlets:") || strings.Index(text, "name: web") > strings.Index(text, "image: nginx") {
		t.Errorf("keys should keep the JSON order:\n%s", text)
	}
}

func TestConfigSaveJSRefused(t *testing.T) {
	dir := t.TempDir()
	cfg := sampleConfig(t)
	for _, name := range []string{"futrou.js", "futrou.cjs", "futrou.mjs", "futrou.ts"} {
		path := writeFile(t, dir, name, "export default {}")
		if _, err := cfg.Save(dir, name); err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Errorf("%s: expected a not-supported error, got %v", name, err)
		}
		if err := cfg.SaveJS(path); err == nil {
			t.Errorf("SaveJS(%s) must fail", name)
		}
		if data, _ := os.ReadFile(path); string(data) != "export default {}" {
			t.Errorf("%s must not be modified, got %q", name, data)
		}
	}
}

func TestConfigSaveRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	invalid := &Config{Serverlets: []ServerletConfig{{}}}
	for _, name := range []string{"futrou.json", "futrou.yaml"} {
		if _, err := invalid.Save(dir, name); err == nil {
			t.Errorf("%s: an invalid config must not be written", name)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s must not exist", name)
		}
	}
}

func TestCanSave(t *testing.T) {
	for path, want := range map[string]bool{
		"futrou.json": true, "futrou.yaml": true, "futrou.YML": true,
		"futrou.js": false, "futrou.cjs": false, "futrou.mjs": false, "futrou.ts": false,
	} {
		if got := CanSave(path); got != want {
			t.Errorf("CanSave(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestConfigRegionLoadsFromEveryFormatAndRoundTrips(t *testing.T) {
	for name, content := range map[string]string{
		"futrou.json": `{"region":"europe"}`,
		"futrou.yaml": "region: europe\n",
		"futrou.yml":  "region: europe\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, content)
			cfg, _, err := LoadConfig(dir, "")
			if err != nil || cfg.Region != "europe" {
				t.Fatalf("region = %q, err = %v", cfg.Region, err)
			}
			saved := t.TempDir()
			if _, err := cfg.Save(saved, name); err != nil {
				t.Fatal(err)
			}
			reloaded, _, err := LoadConfig(saved, "")
			if err != nil || reloaded.Region != "europe" {
				t.Fatalf("region after save = %q, err = %v", reloaded.Region, err)
			}
		})
	}
	dir := t.TempDir()
	writeFile(t, dir, "futrou.json", `{"region":5}`)
	if _, _, err := LoadConfig(dir, ""); err == nil {
		t.Error("a non-string region must be rejected")
	}
}

func TestPullKeepsTheConfiguredRegion(t *testing.T) {
	client := fakeAPI{
		"/v2/workspaces":      []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":        []map[string]interface{}{{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}},
		"/v2/workspaces/ws-1": map[string]interface{}{"id": "ws-1", "name": "acme"},
		"/v2/networks":        []map[string]interface{}{},
		"/v2/serverlets":      []map[string]interface{}{},
		"/v2/dns":             []map[string]interface{}{},
		"/v2/proxies":         []map[string]interface{}{},
		"/v2/storages":        []map[string]interface{}{},
		"/v2/crons":           []map[string]interface{}{},
	}
	cfg := &Config{Workspace: "acme", Project: "demo", Region: "europe"}
	if err := cfg.Pull(client, ""); err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "europe" {
		t.Fatalf("a pull refreshes the resources but must keep the project region, got %q", cfg.Region)
	}
}

func TestExampleSetsProjectRegion(t *testing.T) {
	for _, name := range []string{"futrou.json", "futrou.yaml"} {
		if loadExample(t, name)["region"] == "" || loadExample(t, name)["region"] == nil {
			t.Errorf("examples/%s should set the project region", name)
		}
	}
}

func TestLoadConfigYAMLRejectsNonStringKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "futrou.yaml", "serverlets:\n  - name: web\n    env:\n      1: one\n")
	_, _, err := LoadConfig(dir, "")
	if err == nil || !strings.Contains(err.Error(), "futrou.yaml") {
		t.Fatalf("expected an error naming the file, got %v", err)
	}
}

func TestConfigSaveFailsWhenTheDirectoryCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	blocker := writeFile(t, dir, "blocker", "a file, not a directory")
	cfg := sampleConfig(t)
	for _, name := range []string{"futrou.json", "futrou.yaml"} {
		if _, err := cfg.Save(blocker, name); err == nil {
			t.Errorf("%s: saving below a file must fail", name)
		}
	}
}

func TestConfigSaveYAMLKeepsAllServerletRuntimeOptions(t *testing.T) {
	cfg := loadExampleConfig(t, "futrou.json")
	dir := t.TempDir()
	if _, err := cfg.Save(dir, "out.yaml"); err != nil {
		t.Fatal(err)
	}
	reloaded := &Config{}
	if _, err := reloaded.Load(dir, "out.yaml"); err != nil {
		t.Fatal(err)
	}
	want, _ := toObject(cfg)
	got, _ := toObject(reloaded)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("saving the full example as YAML changed it:\n got %v\nwant %v", got, want)
	}
	web := reloaded.Serverlets[0]
	if web.Entrypoint != "node server.js" || web.MinInstances != 1 || web.HealthcheckRetries != 3 || web.Strategy["type"] != "rolling" || web.Needs[0] != "db:healthy" {
		t.Fatalf("serverlet options were lost: %#v", web)
	}
}

func loadExampleConfig(t *testing.T, name string) *Config {
	t.Helper()
	cfg, _, err := LoadConfig("../../examples", name)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestProxyKeepsApiDefaultsWhenSettingsAreOmitted(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"proxies":[{"domain":"a.com","target":"web"}]}`), &cfg); err != nil {
		t.Fatal(err)
	}
	p := cfg.Proxies[0]
	if !p.PreserveHeaders || !p.PreserveHost || !p.PreservePath || !p.PreserveQuery || !p.VerifyTls || p.EnforceHttps || p.FollowRedirects {
		t.Fatalf("omitted settings must keep the API defaults: %#v", p)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"domain":"a.com","target":"web"}` {
		t.Fatalf("a proxy with only defaults must serialize without them: %s", out)
	}
}

func TestProxyExplicitSettingsSurviveARoundTrip(t *testing.T) {
	var cfg Config
	doc := `{"proxies":[{"domain":"a.com","preserveHost":false,"verifyTls":false,"enforceHttps":true,"followRedirects":true}]}`
	if err := json.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(cfg.Proxies[0])
	var again ProxyConfig
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatal(err)
	}
	if again.PreserveHost || again.VerifyTls || !again.EnforceHttps || !again.FollowRedirects || !again.PreserveHeaders {
		t.Fatalf("round trip changed the proxy: %#v (%s)", again, out)
	}
}

func TestNeedsLoadFromEveryFormat(t *testing.T) {
	for name, content := range map[string]string{
		"futrou.json": `{"serverlets":[{"name":"db"},{"name":"web","needs":["db:healthy"]}]}`,
		"futrou.yaml": "serverlets:\n  - name: db\n  - name: web\n    needs:\n      - db:healthy\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, content)
			cfg, _, err := LoadConfig(dir, "")
			if err != nil || len(cfg.Serverlets[1].Needs) != 1 || cfg.Serverlets[1].Needs[0] != "db:healthy" {
				t.Fatalf("needs = %v, err = %v", cfg.Serverlets[1].Needs, err)
			}
		})
	}
	dir := t.TempDir()
	writeFile(t, dir, "futrou.yaml", "serverlets:\n  - name: web\n    needs: [db]\n")
	if _, _, err := LoadConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("an undeclared need must be rejected, got %v", err)
	}
}
