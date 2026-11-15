package utils

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunShellStreamsStdoutAndStderrSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	var out, errOut bytes.Buffer
	if err := RunShell(t.TempDir(), "echo one; echo two 1>&2; echo three", &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if out.String() != "one\nthree\n" || errOut.String() != "two\n" {
		t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestRunShellRunsInTheGivenFolderWithTheEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	dir := t.TempDir()
	t.Setenv("FUTROU_SHELL_TEST", "value")
	var out bytes.Buffer
	if err := RunShell(dir, "pwd; echo $FUTROU_SHELL_TEST; echo x > made.txt", &out, &out); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(out.String(), real) || !strings.Contains(out.String(), "value") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "made.txt")); err != nil {
		t.Fatal("the command must run in the folder")
	}
}

func TestRunShellReportsTheExitStatusAndAMissingFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	var out bytes.Buffer
	err := RunShell(t.TempDir(), "echo before; exit 7", &out, &out)
	if err == nil || !strings.Contains(err.Error(), "exit status 7") || !strings.Contains(out.String(), "before") {
		t.Fatalf("err = %v out = %q", err, out.String())
	}
	if err := RunShell(filepath.Join(t.TempDir(), "missing"), "true", &out, &out); err == nil {
		t.Fatal("a folder that does not exist is an error")
	}
	if err := RunShell(t.TempDir(), "definitely-not-a-command-xyz", &out, &out); err == nil {
		t.Fatal("an unknown command fails")
	}
}
