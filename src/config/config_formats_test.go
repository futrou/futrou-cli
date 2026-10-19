package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// one config, written in every supported format and in several shapes
const formatsJSON = `{
  "workspace": "acme",
  "project": "demo",
  "region": "europe",
  "networks": [{"name": "backend", "internal": true}],
  "storages": [{"name": "data"}],
  "serverlets": [{"name": "db", "image": "postgres:17", "serverletPlanId": "plan-1"}, {
    "name": "web", "image": "nginx:1", "serverletPlanId": "plan-1",
    "env": {"PORT": "8080"}, "ports": [{"port": 8080, "protocol": "tcp"}],
    "networks": ["backend"], "entrypoint": ["node", "server.js"], "needs": ["db:healthy"]
  }],
  "proxies": [{"domain": "app.example.com", "target": "web", "port": 8080}],
  "dns": [{"domain": "example.com", "name": "example.com", "records": [{"name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300}]}],
  "crons": [{"name": "nightly"}]
}`

const formatsYAML = `
workspace: acme
project: demo
region: europe
networks:
  - name: backend
    internal: true
storages:
  - name: data
serverlets:
  - name: db
    image: postgres:17
    serverletPlanId: plan-1
  - name: web
    image: nginx:1
    serverletPlanId: plan-1
    env: {PORT: "8080"}
    ports: [{port: 8080, protocol: tcp}]
    networks: [backend]
    entrypoint: [node, server.js]
    needs: ["db:healthy"]
proxies:
  - domain: app.example.com
    target: web
    port: 8080
dns:
  - domain: example.com
    name: example.com
    records:
      - {name: www, type: A, value: 192.0.2.1, ttl: 300}
crons:
  - name: nightly
`

const formatsKeyedYAML = `
workspace: acme
project: demo
region: europe
networks:
  backend: {internal: true}
storages:
  data:
serverlets:
  db:
    image: postgres:17
    serverletPlanId: plan-1
  web:
    image: nginx:1
    serverletPlanId: plan-1
    env: {PORT: "8080"}
    ports: [{port: 8080, protocol: tcp}]
    networks: [backend]
    entrypoint: [node, server.js]
    needs: ["db:healthy"]
proxies:
  app.example.com: {target: web, port: 8080}
dns:
  example.com:
    records:
      - {name: www, type: A, value: 192.0.2.1, ttl: 300}
crons:
  nightly:
`

func loadFormat(t *testing.T, name, content string) *Config {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, name, content)
	cfg := &Config{}
	if _, err := cfg.Load(dir, name); err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return cfg
}

func requireJSRuntime(t *testing.T, ext string) {
	t.Helper()
	if _, err := findRuntime(ext); err != nil {
		t.Skipf("no runtime for %s: %v", ext, err)
	}
}

func TestEveryConfigFormatLoadsTheSameConfig(t *testing.T) {
	want := loadFormat(t, "futrou.json", formatsJSON)
	if len(want.Serverlets) != 2 || want.Serverlets[1].Entrypoint != "node server.js" || want.Serverlets[1].Needs[0] != "db:healthy" {
		t.Fatalf("the JSON reference config is off: %#v", want.Serverlets)
	}
	cases := map[string]string{
		"futrou.yaml":       formatsYAML,
		"futrou.yml":        formatsYAML,
		"futrou.mjs":        "export default " + formatsJSON,
		"futrou.cjs":        "module.exports = " + formatsJSON,
		"futrou.js":         "export default () => (" + formatsJSON + ")",
		"futrou.ts":         "const config: Record<string, unknown> = " + formatsJSON + ";\nexport default config;",
		"futrou.async.mjs":  "export default async () => (" + formatsJSON + ")",
		"futrou.named.mjs":  "export const project = 'demo';\nexport const workspace = 'acme';\nexport const region = 'europe';\nexport const networks = [{name: 'backend', internal: true}];\nexport const storages = [{name: 'data'}];\nexport const serverlets = " + `[{"name":"db","image":"postgres:17","serverletPlanId":"plan-1"},{"name":"web","image":"nginx:1","serverletPlanId":"plan-1","env":{"PORT":"8080"},"ports":[{"port":8080,"protocol":"tcp"}],"networks":["backend"],"entrypoint":["node","server.js"],"needs":["db:healthy"]}]` + ";\nexport const proxies = [{domain: 'app.example.com', target: 'web', port: 8080}];\nexport const dns = [{domain: 'example.com', name: 'example.com', records: [{name: 'www', type: 'A', value: '192.0.2.1', ttl: 300}]}];\nexport const crons = [{name: 'nightly'}];\n",
		"futrou.keyed.yaml": formatsKeyedYAML,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			ext := filepath.Ext(name)
			if ext != ".yaml" && ext != ".yml" {
				requireJSRuntime(t, ext)
			}
			got := loadFormat(t, name, content)
			if !reflect.DeepEqual(got, want) {
				g, _ := got.ToJSON()
				w, _ := want.ToJSON()
				t.Fatalf("config differs from the JSON one\n got: %s\nwant: %s", g, w)
			}
		})
	}
}

