package commands

import (
	"net/http"
	"testing"
)

func TestProjectsList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/projects", respond(200, []interface{}{fixtureProject()}))

	out, err := runArgs(t, ts, "projects", "list")
	assertNoError(t, err)
	assertContains(t, out, "proj-abc")
	assertContains(t, out, "my-project")
}

func TestProjectsList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "projects", "list")
	assertError(t, err)
}

func TestProjectsGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/projects/proj-abc", respond(200, fixtureProject()))

	out, err := runArgs(t, ts, "projects", "get", "proj-abc")
	assertNoError(t, err)
	assertContains(t, out, "proj-abc")
	assertContains(t, out, "My Project")
}

func TestProjectsGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "projects", "get")
	assertError(t, err)
}

func TestProjectsGet_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/projects/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "projects", "get", "bad-id")
	assertError(t, err)
}

func TestProjectsCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/projects", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProject())(w, r)
	})

	out, err := runArgs(t, ts, "projects", "create", "--name", "my-project", "--workspace", "123e4567-e89b-12d3-a456-426614174000")
	assertNoError(t, err)
	assertContains(t, out, "created")
	if received["name"] != "my-project" {
		t.Errorf("expected name=my-project, got %v", received["name"])
	}
}

func TestProjectsCreate_withDisplayNameAndWorkspace(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/projects", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProject())(w, r)
	})

	_, err := runArgs(t, ts, "projects", "create",
		"--name", "my-project",
		"--display-name", "My Project",
		"--workspace", "123e4567-e89b-12d3-a456-426614174000",
	)
	assertNoError(t, err)
	if received["displayName"] != "My Project" {
		t.Errorf("expected displayName=My Project, got %v", received["displayName"])
	}
	if received["workspaceId"] != "123e4567-e89b-12d3-a456-426614174000" {
		t.Errorf("expected workspaceId=123e4567-e89b-12d3-a456-426614174000, got %v", received["workspaceId"])
	}
}

func TestProjectsCreate_workspaceByName(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/workspaces", respond(200, []interface{}{fixtureWorkspace()}))
	var received map[string]interface{}
	ts.on("POST", "/v2/projects", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProject())(w, r)
	})

	_, err := runArgs(t, ts, "projects", "create", "--name", "my-project", "--workspace", "my-workspace")
	assertNoError(t, err)
	if received["workspaceId"] != "ws-abc" {
		t.Errorf("expected workspaceId=ws-abc, got %v", received["workspaceId"])
	}
}

func TestProjectsCreate_noWorkspaceSpecified(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "projects", "create", "--name", "my-project")
	assertError(t, err)
}

func TestProjectsCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "projects", "create", "--name", "x")
	assertError(t, err)
}

func TestProjectsUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/projects/proj-abc", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureProject())(w, r)
	})

	out, err := runArgs(t, ts, "projects", "update", "--name", "new-name", "proj-abc")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["name"] != "new-name" {
		t.Errorf("expected name=new-name, got %v", received["name"])
	}
}

func TestProjectsUpdate_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "projects", "update")
	assertError(t, err)
}

func TestProjectsUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "projects", "update", "proj-abc")
	assertError(t, err)
}

func TestProjectsUpdate_displayName(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/projects/proj-abc", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureProject())(w, r)
	})

	_, err := runArgs(t, ts, "projects", "update", "--display-name", "Renamed", "proj-abc")
	assertNoError(t, err)
	if received["displayName"] != "Renamed" {
		t.Errorf("expected displayName=Renamed, got %v", received["displayName"])
	}
}

func TestProjectsDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/projects/proj-abc", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "projects", "delete", "proj-abc")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestProjectsDelete_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "projects", "delete")
	assertError(t, err)
}

func TestProjectsDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/projects/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "projects", "delete", "bad-id")
	assertError(t, err)
}
