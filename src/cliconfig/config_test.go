package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrou-cli/src/constants"
)

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FUTROU_API_URL", "")
	t.Setenv("FUTROU_API_TOKEN", "")
	return filepath.Join(home, constants.ConfigDir, constants.ConfigFile)
}

func TestLoadWithoutAFileReturnsDefaults(t *testing.T) {
	isolatedHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ApiUrl != constants.ApiUrl || len(cfg.ApiTokens) != 0 || len(cfg.DefaultWorkspaces) != 0 {
		t.Fatalf("defaults = %#v", cfg)
	}
}

func TestSaveAndLoadRoundTripWithPrivatePermissions(t *testing.T) {
	path := isolatedHome(t)
	cfg := &CliConfig{ApiUrl: "https://api.example.com"}
	cfg.SetToken("https://api.example.com", "secret-token")
	cfg.SetDefaultWorkspace("https://api.example.com", "ws-1")
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("the credentials file must be private, got %v", info.Mode().Perm())
	}
	raw, _ := os.ReadFile(path)
	if string(raw) == "" || strings.Contains(string(raw), "secret-token") {
		t.Fatalf("the token must not be stored as plaintext: %s", raw)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ApiUrl != "https://api.example.com" || loaded.TokenFor("https://api.example.com") != "secret-token" || loaded.DefaultWorkspaceFor("https://api.example.com") != "ws-1" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestLoadAppliesTheEnvironmentOverrides(t *testing.T) {
	isolatedHome(t)
	if err := Save(&CliConfig{ApiUrl: "https://stored.example.com"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FUTROU_API_URL", "https://env.example.com")
	t.Setenv("FUTROU_API_TOKEN", "env-token")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ApiUrl != "https://env.example.com" || cfg.TokenFor("https://env.example.com") != "env-token" {
		t.Fatalf("env overrides = %#v", cfg)
	}
}

func TestLoadTreatsACorruptFileAsMissing(t *testing.T) {
	path := isolatedHome(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil || cfg.ApiUrl != constants.ApiUrl || len(cfg.ApiTokens) != 0 {
		t.Fatalf("a corrupt file falls back to defaults: %#v, %v", cfg, err)
	}
}

func TestLoadReportsAnUnreadableFile(t *testing.T) {
	path := isolatedHome(t)
	if err := os.MkdirAll(path, 0700); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("a file that cannot be read is an error, not an empty config")
	}
}

func TestDeleteRemovesTheFileAndIsIdempotent(t *testing.T) {
	path := isolatedHome(t)
	if err := Delete(); err != nil {
		t.Fatalf("deleting a missing config is not an error: %v", err)
	}
	if err := Save(&CliConfig{ApiUrl: "https://api.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the file must be gone: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveFailsWhenTheConfigDirectoryCannotBeCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, constants.ConfigDir), []byte("a file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(&CliConfig{}); err == nil {
		t.Fatal("Save must fail when ~/.futrou is a file")
	}
}