func TestEveryFormatRejectsTheSameInvalidConfig(t *testing.T) {
	invalid := map[string]string{
		"json": `{"serverlets": [{"name": "web"}], "region": 7}`,
		"yaml": "serverlets:\n  - name: web\nregion: 7\n",
		"mjs":  `export default {serverlets: [{name: "web"}], region: 7}`,
	}
	for ext, content := range invalid {
		t.Run(ext, func(t *testing.T) {
			if ext == "mjs" {
				requireJSRuntime(t, ".mjs")
			}
			dir := t.TempDir()
			name := "futrou." + ext
			writeFile(t, dir, name, content)
			_, err := (&Config{}).Load(dir, name)
			if err == nil || !strings.Contains(err.Error(), "region") {
				t.Fatalf("error = %v, want a region error", err)
			}
		})
	}
}

func TestLoadConfigShapesAndEdgeCases(t *testing.T) {
	for name, tc := range map[string]struct {
		file, content string
		wantErr       string
	}{
		"empty json":               {"futrou.json", ``, "parsing"},
		"json array at top level":  {"futrou.json", `[]`, "parsing"},
		"json trailing garbage":    {"futrou.json", `{"project":"a"} x`, ""},
		"json null":                {"futrou.json", `null`, ""},
		"fractional number":        {"futrou.json", `{"serverlets":[{"name":"a","image":"b","minInstances":1.5}]}`, ""},
		"json unknown top key":     {"futrou.json", `{"surprise": true}`, ""},
		"yaml top level list":      {"futrou.yaml", "- a\n- b\n", "top level must be a mapping"},
		"yaml scalar":              {"futrou.yaml", "hello\n", "top level must be a mapping"},
		"yaml two documents":       {"futrou.yaml", "project: a\n---\nproject: b\n", "multiple YAML documents"},
		"yaml tab indentation":     {"futrou.yaml", "project:\n\t- a\n", "parsing"},
		"yaml integer map key":     {"futrou.yaml", "serverlets:\n  1: {image: x}\n", ""},
		"yaml anchors and aliases": {"futrou.yaml", "defaults: &d {image: nginx}\nserverlets:\n  - {name: a, <<: *d}\n", ""},
		"yaml null collection":     {"futrou.yaml", "project: demo\nserverlets:\n", ""},
		"yaml empty collection":    {"futrou.yaml", "project: demo\nserverlets: []\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.content)
			_, err := (&Config{}).Load(dir, tc.file)
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			// every case must end in a clean result or an error, never a panic
		})
	}
}

