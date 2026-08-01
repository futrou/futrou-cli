// Package cliconfig manages the CLI's local credentials and preferences.
// It is deliberately separate from the declarative project config package.
package cliconfig

import projectconfig "futrou-cli/src/config"

// CliConfig is the persisted local CLI configuration.
type CliConfig = projectconfig.CliConfig

func Load() (*CliConfig, error) { return projectconfig.Load() }
func Save(cfg *CliConfig) error { return projectconfig.Save(cfg) }
func Delete() error             { return projectconfig.Delete() }
