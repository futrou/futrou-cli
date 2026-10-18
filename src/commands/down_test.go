package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const downWorkspaceID = "11111111-1111-1111-1111-111111111111"

func writeDownConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace":"`+downWorkspaceID+`","project":"demo"}`), 0644); err != nil {
		t.Fatal(err)
	}
}

// downServer mocks a project holding one resource of every kind and records
// the DELETE requests in the order they arrive.
func downServer(t *testing.T) (*testServer, *[]string) {
	ts := newTestServer(t)
	var deleted []string
	ts.on("GET", "/v2/projects", respond(http.StatusOK, []any{map[string]any{"id": "proj-1", "name": "demo", "workspaceId": downWorkspaceID}}))
	collections := map[string]string{
		"/v2/crons": "cron", "/v2/proxies": "proxy", "/v2/serverlets": "serverlet",
		"/v2/storages": "storage", "/v2/networks": "network", "/v2/dns": "dns",
	}
	for path, name := range collections {
		ts.on("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("projectId") != "proj-1" {
				t.Errorf("%s must be scoped to the project, got %q", r.URL.Path, r.URL.RawQuery)
			}
			respond(http.StatusOK, []any{map[string]any{"id": name + "-1", "name": name, "domain": name + ".example.com"}})(w, r)
		})
		ts.on("DELETE", path+"/"+name+"-1", func(w http.ResponseWriter, r *http.Request) {
			deleted = append(deleted, name)
			respondEmpty(w, r)
		})
	}
	ts.on("GET", "/v2/dns/dns-1/records", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/workspaces/"+downWorkspaceID, respond(http.StatusOK, map[string]any{"id": downWorkspaceID, "name": "acme"}))
	ts.on("DELETE", "/v2/projects/proj-1", func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, "project")
		respondEmpty(w, r)
	})
	return ts, &deleted
}

func TestDownDeletesEverythingThenTheProject(t *testing.T) {
	writeDownConfig(t)
	ts, deleted := downServer(t)

	out, err := runArgs(t, ts, "down", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Delete (7)")
	assertContains(t, out, "Deleted 6 resource(s) and project")
	want := "cron,proxy,serverlet,storage,network,dns,project"
	if got := strings.Join(*deleted, ","); got != want {
		t.Fatalf("delete order = %s, want %s", got, want)
	}
}

func TestDownAliasesAndCI(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		ci   string
	}{
		"teardown": {[]string{"teardown", "-y"}, ""},
		"CI":       {[]string{"down"}, "true"},
	} {
		t.Run(name, func(t *testing.T) {
			writeDownConfig(t)
			t.Setenv("CI", tc.ci)
			withStdin(t, "")
			ts, deleted := downServer(t)

			out, err := runArgs(t, ts, tc.args...)
			assertNoError(t, err)
			if strings.Contains(out, "[y/N]") {
				t.Fatalf("must not prompt: %s", out)
			}
			if len(*deleted) != 7 {
				t.Fatalf("expected 7 deletes, got %v", *deleted)
			}
		})
	}
}

func TestDownConfirmation(t *testing.T) {
	for name, tc := range map[string]struct {
		stdin   string
		deleted int
	}{"y": {"y\n", 7}, "yes": {"yes\n", 7}, "enter": {"\n", 0}, "n": {"n\n", 0}} {
		t.Run(name, func(t *testing.T) {
			writeDownConfig(t)
			t.Setenv("CI", "")
			withStdin(t, tc.stdin)
			ts, deleted := downServer(t)

			out, err := runArgs(t, ts, "down")
			assertNoError(t, err)
			assertContains(t, out, "including the project itself? [y/N]")
			if len(*deleted) != tc.deleted {
				t.Fatalf("expected %d deletes, got %v", tc.deleted, *deleted)
			}
		})
	}
}

func TestDownDryRunDeletesNothing(t *testing.T) {
	writeDownConfig(t)
	ts, deleted := downServer(t)

	out, err := runArgs(t, ts, "down", "--dry-run", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Delete (7)")
	if len(*deleted) != 0 {
		t.Fatalf("dry run must not delete: %v", *deleted)
	}
}

func TestDownRequiresProject(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t) // any request fails the test
	_, err := runArgs(t, ts, "down", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "project")
}

func TestDownStopsBeforeDeletingProjectWhenAResourceFails(t *testing.T) {
	writeDownConfig(t)
	ts, deleted := downServer(t)
	ts.on("DELETE", "/v2/storages/storage-1", respond(409, fixtureAPIError("storage in use")))

	_, err := runArgs(t, ts, "down", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "storage in use")
	for _, name := range *deleted {
		if name == "project" || name == "network" || name == "dns" {
			t.Fatalf("%s must not be deleted after a failure: %v", name, *deleted)
		}
	}
}

func TestDownUnknownProjectFails(t *testing.T) {
	writeDownConfig(t)
	ts := newTestServer(t)
	ts.on("GET", "/v2/projects", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/workspaces/"+downWorkspaceID, respond(http.StatusOK, map[string]any{"id": downWorkspaceID, "name": "acme"}))
	_, err := runArgs(t, ts, "down", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "not found")
}

func TestDownJSONPrintsPlan(t *testing.T) {
	writeDownConfig(t)
	ts, _ := downServer(t)
	out, err := runArgs(t, ts, "--log-format", "json", "down", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "project")
}

func TestDownProjectSelectsTheMatchingConfigFile(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	writeFiles(t, dir, map[string]string{
		"futrou.json":      `{"workspace":"` + downWorkspaceID + `","project":"other"}`,
		"futrou.prod.json": `{"workspace":"` + downWorkspaceID + `","project":"demo"}`,
	})
	ts, deleted := downServer(t)
	out, err := runArgs(t, ts, "down", "--project", "prod", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Using config: futrou.prod.json")
	if len(*deleted) != 7 {
		t.Fatalf("the project of futrou.prod.json must be torn down: %v", *deleted)
	}
}
