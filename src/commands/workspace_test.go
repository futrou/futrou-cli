package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

const wsID = "11111111-1111-1111-1111-111111111111"

func TestWorkspaceCommandsCallTheAPI(t *testing.T) {
	for name, tc := range map[string]struct {
		args     []string
		method   string
		path     string
		response any
		wantBody map[string]any // nil: the request must have no body
		wantOut  string
	}{
		"ls":                    {[]string{"workspace", "ls"}, "GET", "/v2/workspaces", []any{fixtureWorkspace()}, nil, "my-workspace"},
		"ls json":               {[]string{"--log-format", "json", "workspace", "ls"}, "GET", "/v2/workspaces", []any{fixtureWorkspace()}, nil, "ws-abc"},
		"show":                  {[]string{"workspace", "show", "ws-abc"}, "GET", "/v2/workspaces/ws-abc", fixtureWorkspace(), nil, "My Workspace"},
		"update":                {[]string{"workspace", "update", "--name", "new", "--display-name", "New", "ws-abc"}, "PATCH", "/v2/workspaces/ws-abc", fixtureWorkspace(), map[string]any{"name": "new", "displayName": "New"}, "Workspace updated"},
		"update json":           {[]string{"--log-format", "json", "workspace", "update", "--name", "n", "ws-abc"}, "PATCH", "/v2/workspaces/ws-abc", fixtureWorkspace(), map[string]any{"name": "n"}, "ws-abc"},
		"rm":                    {[]string{"workspace", "rm", "ws-abc"}, "DELETE", "/v2/workspaces/ws-abc", nil, nil, "Workspace deleted"},
		"rm json":               {[]string{"--log-format", "json", "workspace", "rm", "ws-abc"}, "DELETE", "/v2/workspaces/ws-abc", nil, nil, "deleted"},
		"limit":                 {[]string{"workspace", "limit", "--workspace", wsID}, "GET", "/v2/workspaces/" + wsID + "/limits", map[string]any{"serverlets": 10}, nil, "serverlets"},
		"contact show":          {[]string{"workspace", "contact", "show", "--workspace", wsID}, "GET", "/v2/workspaces/" + wsID + "/contact", map[string]any{"email": "a@b.c"}, nil, "a@b.c"},
		"contact update":        {[]string{"workspace", "contact", "update", "--workspace", wsID, "--firstname", "Ada", "--lastname", "L", "--company", "ACME", "--email", "a@b.c", "--phone", "1", "--street-address", "Main 1", "--city", "Town", "--postal-code", "123", "--country", "DE"}, "PATCH", "/v2/workspaces/" + wsID + "/contact", map[string]any{}, map[string]any{"firstname": "Ada", "lastname": "L", "company": "ACME", "email": "a@b.c", "phone": "1", "streetAddress": "Main 1", "city": "Town", "postalCode": "123", "country": "DE"}, "Contact updated"},
		"contact update json":   {[]string{"--log-format", "json", "workspace", "contact", "update", "--workspace", wsID, "--city", "Town"}, "PATCH", "/v2/workspaces/" + wsID + "/contact", map[string]any{"city": "Town"}, map[string]any{"city": "Town"}, "Town"},
		"user ls":               {[]string{"workspace", "user", "ls", "--workspace", wsID}, "GET", "/v2/workspaces/" + wsID + "/users", []any{map[string]any{"id": "wu-1", "userId": "u-1", "role": "owner"}}, nil, "wu-1"},
		"user ls json":          {[]string{"--log-format", "json", "workspace", "user", "ls", "--workspace", wsID}, "GET", "/v2/workspaces/" + wsID + "/users", []any{map[string]any{"id": "wu-1"}}, nil, "wu-1"},
		"user show":             {[]string{"workspace", "user", "show", "--workspace", wsID, "wu-1"}, "GET", "/v2/workspaces/" + wsID + "/users/wu-1", map[string]any{"id": "wu-1", "role": "owner"}, nil, "owner"},
		"user add":              {[]string{"workspace", "user", "add", "--workspace", wsID, "--user-id", "u-2", "--role", "developer"}, "POST", "/v2/workspaces/" + wsID + "/users", map[string]any{"id": "wu-2"}, map[string]any{"userId": "u-2", "role": "developer"}, "User added"},
		"user add default role": {[]string{"workspace", "user", "add", "--workspace", wsID, "--user-id", "u-2"}, "POST", "/v2/workspaces/" + wsID + "/users", map[string]any{"id": "wu-2"}, map[string]any{"userId": "u-2", "role": "viewer"}, "wu-2"},
		"user add json":         {[]string{"--log-format", "json", "workspace", "user", "add", "--workspace", wsID, "--user-id", "u-2"}, "POST", "/v2/workspaces/" + wsID + "/users", map[string]any{"id": "wu-2"}, map[string]any{"userId": "u-2", "role": "viewer"}, "wu-2"},
		"user update":           {[]string{"workspace", "user", "update", "--workspace", wsID, "--role", "administrator", "wu-1"}, "PATCH", "/v2/workspaces/" + wsID + "/users/wu-1", map[string]any{}, map[string]any{"role": "administrator"}, "role updated"},
		"user update json":      {[]string{"--log-format", "json", "workspace", "user", "update", "--workspace", wsID, "--role", "owner", "wu-1"}, "PATCH", "/v2/workspaces/" + wsID + "/users/wu-1", map[string]any{"id": "wu-1"}, map[string]any{"role": "owner"}, "wu-1"},
		"user rm":               {[]string{"workspace", "user", "rm", "--workspace", wsID, "wu-1"}, "DELETE", "/v2/workspaces/" + wsID + "/users/wu-1", nil, nil, "removed from workspace"},
		"user rm json":          {[]string{"--log-format", "json", "workspace", "user", "rm", "--workspace", wsID, "wu-1"}, "DELETE", "/v2/workspaces/" + wsID + "/users/wu-1", nil, nil, "removed"},
		"legacy plural":         {[]string{"workspaces", "list"}, "GET", "/v2/workspaces", []any{fixtureWorkspace()}, nil, "ws-abc"},
	} {
		t.Run(name, func(t *testing.T) {
			ts := newTestServer(t)
			var body map[string]any
			called := false
			ts.on(tc.method, tc.path, func(w http.ResponseWriter, r *http.Request) {
				called = true
				decodeBody(r, &body)
				if tc.response == nil {
					respondEmpty(w, r)
					return
				}
				respond(200, tc.response)(w, r)
			})
			out, err := runArgs(t, ts, tc.args...)
			assertNoError(t, err)
			if !called {
				t.Fatalf("%s %s was never requested", tc.method, tc.path)
			}
			assertContains(t, out, tc.wantOut)
			for key, want := range tc.wantBody {
				if body[key] != want {
					t.Errorf("body[%s] = %v, want %v (body %v)", key, body[key], want, body)
				}
			}
			if tc.wantBody != nil && len(body) != len(tc.wantBody) {
				t.Errorf("body = %v, want exactly %v", body, tc.wantBody)
			}
		})
	}
}

