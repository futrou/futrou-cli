package commands

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestUpgradeDisabledByEnv(t *testing.T) {
	ts := newTestServer(t)
	for _, v := range []string{"1", "true", "yes"} {
		t.Setenv("FUTROU_UPGRADE_DISABLED", v)
		_, err := runArgs(t, ts, "upgrade")
		if err == nil || err.Error() != errUpgradeViaNpm.Error() {
			t.Fatalf("value %q: expected npm upgrade error, got %v", v, err)
		}
	}
}

func TestUpgradeDisabledFalseValues(t *testing.T) {
	for _, v := range []string{"", "0", "false", "off", " No "} {
		t.Setenv("FUTROU_UPGRADE_DISABLED", v)
		if upgradeDisabled() {
			t.Errorf("value %q should not disable upgrade", v)
		}
	}
}

func TestUpgradeVersionMustBeAVersion(t *testing.T) {
	for _, ok := range []string{"latest", "1.2.3", "v1.2.3", "1.2.3-rc.1", "2.0.0+build_5", "0.0.1"} {
		if err := checkUpgradeVersion(ok); err != nil {
			t.Errorf("%q is a version: %v", ok, err)
		}
	}
	for _, bad := range []string{"1.0; touch pwned", "$(id)", "`id`", "1 2", "-x", "--help", "a|b", "1.0&&id", "1\n2", "'", `"`, "../x", " 1.2.3", ""} {
		if err := checkUpgradeVersion(bad); err == nil {
			t.Errorf("%q must not be accepted as a version", bad)
		}
	}
}

func TestUpgradeRefusesAnUnsafeVersionBeforeRunningAnything(t *testing.T) {
	ts := newTestServer(t)
	ran := false
	old := runUpgradeCommand
	runUpgradeCommand = func(*exec.Cmd) error { ran = true; return nil }
	defer func() { runUpgradeCommand = old }()
	if _, err := runArgs(t, ts, "upgrade", "1.0; touch pwned"); err == nil || !strings.Contains(err.Error(), "not a version") {
		t.Fatalf("err = %v", err)
	}
	if ran {
		t.Fatal("a command ran for an unsafe version")
	}
}

func TestWindowsUpgradeScript(t *testing.T) {
	if got := windowsUpgradeScript("https://x/install.ps1", "latest"); got != "irm https://x/install.ps1 | iex" {
		t.Errorf("latest: %q", got)
	}
	if got := windowsUpgradeScript("https://x/install.ps1", "1.2.3"); !strings.HasSuffix(got, "-Version 1.2.3") {
		t.Errorf("version: %q", got)
	}
}

// installScriptServer serves a script that records its arguments in a file;
// with cut set it promises more bytes than it sends and drops the connection.
func installScriptServer(t *testing.T, marker string, cut bool) *httptest.Server {
	t.Helper()
	script := "#!/bin/bash\nprintf '%s|' \"$@\" > '" + marker + "'\n"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cut {
			w.Header().Set("Content-Length", strconv.Itoa(len(script)+500))
			_, _ = w.Write([]byte(script))
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		_, _ = w.Write([]byte(script))
	}))
}

func TestUnixUpgradeRunsTheDownloadedScriptWithTheVersionAsAnArgument(t *testing.T) {
	for _, tool := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	marker := filepath.Join(t.TempDir(), "args")
	server := installScriptServer(t, marker, false)
	defer server.Close()

	for version, want := range map[string]string{"latest": "|", "1.2.3": "1.2.3|"} {
		os.Remove(marker)
		out, err := exec.Command("bash", unixUpgradeArgs("curl", server.URL, version)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", version, err, out)
		}
		got, _ := os.ReadFile(marker)
		if string(got) != want {
			t.Errorf("%s: the script got arguments %q, want %q", version, got, want)
		}
	}
}

func TestUnixUpgradeNeverRunsAHalfDownloadedScript(t *testing.T) {
	for _, tool := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	marker := filepath.Join(t.TempDir(), "ran")
	server := installScriptServer(t, marker, true)
	defer server.Close()
	if err := exec.Command("bash", unixUpgradeArgs("curl", server.URL, "latest")...).Run(); err == nil {
		t.Error("a cut-off download must fail the upgrade")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the script ran although it was not downloaded whole")
	}
}

func TestUnixUpgradeKeepsTheAddressAndVersionOutOfTheCommandText(t *testing.T) {
	for _, downloader := range []string{"curl", "wget"} {
		args := unixUpgradeArgs(downloader, "https://x/install.sh?a=1;b", "1.2.3")
		if strings.Contains(args[1], "x/install.sh") || strings.Contains(args[1], "1.2.3") {
			t.Errorf("%s: the command text holds the address or version: %s", downloader, args[1])
		}
		if args[len(args)-2] != "https://x/install.sh?a=1;b" || args[len(args)-1] != "1.2.3" {
			t.Errorf("%s: args = %q", downloader, args)
		}
	}
}
