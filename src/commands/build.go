package commands

import (
	"fmt"
	"io"
	"os"

	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

// Where the output of installCommand and buildCommand goes; tests replace them.
var (
	commandStdout io.Writer = os.Stdout
	commandStderr io.Writer = os.Stderr
)

// runProjectCommands runs what the config asks for before a deploy does
// anything: installCommand once per device (again when it changes), then
// buildCommand every time, both in the folder of the config file, as child
// processes whose output is streamed as it is written. A failing command stops
// the deploy. With --log-format json the output of the commands goes to stderr,
// so stdout stays JSON.
func runProjectCommands(c *cli.Context, cfg *projectconfig.Config) error {
	if cfg == nil || (cfg.InstallCommand == "" && cfg.BuildCommand == "") {
		return nil
	}
	dir := cfg.BaseDir()
	out := commandStdout
	if isJSON(c) {
		out = commandStderr
	}
	if cfg.InstallCommand != "" {
		key := cfg.InstallCommand
		if utils.CommandDone(dir, key) {
			fmt.Fprintf(out, "Already installed on this device (installCommand: %s)\n", cfg.InstallCommand)
		} else {
			fmt.Fprintf(out, "Installing: %s\n", cfg.InstallCommand)
			if err := utils.RunShell(dir, cfg.InstallCommand, out, commandStderr); err != nil {
				return fmt.Errorf("installCommand %q failed: %w", cfg.InstallCommand, err)
			}
			utils.MarkCommandDone(dir, key)
		}
	}
	if cfg.BuildCommand != "" {
		fmt.Fprintf(out, "Building: %s\n", cfg.BuildCommand)
		if err := utils.RunShell(dir, cfg.BuildCommand, out, commandStderr); err != nil {
			return fmt.Errorf("buildCommand %q failed: %w", cfg.BuildCommand, err)
		}
	}
	return nil
}