func TestWorkspaceCommandsValidateTheirInput(t *testing.T) {
	ts := newTestServer(t) // any request would fail the test
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"show without id":          {[]string{"workspace", "show"}, "workspace ID required"},
		"update without id":        {[]string{"workspace", "update", "--name", "x"}, "workspace ID required"},
		"update without fields":    {[]string{"workspace", "update", "ws-abc"}, "no fields to update"},
		"rm without id":            {[]string{"workspace", "rm"}, "workspace ID required"},
		"contact update no fields": {[]string{"workspace", "contact", "update", "--workspace", wsID}, "no fields to update"},
		"user show without id":     {[]string{"workspace", "user", "show", "--workspace", wsID}, "workspace user ID required"},
		"user update without id":   {[]string{"workspace", "user", "update", "--workspace", wsID, "--role", "owner"}, "workspace user ID required"},
		"user rm without id":       {[]string{"workspace", "user", "rm", "--workspace", wsID}, "workspace user ID required"},
		"user add without user id": {[]string{"workspace", "user", "add", "--workspace", wsID}, "user-id"},
		"user update without role": {[]string{"workspace", "user", "update", "--workspace", wsID, "wu-1"}, "role"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runArgs(t, ts, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestWorkspaceCommandsReportAPIFailures(t *testing.T) {
	for name, args := range map[string][]string{
		"ls":             {"workspace", "ls"},
		"show":           {"workspace", "show", "ws-abc"},
		"update":         {"workspace", "update", "--name", "x", "ws-abc"},
		"rm":             {"workspace", "rm", "ws-abc"},
		"limit":          {"workspace", "limit", "--workspace", wsID},
		"contact show":   {"workspace", "contact", "show", "--workspace", wsID},
		"contact update": {"workspace", "contact", "update", "--workspace", wsID, "--city", "x"},
		"user ls":        {"workspace", "user", "ls", "--workspace", wsID},
		"user show":      {"workspace", "user", "show", "--workspace", wsID, "wu-1"},
		"user add":       {"workspace", "user", "add", "--workspace", wsID, "--user-id", "u"},
		"user update":    {"workspace", "user", "update", "--workspace", wsID, "--role", "owner", "wu-1"},
		"user rm":        {"workspace", "user", "rm", "--workspace", wsID, "wu-1"},
	} {
		t.Run(name, func(t *testing.T) {
			ts, cloud := newCloudServer(t)
			// every request to the API fails, whatever the method
			for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
				for _, path := range []string{"/v2/workspaces", "/v2/workspaces/ws-abc", "/v2/workspaces/" + wsID + "/limits", "/v2/workspaces/" + wsID + "/contact", "/v2/workspaces/" + wsID + "/users", "/v2/workspaces/" + wsID + "/users/wu-1"} {
					ts.on(method, path, respond(500, map[string]string{"message": "boom"}))
				}
			}
			_ = cloud
			if _, err := runArgs(t, ts, args...); err == nil {
				t.Fatal("an API failure must be reported")
			}
		})
	}
}

func TestWorkspaceCommandsResolveAWorkspaceName(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", func(w http.ResponseWriter, r *http.Request) {
		list := []any{}
		if r.URL.Query().Get("name") == "acme" { // the API filters by name
			list = append(list, map[string]any{"id": wsID, "name": "acme"})
		}
		respond(200, list)(w, r)
	})
	ts.on("GET", "/v2/workspaces/"+wsID+"/limits", respond(200, map[string]any{"ok": true}))
	out, err := runArgs(t, ts, "workspace", "limit", "--workspace", "acme")
	assertNoError(t, err)
	assertContains(t, out, "ok")
	_, err = runArgs(t, ts, "workspace", "limit", "--workspace", "ghost")
	assertError(t, err)
}
