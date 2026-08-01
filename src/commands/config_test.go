package commands

import (
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
