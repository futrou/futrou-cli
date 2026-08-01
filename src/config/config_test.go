package config

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"futrou-cli/src/logger"
)

func TestLoadConfigPrefersJSONAndValidates(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "futrou.json")
	if err := os.WriteFile(jsonPath, []byte(`{"serverlets":[{"name":"api","image":"example/api","ram":128,"cpu":100,"minInstances":1,"maxInstances":2}]}`), 0644); err != nil {
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

	if err := os.WriteFile(jsonPath, []byte(`{"serverlets":[{"name":"api","ram":128.5}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadConfig(dir, "")
	if err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("expected integer validation error, got %v", err)
	}
}

func TestLoadConfigEndToEndFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "futrou.json")
	data := `{
  "$schema": "https://futrou.com/futrou.schema.json",
  "workspace": "acme",
  "project": "website",
  "serverlets": [{"name":"web","image":"nginx:latest","ram":128,"cpu":100,"minInstances":1,"maxInstances":3}],
  "dns": [{"name":"example.com","domain":"example.com","ttl":300,"priority":10}],
  "proxies": [{"domain":"example.com","type":"http","target":"web:8080","port":443}],
  "volumes": [{"name":"uploads"}],
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
	if len(cfg.Serverlets) != 1 || cfg.Serverlets[0].Name != "web" || cfg.Serverlets[0].MaxInstances != 3 {
		t.Fatalf("serverlets were not decoded: %#v", cfg.Serverlets)
	}
	if len(cfg.DNS) != 1 || cfg.DNS[0].Domain != "example.com" || cfg.DNS[0].TTL != 300 || cfg.DNS[0].Priority != 10 {
		t.Fatalf("dns was not decoded: %#v", cfg.DNS)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].Port != 443 || len(cfg.Volumes) != 1 || len(cfg.Crons) != 1 {
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
	source := `export default async () => ({ serverlets: [{ name: "api", image: "example/api", ram: 128, cpu: 100, minInstances: 0, maxInstances: 1 }] })`
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
