package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildProject writes a config with the commands into a new folder and makes it the working folder.
func buildProject(t *testing.T, config string, files map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the commands of these tests are for sh")
	}
	dir := t.TempDir()
	writeTree(t, dir, files)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, dir)
	return dir
}

func captureCommandOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	oldOut, oldErr := commandStdout, commandStderr
	commandStdout, commandStderr = &out, &errOut
	t.Cleanup(func() { commandStdout, commandStderr = oldOut, oldErr })
	return &out, &errOut
}

func lines(path string) []string {
	data, _ := os.ReadFile(path)
	return strings.Fields(string(data))
}

func TestBuildCommandRunsBeforeEveryDeployInTheFolderOfTheConfig(t *testing.T) {
	ts, c, fake := deployStaticoServer(t)
	dir := buildProject(t, `{"workspace": "acme", "project": "demo", "buildCommand": "echo built >> runs.txt && mkdir -p dist && echo hi > dist/index.html",
		"statico": {"docs": {"htmlToFolders": false, "include": {"./dist": true}}}}`, nil)
	out, errOut := captureCommandOutput(t)
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs"); got["index.html"] != "hi\n" {
		t.Fatalf("the built files were not deployed: %v", got)
	}
	_, err = runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	if n := len(lines(filepath.Join(dir, "runs.txt"))); n != 2 {
		t.Fatalf("the build runs before every deploy, ran %d times", n)
	}
	if !strings.Contains(out.String(), "Building: echo built") || errOut.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", out, errOut)
	}
	_ = c
}

func TestBuildCommandOutputIsStreamedFromStdoutAndStderr(t *testing.T) {
	ts, _, _ := deployStaticoServer(t)
	buildProject(t, `{"workspace": "acme", "project": "demo", "buildCommand": "echo to-stdout; echo to-stderr 1>&2"}`, nil)
	out, errOut := captureCommandOutput(t)
	_, err := runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	if !strings.Contains(out.String(), "\nto-stdout\n") || strings.Contains(out.String(), "\nto-stderr\n") {
		t.Fatalf("stdout = %q", out)
	}
	if errOut.String() != "to-stderr\n" {
		t.Fatalf("stderr = %q", errOut)
	}
}

func TestInstallCommandRunsOnceAndAgainWhenItChanges(t *testing.T) {
	ts, _, _ := deployStaticoServer(t)
	dir := buildProject(t, `{"workspace": "acme", "project": "demo", "installCommand": "echo installed >> installs.txt", "buildCommand": "echo b >> builds.txt"}`, nil)
	out, _ := captureCommandOutput(t)
	for i := 0; i < 3; i++ {
		_, err := runArgs(t, ts, "deploy", "--dry-run")
		assertNoError(t, err)
	}
	if n := len(lines(filepath.Join(dir, "installs.txt"))); n != 1 {
		t.Fatalf("install ran %d times, once is enough", n)
	}
	if n := len(lines(filepath.Join(dir, "builds.txt"))); n != 3 {
		t.Fatalf("build ran %d times, want every time", n)
	}
	if !strings.Contains(out.String(), "Already installed on this device") {
		t.Fatalf("stdout = %q", out)
	}
	// a changed command is a new install
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace": "acme", "project": "demo", "installCommand": "echo reinstalled >> installs.txt"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	if got := lines(filepath.Join(dir, "installs.txt")); len(got) != 2 || got[1] != "reinstalled" {
		t.Fatalf("installs = %v", got)
	}
}

func TestInstallIsRememberedPerProjectFolder(t *testing.T) {
	ts, _, _ := deployStaticoServer(t)
	config := `{"workspace": "acme", "project": "demo", "installCommand": "echo x >> installs.txt"}`
	first := buildProject(t, config, nil)
	captureCommandOutput(t)
	_, err := runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	second := buildProject(t, config, nil)
	_, err = runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	if len(lines(filepath.Join(first, "installs.txt"))) != 1 || len(lines(filepath.Join(second, "installs.txt"))) != 1 {
		t.Fatal("each folder is installed once")
	}
}

func TestFailedInstallIsNotRememberedAndStopsTheDeploy(t *testing.T) {
	ts, c, fake := deployStaticoServer(t)
	dir := buildProject(t, `{"workspace": "acme", "project": "demo", "installCommand": "echo try >> installs.txt; exit 3", "statico": {"docs": {}}}`, map[string]string{"index.html": "i"})
	captureCommandOutput(t)
	for i := 0; i < 2; i++ {
		_, err := runArgs(t, ts, "deploy", "--yes")
		assertError(t, err)
		if !strings.Contains(err.Error(), "installCommand") || !strings.Contains(err.Error(), "exit status 3") {
			t.Fatalf("err = %v", err)
		}
	}
	if n := len(lines(filepath.Join(dir, "installs.txt"))); n != 2 {
		t.Fatalf("a failed install is tried again, ran %d times", n)
	}
	if len(c.mutations()) != 0 || len(fake.Staticos) != 0 || len(c.log) != 0 {
		t.Fatalf("nothing may reach the API: %v", c.log)
	}
}

