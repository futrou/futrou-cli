package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"futrou-cli/src/constants"
)

// Config holds the persisted CLI configuration.
type Config struct {
	ApiUrl string `json:"apiUrl,omitempty"`
	ApiKey string `json:"apiKey,omitempty"`
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

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.ApiUrl == "" {
		cfg.ApiUrl = constants.DefaultApiUrl
	}

	if t := os.Getenv(constants.EnvApiToken); t != "" {
		cfg.ApiKey = t
	}
	if u := os.Getenv(constants.EnvApiUrl); u != "" {
		cfg.ApiUrl = u
	}

	return cfg, nil
}

// Save writes config to ~/.futrou/config.json.
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

// Delete removes ~/.futrou/config.json.
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
