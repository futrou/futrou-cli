package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"futrou-cli/src/constants"

	"github.com/urfave/cli/v2"
)

var upgradeCommand = &cli.Command{
	Name:      "upgrade",
	Usage:     "Upgrade Futrou CLI to the latest or a specific version",
	ArgsUsage: "[version]",
	Action: func(c *cli.Context) error {
		if upgradeDisabled() {
			return errUpgradeViaNpm
		}
		version := c.Args().First()
		if version == "" {
			version = "latest"
		}
		if err := checkUpgradeVersion(version); err != nil {
			return err
		}

		switch runtime.GOOS {
		case "windows":
			return runUpgradeWindows(version)
		default:
			return runUpgradeUnix(version)
		}
	},
}

var errUpgradeViaNpm = errors.New("this Futrou CLI was installed through npm, so it cannot upgrade itself — run `npm install -g futrou@latest` (or use `npx futrou@latest`) instead")

// upgradeDisabled reports whether FUTROU_UPGRADE_DISABLED is set to a true value
// (anything but empty, 0 or false), as the npm wrapper does.
func upgradeDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(constants.EnvUpgradeDisabled))) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// versionPattern is what a version argument may look like: "1.2.3", "v1.2.3",
// "1.2.3-rc.1". It keeps anything a shell would read as syntax out of the
// install script's command line.
var versionPattern = regexp.MustCompile(`^v?[0-9A-Za-z][0-9A-Za-z.+_-]*$`)

// checkUpgradeVersion accepts "latest" or a version number.
func checkUpgradeVersion(version string) error {
	if version == "latest" || versionPattern.MatchString(version) {
		return nil
	}
	return fmt.Errorf("%q is not a version: use latest or a number such as 1.2.3", version)
}

// unixUpgradeArgs is the bash command that downloads the install script with
// downloader (curl or wget) and runs it. The script is downloaded whole before
// bash sees it, so a connection that drops halfway never runs half a script,
// and the address and the version are passed as arguments, never put into the
// command text.
func unixUpgradeArgs(downloader, script, version string) []string {
	fetch := `curl -fsSL "$1"`
	if downloader == "wget" {
		fetch = `wget -qO- "$1"`
	}
	run := `printf '%s\n' "$body" | bash`
	args := []string{"-c", `body=$(` + fetch + `) && ` + run, "futrou-upgrade", script}
	if version != "latest" {
		args[1] = `body=$(` + fetch + `) && printf '%s\n' "$body" | bash -s -- "$2"`
		args = append(args, version)
	}
	return args
}

func runUpgradeUnix(version string) error {
	downloader := ""
	switch {
	case hasCommand("curl"):
		downloader = "curl"
	case hasCommand("wget"):
		downloader = "wget"
	default:
		return fmt.Errorf("curl or wget is required to upgrade — install one and try again")
	}
	return runUpgradeCommand(exec.Command("bash", unixUpgradeArgs(downloader, constants.UpgradeUnixUrl, version)...))
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// windowsUpgradeScript is the PowerShell the Windows upgrade runs; version has
// passed checkUpgradeVersion.
func windowsUpgradeScript(script, version string) string {
	if version == "latest" {
		return fmt.Sprintf(`irm %s | iex`, script)
	}
	return fmt.Sprintf(`& ([scriptblock]::Create((irm %s))) -Version %s`, script, version)
}

func runUpgradeWindows(version string) error {
	return runUpgradeCommand(exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", windowsUpgradeScript(constants.UpgradeWindowsUrl, version)))
}

// runUpgradeCommand runs the install script's command on the terminal; tests replace it.
var runUpgradeCommand = func(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	return nil
}