func TestFailedBuildStopsTheDeployBeforeAnyRequest(t *testing.T) {
	ts, c, fake := deployStaticoServer(t)
	buildProject(t, `{"workspace": "acme", "project": "demo", "buildCommand": "echo broken 1>&2; exit 1", "statico": {"docs": {}}}`, map[string]string{"index.html": "i"})
	_, errOut := captureCommandOutput(t)
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	if !strings.Contains(err.Error(), "buildCommand") || !strings.Contains(errOut.String(), "broken") {
		t.Fatalf("err = %v stderr = %q", err, errOut)
	}
	if len(c.log) != 0 || len(fake.Staticos) != 0 || len(fake.Log) != 0 {
		t.Fatalf("a failed build stops everything: %v %v", c.log, fake.Log)
	}
}

func TestBuildRunsBeforeLoginIsNeeded(t *testing.T) {
	ts, _, _ := deployStaticoServer(t)
	dir := buildProject(t, `{"workspace": "acme", "project": "demo", "buildCommand": "echo b >> builds.txt"}`, nil)
	captureCommandOutput(t)
	_, err := runArgsNoAuth(t, ts, "deploy", "--dry-run")
	assertError(t, err) // not logged in
	if len(lines(filepath.Join(dir, "builds.txt"))) != 1 {
		t.Fatal("the build runs before anything else, login included")
	}
}

func TestStaticoUpRunsInstallAndBuildToo(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := buildProject(t, `{"installCommand": "echo i >> installs.txt", "buildCommand": "mkdir -p dist && echo built > dist/index.html"}`, nil)
	captureCommandOutput(t)
	fake.AddStatico("docs", nil)
	for i := 0; i < 2; i++ {
		_, err := runArgs(t, ts, "statico", "up", "--html-to-folders=false", "--include", "./dist", "docs")
		assertNoError(t, err)
	}
	if got := fake.ActiveFiles("docs"); got["index.html"] != "built\n" {
		t.Fatalf("served = %v", got)
	}
	if len(lines(filepath.Join(dir, "installs.txt"))) != 1 {
		t.Fatal("install once")
	}
}

func TestStaticoUpWithoutConfigCommandsDoesNothingExtra(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	out, errOut := captureCommandOutput(t)
	fake.AddStatico("docs", nil)
	_, err := runArgs(t, ts, "statico", "up", "--include", ".", "docs")
	assertNoError(t, err)
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("no commands, no output: %q %q", out, errOut)
	}
}

func TestStaticoUpStopsWhenTheBuildFails(t *testing.T) {
	ts, fake := staticoServer(t)
	buildProject(t, `{"buildCommand": "exit 2"}`, map[string]string{"index.html": "i"})
	captureCommandOutput(t)
	_, err := runArgs(t, ts, "statico", "up", "--include", ".", "docs")
	assertError(t, err)
	if len(fake.Log) != 0 || len(fake.Staticos) != 0 {
		t.Fatalf("nothing may reach the API: %v", fake.Log)
	}
}

func TestCommandOutputGoesToStderrInJSONMode(t *testing.T) {
	ts, _, _ := deployStaticoServer(t)
	buildProject(t, `{"workspace": "acme", "project": "demo", "buildCommand": "echo building"}`, nil)
	out, errOut := captureCommandOutput(t)
	_, err := runArgs(t, ts, "--log-format", "json", "deploy", "--dry-run")
	assertNoError(t, err)
	if out.Len() != 0 || !strings.Contains(errOut.String(), "building") {
		t.Fatalf("stdout must stay free for JSON: stdout %q stderr %q", out, errOut)
	}
}

func TestProjectCommandsValidation(t *testing.T) {
	for name, config := range map[string]string{
		"build is a list":      `{"workspace": "acme", "project": "demo", "buildCommand": ["npm", "run"]}`,
		"install is blank":     `{"workspace": "acme", "project": "demo", "installCommand": "   "}`,
		"build is a number":    `{"workspace": "acme", "project": "demo", "buildCommand": 5}`,
		"install is an object": `{"workspace": "acme", "project": "demo", "installCommand": {"run": "npm ci"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			ts, c, _ := deployStaticoServer(t)
			buildProject(t, config, nil)
			_, err := runArgs(t, ts, "deploy", "--dry-run")
			assertError(t, err)
			if len(c.log) != 0 {
				t.Fatalf("an invalid config must not run or request anything: %v", c.log)
			}
		})
	}
}