func TestLoadJSFailureModes(t *testing.T) {
	requireJSRuntime(t, ".mjs")
	for name, tc := range map[string]struct{ content, wantErr string }{
		"throws":             {`throw new Error("boom")`, "evaluating"},
		"syntax error":       {`export default {`, "evaluating"},
		"array export":       {`export default []`, "evaluating"},
		"null export":        {`export default null`, "evaluating"},
		"string export":      {`export default "nope"`, "evaluating"},
		"rejecting function": {`export default async () => { throw new Error("nope") }`, "evaluating"},
		"function to array":  {`export default () => []`, "evaluating"},
		"undefined default":  {`export const project = "named"`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "futrou.mjs", tc.content)
			err := (&Config{}).LoadJS(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
	if err := (&Config{}).LoadJS(filepath.Join(t.TempDir(), "missing.mjs")); err == nil {
		t.Fatal("a missing JS config must fail")
	}
}

func TestFindRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := findRuntime(".mjs"); err == nil || !strings.Contains(err.Error(), "node, bun, or deno") {
		t.Fatalf("error = %v", err)
	}
	if _, err := findRuntime(".ts"); err == nil || !strings.Contains(err.Error(), "bun or deno") {
		t.Fatalf("error = %v", err)
	}
	_, err := evaluateJSConfig("futrou.mjs")
	if err == nil || !strings.Contains(err.Error(), "none found") {
		t.Fatalf("a missing runtime must be explained: %v", err)
	}
	if _, err := evaluateJSConfig("futrou.txt"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v", err)
	}

	// each runtime is found when it is the only one on the PATH
	for _, tc := range []struct{ name, ext string }{{"node", ".js"}, {"bun", ".ts"}, {"deno", ".ts"}, {"bun", ".mjs"}, {"deno", ".cjs"}} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, tc.name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		rt, err := findRuntime(tc.ext)
		if err != nil || rt.name != tc.name {
			t.Errorf("findRuntime(%s) with only %s = %v, %v", tc.ext, tc.name, rt, err)
		}
	}
}

func TestEvaluateJSConfigWithDeno(t *testing.T) {
	deno, err := exec.LookPath("deno")
	if err != nil {
		t.Skip("deno is not available")
	}
	// Deno is only picked when node and bun are missing, so give it a PATH of its own.
	dir := t.TempDir()
	if err := os.Symlink(deno, filepath.Join(dir, "deno")); err != nil {
		t.Skip("cannot link deno")
	}
	t.Setenv("PATH", dir)
	t.Setenv("HOME", t.TempDir())
	path := writeFile(t, t.TempDir(), "futrou.ts", `const c: {project: string} = {project: "deno"}; export default () => c;`)
	cfg := &Config{}
	if err := cfg.LoadJS(path); err != nil || cfg.Project != "deno" {
		t.Fatalf("deno load = %q, %v", cfg.Project, err)
	}
	bad := writeFile(t, t.TempDir(), "futrou.mjs", `export default 5`)
	if err := (&Config{}).LoadJS(bad); err == nil {
		t.Fatal("a non-object export must fail under deno too")
	}
}

func TestSaveRoundTripsEveryFormatShape(t *testing.T) {
	want := loadFormat(t, "futrou.json", formatsJSON)
	for _, name := range []string{"futrou.json", "futrou.yaml", "futrou.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path, err := want.Save(dir, name)
			if err != nil {
				t.Fatal(err)
			}
			got := &Config{}
			if _, err := got.Load(dir, filepath.Base(path)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				g, _ := got.ToJSON()
				w, _ := want.ToJSON()
				t.Fatalf("round trip differs\n got: %s\nwant: %s", g, w)
			}
		})
	}
}

func TestToJSIsLoadable(t *testing.T) {
	requireJSRuntime(t, ".mjs")
	want := loadFormat(t, "futrou.json", formatsJSON)
	js, err := want.ToJS()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(js), "}") && !strings.HasSuffix(strings.TrimSpace(js), ";") {
		t.Fatalf("generated JS looks truncated: %s", js)
	}
	got := loadFormat(t, "futrou.mjs", js)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("generated ESM did not load back to the same config")
	}
}

func TestSaveConfigAndLoadProjectConfigHelpers(t *testing.T) {
	if _, err := SaveConfig(t.TempDir(), "", nil); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("a nil config must be refused: %v", err)
	}
	dir := t.TempDir()
	cfg := &Config{Project: "demo"}
	path, err := SaveConfig(dir, "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	loaded, loadedPath, err := LoadProjectConfig(dir, "", "")
	if err != nil || loaded.Project != "demo" || loadedPath != path {
		t.Fatalf("loaded = %#v from %s, %v", loaded, loadedPath, err)
	}
	if _, _, err := LoadProjectConfig(dir, "", "../evil"); err == nil {
		t.Fatal("a project name with a path must be rejected")
	}
	if _, _, err := LoadConfig(t.TempDir(), ""); err == nil {
		t.Fatal("a directory without any config must fail")
	}
}
