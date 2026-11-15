package commands

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/statico"
	"futrou-cli/src/statico/mock"
)

// staticoServer is a test server with the statico and storage-file endpoints
// of Futrou Cloud; it fails the test for any request without the CLI's token.
func staticoServer(t *testing.T) (*testServer, *mock.Server) {
	t.Helper()
	ts := newTestServer(t)
	writeCLIConfig(t, ts.URL, mock.WorkspaceID) // the login default, as for a logged-in user
	fake := mock.New()
	ts.fallback = fake.Handle
	t.Cleanup(func() {
		if fake.Unauthorized != 0 {
			t.Errorf("%d request(s) were not authorised with the CLI's API token", fake.Unauthorized)
		}
	})
	return ts, fake
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStaticoLsShowAndVersions(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i"})
	withWorkingDirectory(t, t.TempDir())

	out, err := runArgs(t, ts, "statico", "ls")
	assertNoError(t, err)
	assertContains(t, out, "docs")

	out, err = runArgs(t, ts, "statico", "show", "docs")
	assertNoError(t, err)
	assertContains(t, out, `"activeVersion": "v1"`)
	assertContains(t, out, `"versions"`)

	assertContains(t, out, `"status": "completed"`)
}

func TestStaticoHiddenPluralAlias(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	withWorkingDirectory(t, t.TempDir())
	out, err := runArgs(t, ts, "staticos", "list")
	assertNoError(t, err)
	assertContains(t, out, "docs")
}

func TestStaticoRemove(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	withWorkingDirectory(t, t.TempDir())
	_, err := runArgs(t, ts, "statico", "rm", "docs")
	assertNoError(t, err)
	if len(fake.Staticos) != 0 {
		t.Fatal("the website was not deleted")
	}
}

func TestStaticoCommandErrors(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	withWorkingDirectory(t, t.TempDir())
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"unknown website":          {[]string{"statico", "show", "nope"}, "not found"},
		"deploy an empty folder":   {[]string{"statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--include", ".", "docs"}, "matches no files"},
		"deploy without include":   {[]string{"statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "docs"}, "no files or folders were included"},
		"deploy only an exclude":   {[]string{"statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--exclude", "*.map", "docs"}, "no files or folders were included"},
		"deploy missing include":   {[]string{"statico", "deploy", "--include", "nope", "docs"}, "does not exist"},
		"activate needs --version": {[]string{"statico", "activate", "docs"}, "version"},
		"activate unknown version": {[]string{"statico", "activate", "--version", "nope", "docs"}, ""},
		"download unknown version": {[]string{"statico", "download", "--version", "nope", "docs"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runArgs(t, ts, tc.args...)
			assertError(t, err)
			if tc.want != "" && !strings.Contains(out+err.Error(), tc.want) {
				t.Fatalf("output %q / error %q, want %q", out, err, tc.want)
			}
		})
	}
}

func TestStaticoLsRequiresLogin(t *testing.T) {
	ts, _ := staticoServer(t)
	_, err := runArgsNoAuth(t, ts, "statico", "ls")
	assertError(t, err)
}

func TestStaticoUploadSendsTheIncludedFolderAndActivates(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"dist/index.html": "<h1>", "dist/app.js": "js", "dist/app.js.map": "map", "dist/img/a.png": "png"})
	withWorkingDirectory(t, dir)

	out, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./dist:./", "--exclude", "./dist/*.map", "docs")
	assertNoError(t, err)
	assertContains(t, out, "is live at https://"+mock.PublicDomain(fake.Statico("docs"))+" (3 uploaded, 0 deleted, 0 unchanged)")
	got := fake.ActiveFiles("docs")
	if len(got) != 3 || got["index.html"] != "<h1>" || got["img/a.png"] != "png" {
		t.Fatalf("served = %v", got)
	}
	if _, err := os.Stat(".futrou"); !os.IsNotExist(err) {
		t.Fatal(".futrou must be removed after an upload that worked")
	}
}

func TestStaticoUploadIncludeOfAFolderWithoutADestinationIsItsFilesAtTheRoot(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"dist/index.html": "i", "other/a.txt": "a"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./dist", "--include", "./other/a.txt:renamed.txt", "docs")
	assertNoError(t, err)
	want := map[string]string{"index.html": "i", "renamed.txt": "a"}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}

func TestStaticoUploadExcludesRunAfterTheIncludes(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"src/a.js": "a", "src/b.css": "b", "src/c.html": "c", "src/img/d.jpg": "d"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./src:./", "--exclude", "./src/*.{js,css}", "--exclude", "./src/img", "docs")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, map[string]string{"c.html": "c"}) {
		t.Fatalf("served = %v", got)
	}
}

func TestStaticoUploadIncludesAreRunInTheOrderGiven(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a/index.html": "from a", "b/index.html": "from b"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./a:./", "--include", "./b:./", "docs")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs")["index.html"]; got != "from b" {
		t.Fatalf("the later include wins: %q", got)
	}
}

func TestStaticoFlagValuesWithCommasAreNotSplit(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a,b/index.html": "i", "a.js": "x", "a.css": "y", "a.txt": "z"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./a,b:./", "--include", "./a.{js,css,txt}:./", "--exclude", "./a.{js,css}", "docs")
	assertNoError(t, err)
	want := map[string]string{"index.html": "i", "a.txt": "z"}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}

func TestStaticoUploadPositionalPathsAndAnswers(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"dist/index.html": "i"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--version", "release-1", "--default-file", "/home.html", "--default-status", "200",
		"--fallback-file", "/index.html", "--fallback-status", "200", "--include", "./dist:./", "docs")
	assertNoError(t, err)
	v := fake.Statico("docs").Versions[0]
	if v.Name != "release-1" || v.Extra["defaultFile"] != "/home.html" || v.Extra["fallbackFile"] != "/index.html" || v.Extra["fallbackStatus"] != float64(200) || v.Extra["defaultStatus"] != float64(200) {
		t.Fatalf("version = %+v", v)
	}
}

func TestStaticoCommandsAreDeployVersionDownloadAndTheWebsiteBasics(t *testing.T) {
	var names []string
	for _, cmd := range staticoCommand.Subcommands {
		names = append(names, cmd.Name)
	}
	if want := []string{"ls", "show", "rm", "deploy", "version", "activate", "download"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("commands = %v, want %v (upload, sync and create are part of deploy)", names, want)
	}
}

func TestStaticoDeployFlagsHaveNoOldNames(t *testing.T) {
	for _, cmd := range staticoCommand.Subcommands {
		if cmd.Name != "deploy" {
			continue
		}
		for _, f := range cmd.Flags {
			for _, name := range f.Names() {
				switch name {
				case "no-activate", "no-html-to-folders", "spa", "keep-extra", "redirects", "direction":
					t.Errorf("--%s was replaced", name)
				}
			}
		}
	}
}

func TestStaticoUploadCopiesPagesAsFolderIndexesUnlessTurnedOff(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	fake.AddStatico("plain", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"site/index.html": "home", "site/about.html": "about"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--include", "./site:./", "docs")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs"); got["about/index.html"] != "about" || got["about.html"] != "about" {
		t.Fatalf("served = %v", got)
	}
	_, err = runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./site:./", "plain")
	assertNoError(t, err)
	if got := fake.ActiveFiles("plain"); len(got) != 2 {
		t.Fatalf("--no-html-to-folders: served = %v", got)
	}
}

func TestStaticoUploadReadsTheRedirectsFile(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"site/index.html": "i", "site/_redirects": "/old /new 301\n/blog/* /news/:splat\n/x /y 200\n"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./site:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "warning: _redirects line 3")
	if _, served := fake.ActiveFiles("docs")["_redirects"]; served {
		t.Fatal("_redirects must not be uploaded")
	}
	if redirects, _ := fake.Statico("docs").Versions[0].Extra["redirects"].([]interface{}); len(redirects) != 2 {
		t.Fatalf("redirects = %v", fake.Statico("docs").Versions[0].Extra)
	}
}

func TestStaticoUploadShowsProgressForEveryFile(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "aaaa", "b.txt": "bb"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".:./", "docs")
	assertNoError(t, err)
	for _, want := range []string{"uploading 2 file(s)", "uploading a.txt", "uploading b.txt", "uploaded a.txt: ", "files, ", "100%"} {
		assertContains(t, out, want)
	}
	if strings.Contains(out, "\033[") {
		t.Fatalf("a captured output is not a terminal: no escape codes:\n%q", out)
	}
}

func TestStaticoUploadClearsFutrouAndKeepsTheStagedFilesOfAFailure(t *testing.T) {
	ts, fake := staticoServer(t)
	_ = fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i", ".futrou/statico/old/v1/x": "stale", ".futrou/tmp/y": "stale"})
	withWorkingDirectory(t, dir)
	fake.FailWrites = 500
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".:./", "--exclude", ".futrou", "docs")
	assertError(t, err)
	if _, statErr := os.Stat(filepath.Join(dir, ".futrou", "statico", "old")); !os.IsNotExist(statErr) {
		t.Fatal("the leftovers of earlier runs must be cleared first")
	}
	if b, readErr := os.ReadFile(filepath.Join(dir, ".futrou", "statico", "docs", "v1", "index.html")); readErr != nil || string(b) != "i" {
		t.Fatalf("the staged files of a failure stay for inspection: %q %v", b, readErr)
	}
	if !strings.Contains(err.Error(), filepath.Join(".futrou", "statico", "docs", "v1")) {
		t.Fatalf("err = %v", err)
	}
}

func TestStaticoUploadSecondRunIsUpToDateAndSendsNothing(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--include", ".", "docs")
	assertNoError(t, err)
	puts := fake.Count("PUT", "/v2/storages/")
	out, err := runArgs(t, ts, "statico", "deploy", "--include", ".", "docs")
	assertNoError(t, err)
	assertContains(t, out, "already up to date")
	if fake.Count("PUT", "/v2/storages/") != puts || len(fake.Statico("docs").Versions) != 1 {
		t.Fatal("an unchanged folder must send nothing and create no version")
	}
}

func TestStaticoUploadDryRunChangesNothing(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"old.txt": "o"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"new.txt": "n"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--dry-run", "--include", ".", "docs")
	assertNoError(t, err)
	assertContains(t, out, "↑ new.txt")
	assertContains(t, out, "✗ old.txt")
	if len(fake.Statico("docs").Versions) != 1 || fake.Count("POST", "/v2/staticos/") != 0 {
		t.Fatal("a dry run changed something")
	}
}

func TestStaticoDeployNotActivatedThenServedByVersion(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "v2"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--activate=false", "--include", ".", "docs")
	assertNoError(t, err)
	assertContains(t, out, "not activated")
	if fake.ActiveFiles("docs")["index.html"] != "v1" {
		t.Fatal("--activate=false changed what is served")
	}
	// naming an existing version serves it, without an upload
	puts := fake.Count("PUT", "/v2/storages/") + fake.Archives
	out, err = runArgs(t, ts, "statico", "deploy", "--version", "v2", "docs")
	assertNoError(t, err)
	assertContains(t, out, "version v2 is live")
	if fake.ActiveFiles("docs")["index.html"] != "v2" || fake.Count("PUT", "/v2/storages/")+fake.Archives != puts {
		t.Fatal("the version was not served, or files were sent again")
	}
}

func TestStaticoDeployWithANewVersionNameUploadsIt(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "named"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--version", "release-9", "--include", ".", "docs")
	assertNoError(t, err)
	if fake.Statico("docs").Versions[len(fake.Statico("docs").Versions)-1].Name != "release-9" || fake.ActiveFiles("docs")["index.html"] != "named" {
		t.Fatalf("versions = %+v", fake.Statico("docs").Versions)
	}
}

func TestStaticoDeployBooleanFlagsDefaultToTrue(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"about.html": "a"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--activate=true", "--html-to-folders=true", "--include", ".:./", "docs")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs"); got["about/index.html"] != "a" {
		t.Fatalf("served = %v", got)
	}
}

func TestStaticoActivateServesTheVersionNamed(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	for _, content := range []string{"v2", "v3"} {
		writeTree(t, dir, map[string]string{"index.html": content})
		_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "index.html", "docs")
		assertNoError(t, err)
	}
	out, err := runArgs(t, ts, "statico", "activate", "--version", "v1", "docs")
	assertNoError(t, err)
	assertContains(t, out, "version v1 is live at https://")
	if fake.ActiveFiles("docs")["index.html"] != "v1" {
		t.Fatalf("served = %v", fake.ActiveFiles("docs"))
	}
	_, err = runArgs(t, ts, "statico", "activate", "--version", "v3", "docs")
	assertNoError(t, err)
	if fake.ActiveFiles("docs")["index.html"] != "v3" {
		t.Fatal("any version can be served, forward as well as back")
	}
	out, err = runArgs(t, ts, "--log-format", "json", "statico", "activate", "--version", "v2", "docs")
	assertNoError(t, err)
	assertContains(t, out, `\"version\": \"v2\"`)
}

// The command the CLI prints for serving a version that was not activated must
// work when it is typed as printed.
func TestStaticoNotActivatedHintIsACommandThatWorks(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "v9"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--activate=false", "--version", "v9", "--include", ".", "docs")
	assertNoError(t, err)
	start := strings.Index(out, "(futrou ")
	end := start + strings.Index(out[start:], ")")
	if start < 0 || end < start {
		t.Fatalf("no command in the message:\n%s", out)
	}
	hint := out[start+1 : end]
	if want := statico.ActivateHint("docs", "v9"); hint != want {
		t.Fatalf("hint = %q, want %q", hint, want)
	}
	args := strings.Fields(hint)
	if args[0] != "futrou" {
		t.Fatalf("hint = %q", hint)
	}
	out, err = runArgs(t, ts, args[1:]...)
	assertNoError(t, err)
	assertContains(t, out, "version v9 is live")
	if fake.ActiveFiles("docs")["index.html"] != "v9" {
		t.Fatal("the printed command did not serve the version")
	}
}

// `version ls` and `version activate` are the names; `versions`, `activate`
// and a bare `version` keep working.
func TestStaticoVersionGroupAndItsOlderNames(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "v2"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--include", ".", "docs")
	assertNoError(t, err)
	for _, args := range [][]string{
		{"statico", "version", "ls", "docs"},
		{"statico", "version", "docs"},
		{"statico", "versions", "docs"},
		{"statico", "versions", "ls", "docs"},
	} {
		out, err := runArgs(t, ts, args...)
		assertNoError(t, err)
		assertContains(t, out, "NUMBER")
	}
	for _, args := range [][]string{
		{"statico", "version", "activate", "--version", "v1", "docs"},
		{"statico", "activate", "--version", "v1", "docs"},
		{"statico", "versions", "activate", "--version", "v1", "docs"},
	} {
		_, err := runArgs(t, ts, "statico", "version", "activate", "--version", "v2", "docs")
		assertNoError(t, err)
		out, err := runArgs(t, ts, args...)
		assertNoError(t, err)
		assertContains(t, out, "version v1 is live")
		if fake.ActiveFiles("docs")["index.html"] != "v1" {
			t.Fatalf("%v did not serve v1", args)
		}
	}
	if _, err := runArgs(t, ts, "statico", "version", "activate", "docs"); err == nil {
		t.Fatal("activate without --version must fail")
	}
	for _, cmd := range staticoCommand.Subcommands {
		if cmd.Name == "activate" && !cmd.Hidden {
			t.Error("the old `statico activate` must stay hidden")
		}
	}
}

func TestStaticoUploadUsesTheDeclaredWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	st := fake.AddStatico("docs", nil)
	st.ProjectID, st.WorkspaceID = testProjectID, testWorkspaceID
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"public/index.html": "i", "public/skip.map": "m", "other.txt": "o"})
	config := `{"statico": {"docs": {"htmlToFolders": false, "fallbackFile": "/index.html", "fallbackStatus": 200,
		"redirects": [{"from": "/old", "to": "/new", "statusCode": 301}], "include": {"./public": "./", "./public/*.map": false}}}}`
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, dir)
	ts.on("GET", "/v2/workspaces", respond(200, []any{map[string]any{"id": testWorkspaceID, "name": "acme"}}))
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "docs")
	assertNoError(t, err)
	got := fake.ActiveFiles("docs")
	if len(got) != 1 || got["index.html"] != "i" {
		t.Fatalf("served = %v (the config file and other.txt are not part of it)", got)
	}
	v := fake.Statico("docs").Versions[0]
	if v.Extra["fallbackFile"] != "/index.html" {
		t.Fatalf("the declared answers were not used: %v", v.Extra)
	}
	if redirects, _ := v.Extra["redirects"].([]interface{}); len(redirects) != 1 {
		t.Fatalf("the declared redirects were not used: %v", v.Extra)
	}
}

func TestStaticoUploadExcludeRefinesTheDeclaredSelection(t *testing.T) {
	ts, fake := staticoServer(t)
	st := fake.AddStatico("docs", nil)
	st.ProjectID, st.WorkspaceID = testProjectID, testWorkspaceID
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"public/index.html": "i", "public/big.bin": "b"})
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"statico": {"docs": {"htmlToFolders": false, "include": {"./public": "./"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, dir)
	ts.on("GET", "/v2/workspaces", respond(200, []any{map[string]any{"id": testWorkspaceID, "name": "acme"}}))
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--exclude", "./public/*.bin", "docs")
	assertNoError(t, err)
	if got := fake.ActiveFiles("docs"); len(got) != 1 || got["index.html"] != "i" {
		t.Fatalf("served = %v", got)
	}
}

func TestStaticoUploadJSONOutput(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "a"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "--log-format", "json", "statico", "deploy", "--include", ".", "docs")
	assertNoError(t, err)
	assertContains(t, out, `\"uploaded\": 1`)
	assertContains(t, out, `\"activated\": true`)
}

func readTarGz(t *testing.T, file string) map[string]string {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("%s is not a tar.gz: %v", file, err)
	}
	out := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
	return out
}

func TestStaticoDownloadServedVersionAsTarGz(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i", "blog/a.html": "a"})
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "download", "docs")
	assertNoError(t, err)
	assertContains(t, out, "Downloaded version v1 of docs to docs-v1.tar.gz")
	got := readTarGz(t, filepath.Join(dir, "docs-v1.tar.gz"))
	if !reflect.DeepEqual(got, map[string]string{"index.html": "i", "blog/a.html": "a"}) {
		t.Fatalf("archive = %v", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("no temporary file may stay: %v", entries)
	}
}

func TestStaticoDownloadASpecificVersion(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"old.txt": "o"})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"new.txt": "n"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "new.txt", "docs")
	assertNoError(t, err)
	_, err = runArgs(t, ts, "statico", "download", "--version", "v1", "docs")
	assertNoError(t, err)
	_, err = runArgs(t, ts, "statico", "download", "docs")
	assertNoError(t, err)
	if got := readTarGz(t, filepath.Join(dir, "docs-v1.tar.gz")); !reflect.DeepEqual(got, map[string]string{"old.txt": "o"}) {
		t.Fatalf("v1 = %v", got)
	}
	if got := readTarGz(t, filepath.Join(dir, "docs-v2.tar.gz")); got["new.txt"] != "n" {
		t.Fatalf("the served version = %v", got)
	}
	out, err := runArgs(t, ts, "--log-format", "json", "statico", "download", "docs")
	assertNoError(t, err)
	assertContains(t, out, `\"file\": \"docs-v2.tar.gz\"`)
}

func TestStaticoDownloadFailureLeavesNoFile(t *testing.T) {
	ts, fake := staticoServer(t)
	st := fake.AddStatico("docs", map[string]string{"a": "a"})
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	fake.Fail["GET /v2/storages/statico-"+st.ID+"/archive"] = 500
	_, err := runArgs(t, ts, "statico", "download", "docs")
	assertError(t, err)
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a failed download leaves nothing: %v", entries)
	}
}

func TestStaticoUploadPrintsTheDomainOfTheWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	st := fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	url := "https://" + mock.PublicDomain(st)

	out, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "✓ docs: version v1 is live at "+url)

	// nothing changed: the success message still says where it is
	out, err = runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "✓ docs is already up to date at "+url)

	// a version that is not activated is not served, so there is no address to print
	writeTree(t, dir, map[string]string{"index.html": "changed"})
	out, err = runArgs(t, ts, "statico", "deploy", "--activate=false", "--html-to-folders=false", "--include", ".:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "uploaded, not activated")
	if strings.Contains(out, "https://") {
		t.Fatalf("a version that is not served has no address:\n%s", out)
	}
}

func TestStaticoUploadWithoutADomainFromTheAPIPrintsNoAddress(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.OmitDomain = true
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "✓ docs: version v1 is live (")
	if strings.Contains(out, " at ") || strings.Contains(out, "https://") {
		t.Fatalf("no domain was sent, so none is printed:\n%s", out)
	}
}

func TestStaticoUploadJSONOutputHasTheURL(t *testing.T) {
	ts, fake := staticoServer(t)
	st := fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "a"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "--log-format", "json", "statico", "deploy", "--include", ".:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, `\"url\": \"https://`+mock.PublicDomain(st)+`\"`)
}

// deployStaticoServer is the stateful cloud plus the statico endpoints.
func deployStaticoServer(t *testing.T) (*testServer, *cloud, *mock.Server) {
	t.Helper()
	ts, c := newCloudServer(t)
	fake := mock.New()
	fake.Workspace = "ws-1" // the workspace of the cloud fake
	statico := c.handle
	ts.fallback = func(w http.ResponseWriter, r *http.Request) bool {
		if fake.Handle(w, r) {
			return true
		}
		return statico(w, r)
	}
	t.Cleanup(func() {
		if fake.Unauthorized != 0 {
			t.Errorf("%d request(s) were not authorised with the CLI's API token", fake.Unauthorized)
		}
	})
	return ts, c, fake
}

func writeStaticoProject(t *testing.T, config string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, files)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, dir)
	return dir
}

// ---- the first upload creates the website ----

func firstUploadProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"dist/index.html": "i", "dist/a.js": "js"})
	withWorkingDirectory(t, dir)
	return dir
}

func TestStaticoFirstUploadCreatesTheWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	out, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", "./dist:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "✓ Created statico website docs")
	assertContains(t, out, "is live at https://")
	st := fake.Statico("docs")
	if st == nil || !st.Public || st.ProjectID != testProjectID || st.WorkspaceID != testWorkspaceID {
		t.Fatalf("website = %+v", st)
	}
	if got := fake.ActiveFiles("docs"); len(got) != 2 || got["index.html"] != "i" {
		t.Fatalf("served = %v", got)
	}
	// the second upload finds it and creates nothing
	before := len(fake.Created)
	out, err = runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", "./dist:./", "docs")
	assertNoError(t, err)
	if len(fake.Created) != before || strings.Contains(out, "Created statico website") {
		t.Fatalf("an existing website is not created again:\n%s", out)
	}
	assertContains(t, out, "already up to date")
}

func TestStaticoFirstUploadOptionsPrivateAndRegion(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--public=false", "--region", "europe", "--include", "./dist:./", "docs")
	assertNoError(t, err)
	created := fake.Created[0]
	if created["public"] != false || created["region"] != "europe" || created["name"] != "docs" {
		t.Fatalf("create body = %v", created)
	}
	if fake.Statico("docs").Public {
		t.Fatal("the website must be private")
	}
}

func TestStaticoFirstUploadUsesTheDeclaredWebsiteSettings(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := firstUploadProject(t)
	config := `{"statico": {"docs": {"public": false, "regionId": "reg-1", "include": {"./dist": "./"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "docs")
	assertNoError(t, err)
	created := fake.Created[0]
	if created["public"] != false || created["regionId"] != "reg-1" {
		t.Fatalf("a declared website brings its own settings: %v", created)
	}
	// a flag still wins over the config
	fake.Staticos = nil
	fake.Created = nil
	_, err = runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--public=true", "docs")
	assertNoError(t, err)
	if fake.Created[0]["public"] != true {
		t.Fatalf("create body = %v", fake.Created[0])
	}
}

func TestStaticoSyncUpAlsoCreatesTheWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--include", "./dist:./", "docs")
	assertNoError(t, err)
	if fake.Statico("docs") == nil || len(fake.ActiveFiles("docs")) == 0 {
		t.Fatal("sync up creates the website on its first run")
	}
}

func TestStaticoDryRunDoesNotCreateTheWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	out, err := runArgs(t, ts, "statico", "deploy", "--dry-run", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", "./dist:./", "docs")
	assertNoError(t, err)
	assertContains(t, out, "does not exist yet: the upload would create it")
	assertContains(t, out, "2 file(s) selected: 2 to upload")
	if len(fake.Staticos) != 0 || len(fake.Created) != 0 {
		t.Fatal("a dry run must not create the website")
	}
	if _, err := os.Stat(".futrou"); err == nil {
		t.Fatal("a dry run stages nothing")
	}
}

func TestStaticoUploadToAnIDThatDoesNotExistIsAnErrorNotACreate(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--include", "./dist:./", "00000000-0000-4000-8000-0000000000ff")
	assertError(t, err)
	if !strings.Contains(err.Error(), "not found") || len(fake.Staticos) != 0 {
		t.Fatalf("err = %v staticos = %d", err, len(fake.Staticos))
	}
}

func TestStaticoFirstUploadWithBadFilesCreatesNothing(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--include", "./missing:./", "docs")
	assertError(t, err)
	if len(fake.Staticos) != 0 {
		t.Fatal("a selection that matches nothing must not leave an empty website behind")
	}
}

func TestStaticoFirstUploadFailsClearlyWhenTheWebsiteCannotBeCreated(t *testing.T) {
	ts, fake := staticoServer(t)
	firstUploadProject(t)
	fake.Fail["POST /v2/staticos"] = 403
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--include", "./dist:./", "docs")
	assertError(t, err)
	if !strings.Contains(err.Error(), `creating the statico website "docs"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestStaticoDownloadAndActivateNeedAnExistingWebsite(t *testing.T) {
	ts, fake := staticoServer(t)
	withWorkingDirectory(t, t.TempDir())
	for _, args := range [][]string{
		{"statico", "download", "docs"},
		{"statico", "activate", "--version", "v1", "docs"},
		{"statico", "show", "docs"},
	} {
		if _, err := runArgs(t, ts, args...); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("%v: err = %v, only an upload creates a website", args, err)
		}
	}
	if len(fake.Staticos) != 0 {
		t.Fatal("nothing may be created")
	}
}

func TestStaticoDeployWithoutANameUsesTheNameOfTheFolder(t *testing.T) {
	ts, fake := staticoServer(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "my-site")
	writeTree(t, dir, map[string]string{"index.html": "i", "css/a.css": "c"})
	withWorkingDirectory(t, dir)
	out, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", ".")
	assertNoError(t, err)
	assertContains(t, out, "Created statico website my-site")
	if got := fake.ActiveFiles("my-site"); len(got) != 2 || got["css/a.css"] != "c" {
		t.Fatalf("the whole folder is uploaded to the root: %v", got)
	}
	// the second run finds it
	out, err = runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", ".")
	assertNoError(t, err)
	if strings.Contains(out, "Created") || len(fake.Staticos) != 1 {
		t.Fatalf("the website is found and used:\n%s", out)
	}
	assertContains(t, out, "already up to date")
}

func TestStaticoNameFromTheConfigBeatsTheFolder(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := filepath.Join(t.TempDir(), "folder-name")
	writeTree(t, dir, map[string]string{"public/index.html": "i"})
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"statico": {"declared": {"include": {"./public": "./"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false")
	assertNoError(t, err)
	if fake.Statico("declared") == nil || fake.Statico("folder-name") != nil {
		t.Fatal("the one declared website is the default name")
	}
	// two declared websites: the folder name, as there is no telling which
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"statico": {"a": {}, "b": {}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if name, err := defaultStaticoName(); err != nil || name != "folder-name" {
		t.Fatalf("name = %q err = %v", name, err)
	}
}

func TestStaticoExplicitNameBeatsEveryDefault(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := filepath.Join(t.TempDir(), "folder")
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", ".", "other")
	assertNoError(t, err)
	if fake.Statico("other") == nil || fake.Statico("folder") != nil {
		t.Fatal("the name given is used")
	}
}

func TestStaticoVersionsActivateAndDownloadUseTheFolderName(t *testing.T) {
	ts, fake := staticoServer(t)
	dir := filepath.Join(t.TempDir(), "docs")
	writeTree(t, dir, map[string]string{"index.html": "v2"})
	withWorkingDirectory(t, dir)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", ".")
	assertNoError(t, err)
	_, err = runArgs(t, ts, "statico", "download")
	assertNoError(t, err)
	if got := readTarGz(t, filepath.Join(dir, "docs-v2.tar.gz")); got["index.html"] != "v2" {
		t.Fatalf("archive = %v", got)
	}
	out, err := runArgs(t, ts, "statico", "versions")
	assertNoError(t, err)
	assertContains(t, out, "v1")
	assertContains(t, out, "v2")
	_, err = runArgs(t, ts, "statico", "activate", "--version", "v1")
	assertNoError(t, err)
	if fake.ActiveFiles("docs")["index.html"] != "v1" {
		t.Fatal("activate without a name uses the folder's website")
	}
}

func TestStaticoVersionsListsNewestFirst(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"index.html": "v1"})
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	for _, c := range []string{"v2", "v3"} {
		writeTree(t, dir, map[string]string{"index.html": c})
		_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "index.html", "docs")
		assertNoError(t, err)
	}
	out, err := runArgs(t, ts, "--log-format", "json", "statico", "versions", "docs")
	assertNoError(t, err)
	if i3, i1 := strings.Index(out, `\"name\": \"v3\"`), strings.Index(out, `\"name\": \"v1\"`); i3 < 0 || i1 < 0 || i3 > i1 {
		t.Fatalf("newest first:\n%s", out)
	}
}

func TestStaticoUpIsAnAliasOfDeploy(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"index.html": "i"})
	withWorkingDirectory(t, dir)
	_, err := runArgs(t, ts, "statico", "up", "--include", ".", "docs")
	assertNoError(t, err)
	if len(fake.ActiveFiles("docs")) == 0 {
		t.Fatal("up must deploy")
	}
	for _, cmd := range staticoCommand.Subcommands {
		if cmd.Name == "deploy" && (len(cmd.Aliases) != 1 || cmd.Aliases[0] != "up") {
			t.Errorf("deploy aliases = %v", cmd.Aliases)
		}
	}
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}

func TestStaticoWithoutAnyWorkspaceSaysSo(t *testing.T) {
	ts, _ := staticoServer(t)
	writeCLIConfig(t, ts.URL, "")
	ts.on("GET", "/v2/workspaces", respond(200, []map[string]any{}))                                                     // the account has none
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{"user": map[string]any{"id": "u1", "email": "a@b.c"}})) // a token of no workspace
	withWorkingDirectory(t, t.TempDir())
	_, err := runArgs(t, ts, "statico", "ls")
	assertError(t, err)
	assertContains(t, err.Error(), "--workspace")
}

func TestStaticoWithTokenButNoDefaultWorkspaceUsesTheOnlyWorkspace(t *testing.T) {
	ts, fake := staticoServer(t)
	writeCLIConfig(t, ts.URL, "") // logged in by token, no default workspace
	fake.AddStatico("docs", nil)
	ts.on("GET", "/v2/workspaces", respond(200, []map[string]any{{"id": mock.WorkspaceID, "name": "acme"}}))
	withWorkingDirectory(t, t.TempDir())
	out, err := runArgs(t, ts, "statico", "ls")
	assertNoError(t, err)
	assertContains(t, out, "docs")
}

// loginWithoutDefaultWorkspace stores a login (its token is the one the fake
// accepts) that has no default workspace, as an old login or one made by another
// client does, and runs a command with it: no token on the command line.
func loginWithoutDefaultWorkspace(t *testing.T, ts *testServer, args ...string) (string, error) {
	t.Helper()
	writeCLIConfig(t, ts.URL, "")
	cfg, err := cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetToken(ts.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	if err := cliconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FUTROU_API_TOKEN", "")
	return captureRun(append([]string{"futrou", "--api-url", ts.URL}, args...))
}

// A login without a default workspace never asks for one: the oldest workspace is
// used, and remembered for the next command.
func TestStaticoWithSeveralWorkspacesAndNoDefaultUsesTheOldest(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	ts.on("GET", "/v2/workspaces", respond(200, []map[string]any{
		{"id": "newer", "name": "newer", "createdAt": "2025-05-01T00:00:00Z"},
		{"id": mock.WorkspaceID, "name": "older", "createdAt": "2023-01-01T00:00:00Z"},
		{"id": "newest", "name": "newest", "createdAt": "2026-01-01T00:00:00Z"},
	}))
	withWorkingDirectory(t, t.TempDir())
	out, err := loginWithoutDefaultWorkspace(t, ts, "statico", "ls")
	assertNoError(t, err)
	assertContains(t, out, "docs")
	cfg, err := cliconfig.Load()
	if err != nil || cfg.DefaultWorkspaceFor(ts.URL) != mock.WorkspaceID {
		t.Errorf("the oldest workspace must become the default: %v, %q", err, cfg.DefaultWorkspaceFor(ts.URL))
	}
}

// An account whose workspace list is empty (a staff user, an API key) still has
// the workspace of its token.
func TestStaticoWithAnEmptyWorkspaceListUsesTheWorkspaceOfTheToken(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", nil)
	ts.on("GET", "/v2/workspaces", respond(200, []map[string]any{}))
	ts.on("GET", "/v2/auth/context", respond(200, map[string]any{
		"apiToken": map[string]any{"workspaceId": mock.WorkspaceID},
		"user":     map[string]any{"id": "u1", "email": "a@b.c"},
	}))
	withWorkingDirectory(t, t.TempDir())
	out, err := loginWithoutDefaultWorkspace(t, ts, "statico", "ls")
	assertNoError(t, err)
	assertContains(t, out, "docs")
	cfg, _ := cliconfig.Load()
	if cfg.DefaultWorkspaceFor(ts.URL) != mock.WorkspaceID {
		t.Errorf("the token's workspace must be remembered: %q", cfg.DefaultWorkspaceFor(ts.URL))
	}
}

func TestStaticoIsDescribedAsManagingStaticWebsites(t *testing.T) {
	if staticoCommand.Usage != "Manage static websites" {
		t.Errorf("usage = %q", staticoCommand.Usage)
	}
	out, err := captureRun([]string{"futrou", "--help"})
	assertNoError(t, err)
	assertContains(t, out, "Manage static websites")
}

// A website that was never deployed has no active version; the real API says so
// with a 400 "Version not found", which must not stop the first deploy.
func TestStaticoDeployFirstVersionOfANewWebsite(t *testing.T) {
	for _, status := range []int{404, 400} {
		ts, fake := staticoServer(t)
		fake.MissingVersionStatus = status
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"site/index.html": "<h1>", "site/app.js": "js"})
		withWorkingDirectory(t, dir)

		out, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", "./site:./", "fresh")
		assertNoError(t, err)
		assertContains(t, out, "is live at https://")
		if got := fake.ActiveFiles("fresh"); len(got) != 2 || got["index.html"] != "<h1>" {
			t.Errorf("status %d: active files = %v", status, got)
		}
	}
}

// The deployed API rejects the archive route (its Content-Type check does not
// exempt it): the files must then be sent one by one and the deploy must work.
func TestStaticoDeployFallsBackToSingleFilesWhenArchivesAreRefused(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.MissingVersionStatus = 400
	fake.RejectArchives = true
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"site/index.html": "<h1>", "site/app.js": "js", "site/a/b.txt": "b"})
	withWorkingDirectory(t, dir)

	out, err := runArgs(t, ts, "statico", "deploy", "--workspace", testWorkspaceID, "--project", testProjectID, "--html-to-folders=false", "--include", "./site:./", "fresh")
	assertNoError(t, err)
	assertContains(t, out, "is live at https://")
	if got := fake.ActiveFiles("fresh"); len(got) != 3 || got["a/b.txt"] != "b" {
		t.Errorf("active files = %v", got)
	}
	if fake.Archives != 0 {
		t.Errorf("archives = %d, want none", fake.Archives)
	}
}
