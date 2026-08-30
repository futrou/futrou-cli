package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveWorkspaceAndProject_PrefersFutrouJSON guards the priority order
// added so every command scoped by workspace/project (create, storages list,
// etc.) picks up futrou.json's declared workspace/project automatically,
// without needing --workspace/--project on every invocation. Uses a
// non-mutating command (list) so it isn't entangled with the unrelated
// post-mutation futrou.json auto-sync feature.
func TestResolveWorkspaceAndProject_PrefersFutrouJSON(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace":"`+testWorkspaceID+`","project":"`+testProjectID+`"}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	var gotQuery string
	ts.on("GET", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		respond(200, []interface{}{})(w, r)
	})

	// No --workspace or --project flags: must come from futrou.json.
	_, err := runArgs(t, ts, "storages", "list")
	assertNoError(t, err)

	assertContains(t, gotQuery, "workspaceId="+testWorkspaceID)
	assertContains(t, gotQuery, "projectId="+testProjectID)
}

// TestResolveWorkspaceAndProject_FlagOverridesFutrouJSON ensures an explicit
// --workspace/--project flag still wins even when futrou.json declares
// different values, so scripts can opt out of the project-directory default.
func TestResolveWorkspaceAndProject_FlagOverridesFutrouJSON(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"workspace":"33333333-3333-3333-3333-333333333333","project":"44444444-4444-4444-4444-444444444444"}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	var gotQuery string
	ts.on("GET", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		respond(200, []interface{}{})(w, r)
	})

	_, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", testProjectID)
	assertNoError(t, err)

	assertContains(t, gotQuery, "workspaceId="+testWorkspaceID)
	assertContains(t, gotQuery, "projectId="+testProjectID)
}

// TestResolveWorkspaceID_ErrorsWithoutAnySource confirms the error path
// still fires with a helpful message when there's no flag, no futrou.json,
// and no login default to fall back to.
func TestResolveWorkspaceID_ErrorsWithoutAnySource(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	ts := newTestServer(t)
	_, err := runArgs(t, ts, "storages", "create", "--name", "x", "--plan", "y")
	assertError(t, err)
	if err != nil {
		assertContains(t, err.Error(), "no workspace specified")
	}
}
