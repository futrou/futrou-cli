package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLooksLikeUUID(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"11111111-1111-1111-1111-111111111111", true},
		{"019ec1d7-e8d3-718d-9bb2-5aef81dc7911", true},
		{"default", false},
		{"my-workspace", false},
		{"", false},
		{"11111111111111111111111111111111", false},
	}
	for _, tt := range tests {
		if got := looksLikeUUID(tt.value); got != tt.want {
			t.Errorf("looksLikeUUID(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestResolveWorkspaceID_UUIDShortCircuitsLookup(t *testing.T) {
	ts := newTestServer(t)
	// No route mocked for /v2/workspaces: a real lookup would fail the test.
	ts.on("GET", "/v2/storages", respond(200, []interface{}{}))
	_, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", testProjectID)
	assertNoError(t, err)
}

func TestResolveWorkspaceID_NameLookupSuccess(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "acme" {
			t.Errorf("expected name=acme, got %s", r.URL.RawQuery)
		}
		respond(200, []interface{}{map[string]interface{}{"id": testWorkspaceID, "name": "acme"}})(w, r)
	})
	var gotQuery string
	ts.on("GET", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		respond(200, []interface{}{})(w, r)
	})

	_, err := runArgs(t, ts, "storages", "list", "--workspace", "acme", "--project", testProjectID)
	assertNoError(t, err)
	assertContains(t, gotQuery, "workspaceId="+testWorkspaceID)
}

func TestResolveWorkspaceID_NameLookupNotFound(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", respond(200, []interface{}{}))

	_, err := runArgs(t, ts, "storages", "list", "--workspace", "nonexistent", "--project", testProjectID)
	assertError(t, err)
	if err != nil {
		assertContains(t, err.Error(), `no workspace named "nonexistent"`)
	}
}

func TestResolveWorkspaceID_NameLookupAPIError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", respond(500, map[string]interface{}{"message": "boom"}))

	_, err := runArgs(t, ts, "storages", "list", "--workspace", "acme", "--project", testProjectID)
	assertError(t, err)
}

func TestResolveProjectID_NameLookupSuccess(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces/"+testWorkspaceID+"/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "my-app" {
			t.Errorf("expected name=my-app, got %s", r.URL.RawQuery)
		}
		respond(200, []interface{}{map[string]interface{}{"id": testProjectID, "name": "my-app"}})(w, r)
	})
	var gotQuery string
	ts.on("GET", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		respond(200, []interface{}{})(w, r)
	})

	_, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", "my-app")
	assertNoError(t, err)
	assertContains(t, gotQuery, "projectId="+testProjectID)
}

func TestResolveProjectID_NameLookupNotFound(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces/"+testWorkspaceID+"/projects", respond(200, []interface{}{}))

	_, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", "nonexistent")
	assertError(t, err)
	if err != nil {
		assertContains(t, err.Error(), `no project named "nonexistent"`)
	}
}

func TestResolveProjectID_DefaultsToNamedDefaultProject(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces/"+testWorkspaceID+"/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "default" {
			t.Errorf("expected name=default, got %s", r.URL.RawQuery)
		}
		respond(200, []interface{}{map[string]interface{}{"id": testProjectID, "name": "default"}})(w, r)
	})
	ts.on("GET", "/v2/storages", respond(200, []interface{}{}))

	// No --project flag and no futrou.json: must fall back to "default".
	_, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID)
	assertNoError(t, err)
}

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
