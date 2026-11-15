// Package cliconfig manages the CLI's local credentials and preferences
// (~/.futrou/cli.json) and the login that fills them. It is separate from the
// declarative project config of futrou.json, which lives in package config.
package cliconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"futrou-cli/src/constants"
	"futrou-cli/src/utils"
)

func encryptToken(plaintext string) (string, error) {
	return utils.EncryptToken(plaintext, constants.Name)
}

func decryptToken(stored string) (string, error) {
	return utils.DecryptToken(stored, constants.Name)
}

// CliConfig holds the persisted CLI configuration.
// ApiTokens is keyed by normalized API URL so credentials for different
// Futrou environments (e.g. production vs. self-hosted) don't collide.
type CliConfig struct {
	ApiUrl            string            `json:"apiUrl,omitempty"`
	ApiTokens         map[string]string `json:"apiTokens,omitempty"`
	DefaultWorkspaces map[string]string `json:"defaultWorkspaces,omitempty"`
}

// normalizeUrlKey is the map key of an API URL: the normalized URL, lower-cased,
// so any spelling of the same API ("host", "https://host/v2") shares one entry.
func normalizeUrlKey(apiUrl string) string {
	return strings.ToLower(utils.NormalizeApiUrl(apiUrl))
}

// TokenFor returns the stored API key for the given API URL, if any.
// Tokens encrypted on a different device cannot be decrypted here and
// are treated as absent, prompting the user to log in again.
func (cfg *CliConfig) TokenFor(apiUrl string) string {
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
// key derived from this device's machine ID where available. When the token
// cannot be encrypted it returns an error and stores nothing: a token is never
// kept in plaintext.
func (cfg *CliConfig) SetToken(apiUrl, token string) error {
	encrypted, err := encryptToken(token)
	if err != nil {
		return err
	}
	if cfg.ApiTokens == nil {
		cfg.ApiTokens = map[string]string{}
	}
	cfg.ApiTokens[normalizeUrlKey(apiUrl)] = encrypted
	return nil
}

// DefaultWorkspaceFor returns the stored default workspace ID for the given
// API URL, if any.
func (cfg *CliConfig) DefaultWorkspaceFor(apiUrl string) string {
	if cfg.DefaultWorkspaces == nil {
		return ""
	}
	return cfg.DefaultWorkspaces[normalizeUrlKey(apiUrl)]
}

// SetDefaultWorkspace stores the default workspace ID for the given API URL.
func (cfg *CliConfig) SetDefaultWorkspace(apiUrl, workspaceID string) {
	if cfg.DefaultWorkspaces == nil {
		cfg.DefaultWorkspaces = map[string]string{}
	}
	cfg.DefaultWorkspaces[normalizeUrlKey(apiUrl)] = workspaceID
}

// RemoveApiUrl clears the stored token and default workspace for the given
// API URL, leaving data for other API URLs untouched.
func (cfg *CliConfig) RemoveApiUrl(apiUrl string) {
	key := normalizeUrlKey(apiUrl)
	delete(cfg.ApiTokens, key)
	delete(cfg.DefaultWorkspaces, key)
}

// ClearToken forgets the stored token for the given API URL (a login the API
// no longer accepts) and keeps the rest, such as the default workspace.
func (cfg *CliConfig) ClearToken(apiUrl string) {
	delete(cfg.ApiTokens, normalizeUrlKey(apiUrl))
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.CliConfigDir, constants.CliConfigFile), nil
}

// Load reads config from ~/.futrou/cli.json.
// Env vars FUTROU_API_TOKEN and FUTROU_API_URL take precedence over stored values.
func Load() (*CliConfig, error) {
	cfg, err := readStored()
	if err != nil {
		return nil, err
	}
	if u := os.Getenv(constants.EnvApiUrl); u != "" {
		cfg.ApiUrl = u
	}
	if t := os.Getenv(constants.EnvApiToken); t != "" {
		// An env token that cannot be encrypted is not stored; the caller then
		// reports "not logged in" like for a missing token.
		_ = cfg.SetToken(cfg.ApiUrl, t)
	}
	return cfg, nil
}

// readStored reads cli.json. A missing, unreadable home or corrupt file gives
// the defaults, so callers see "not logged in" rather than a parse failure.
func readStored() (*CliConfig, error) {
	cfg := &CliConfig{ApiUrl: constants.ApiUrl}
	path, err := configPath()
	if err != nil {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return &CliConfig{ApiUrl: constants.ApiUrl}, nil
	}
	if cfg.ApiUrl == "" {
		cfg.ApiUrl = constants.ApiUrl
	}
	return cfg, nil
}

// Save writes config to ~/.futrou/cli.json.
func Save(cfg *CliConfig) error {
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
