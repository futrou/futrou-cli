package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"futrou-cli/src/constants"
	"futrou-cli/src/utils"
)

func encryptToken(plaintext string) string {
	return utils.EncryptToken(plaintext, constants.Name)
}

func decryptToken(stored string) (string, error) {
	return utils.DecryptToken(stored, constants.Name)
}

// Config holds the persisted CLI configuration.
// ApiTokens is keyed by normalized API URL so credentials for different
// Futrou environments (e.g. production vs. self-hosted) don't collide.
type Config struct {
	ApiUrl            string            `json:"apiUrl,omitempty"`
	ApiTokens         map[string]string `json:"apiTokens,omitempty"`
	DefaultWorkspaces map[string]string `json:"defaultWorkspaces,omitempty"`
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
	if cfg.ApiTokens == nil {
		return ""
	}
	stored := cfg.ApiTokens[normalizeUrlKey(apiUrl)]
	token, err := decryptToken(stored)
	if err != nil {
		return ""
	}
	return token
}

// SetToken stores the API key for the given API URL, encrypted with a
// key derived from this device's machine ID where available.
func (cfg *Config) SetToken(apiUrl, token string) {
	if cfg.ApiTokens == nil {
		cfg.ApiTokens = map[string]string{}
	}
	cfg.ApiTokens[normalizeUrlKey(apiUrl)] = encryptToken(token)
}

// DefaultWorkspaceFor returns the stored default workspace ID for the given
// API URL, if any.
func (cfg *Config) DefaultWorkspaceFor(apiUrl string) string {
	if cfg.DefaultWorkspaces == nil {
		return ""
	}
	return cfg.DefaultWorkspaces[normalizeUrlKey(apiUrl)]
}

// SetDefaultWorkspace stores the default workspace ID for the given API URL.
func (cfg *Config) SetDefaultWorkspace(apiUrl, workspaceID string) {
	if cfg.DefaultWorkspaces == nil {
		cfg.DefaultWorkspaces = map[string]string{}
	}
	cfg.DefaultWorkspaces[normalizeUrlKey(apiUrl)] = workspaceID
}

// RemoveApiUrl clears the stored token and default workspace for the given
// API URL, leaving data for other API URLs untouched.
func (cfg *Config) RemoveApiUrl(apiUrl string) {
	key := normalizeUrlKey(apiUrl)
	delete(cfg.ApiTokens, key)
	delete(cfg.DefaultWorkspaces, key)
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

	if u := os.Getenv(constants.EnvApiUrl); u != "" {
		cfg.ApiUrl = u
	}
	if t := os.Getenv(constants.EnvApiToken); t != "" {
		cfg.SetToken(cfg.ApiUrl, t)
	}

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
