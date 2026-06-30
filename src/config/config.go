package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"futrou-cli/src/constants"
)

// Config holds the persisted CLI configuration.
// ApiKey is keyed by normalized API URL so credentials for different
// Futrou environments (e.g. production vs. self-hosted) don't collide.
type Config struct {
	ApiUrl string            `json:"apiUrl,omitempty"`
	Tokens map[string]string `json:"tokens,omitempty"`

	// ApiKey is deprecated: kept only to migrate older config files that
	// stored a single global token. New writes always use Tokens.
	ApiKey string `json:"apiKey,omitempty"`
}

// normalizeUrlKey lower-cases and trims the URL so it can be used as a
// stable map key regardless of trailing slash or /v2 suffix.
func normalizeUrlKey(apiUrl string) string {
	apiUrl = strings.TrimRight(apiUrl, "/")
	apiUrl = strings.TrimSuffix(apiUrl, "/v2")
	return strings.ToLower(apiUrl)
}

// TokenFor returns the stored API key for the given API URL, if any.
// Tokens encrypted on a different device cannot be decrypted here and
// are treated as absent, prompting the user to log in again.
func (cfg *Config) TokenFor(apiUrl string) string {
	if cfg.Tokens == nil {
		return ""
	}
	stored := cfg.Tokens[normalizeUrlKey(apiUrl)]
	token, err := decryptToken(stored)
	if err != nil {
		return ""
	}
	return token
}

// SetToken stores the API key for the given API URL, encrypted with a
// key derived from this device's machine ID where available.
func (cfg *Config) SetToken(apiUrl, token string) {
	if cfg.Tokens == nil {
		cfg.Tokens = map[string]string{}
	}
	cfg.Tokens[normalizeUrlKey(apiUrl)] = encryptToken(token)
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.ConfigDir, constants.ConfigFile), nil
}

// Load reads config from ~/.futrou/cli.json.
// Env vars FUTROU_API_TOKEN and FUTROU_API_URL take precedence over stored values.
func Load() (*Config, error) {
	cfg := &Config{ApiUrl: constants.DefaultApiUrl}

	path, err := configPath()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// A corrupt/unparseable config file is treated like a missing one: the
	// CLI falls back to defaults instead of surfacing a low-level JSON error,
	// so callers see "not logged in" rather than a parse failure.
	if err := json.Unmarshal(data, cfg); err != nil {
		return &Config{ApiUrl: constants.DefaultApiUrl}, nil
	}

	if cfg.ApiUrl == "" {
		cfg.ApiUrl = constants.DefaultApiUrl
	}

	// Migrate legacy single-token config into the per-URL map.
	if cfg.ApiKey != "" && cfg.TokenFor(cfg.ApiUrl) == "" {
		cfg.SetToken(cfg.ApiUrl, cfg.ApiKey)
	}

	if u := os.Getenv(constants.EnvApiUrl); u != "" {
		cfg.ApiUrl = u
	}
	if t := os.Getenv(constants.EnvApiToken); t != "" {
		cfg.SetToken(cfg.ApiUrl, t)
	}

	cfg.ApiKey = cfg.TokenFor(cfg.ApiUrl)

	return cfg, nil
}

// Save writes config to ~/.futrou/cli.json.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	// Drop the legacy field on write — Tokens is the source of truth going forward.
	cfg.ApiKey = ""

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// Delete removes ~/.futrou/cli.json.
func Delete() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
