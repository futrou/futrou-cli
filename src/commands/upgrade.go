package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

var upgradeCommand = &cli.Command{
	Name:      "upgrade",
	Usage:     "Upgrade Futrou CLI to the latest or a specific version",
	ArgsUsage: "[version]",
	Action: func(c *cli.Context) error {
		version := c.Args().First()
		if version == "" {
			version = "latest"
		}

		switch runtime.GOOS {
		case "windows":
			return runUpgradeWindows(version)
		default:
			return runUpgradeUnix(version)
		}
	},
}

func runUpgradeUnix(version string) error {
	script := constants.UpgradeUnixUrl

	// Check for curl or wget
	var cmd *exec.Cmd
	if path, err := exec.LookPath("curl"); err == nil {
		if version == "latest" {
			cmd = exec.Command("bash", "-c", fmt.Sprintf(`curl -fsSL %s | bash`, script))
		} else {
			cmd = exec.Command("bash", "-c", fmt.Sprintf(`curl -fsSL %s | bash -s %s`, script, version))
		}
		_ = path
	} else if path, err := exec.LookPath("wget"); err == nil {
		if version == "latest" {
			cmd = exec.Command("bash", "-c", fmt.Sprintf(`wget -qO- %s | bash`, script))
		} else {
			cmd = exec.Command("bash", "-c", fmt.Sprintf(`wget -qO- %s | bash -s %s`, script, version))
		}
		_ = path
	} else {
		return fmt.Errorf("curl or wget is required to upgrade — install one and try again")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	return nil
}

func runUpgradeWindows(version string) error {
	script := constants.UpgradeWindowsUrl

	var psCmd string
	if version == "latest" {
		psCmd = fmt.Sprintf(`irm %s | iex`, script)
	} else {
		psCmd = fmt.Sprintf(`& ([scriptblock]::Create((irm %s))) -Version %s`, script, version)
	}

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	return nil
}
