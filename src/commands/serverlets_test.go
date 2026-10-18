package commands

import (
	"net/http"
	"testing"
)

func TestServerletsList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	out, err := runArgs(t, ts, "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
	assertContains(t, out, "my-app")
}

func TestServerletsList_empty(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{}))

	out, err := runArgs(t, ts, "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "No results.")
}

func TestServerletsList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "serverlets", "list")
	assertError(t, err)
}

func TestServerletsGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-123", respond(200, fixtureServerlet()))

	out, err := runArgs(t, ts, "serverlets", "get", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
	assertContains(t, out, "nginx:latest")
}

func TestServerletsGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "get")
	assertError(t, err)
}

func TestServerletsGet_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "serverlets", "get", "bad-id")
	assertError(t, err)
}

func TestServerletsCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureServerlet())(w, r)
	})

	out, err := runArgs(t, ts, "serverlets", "create", "--name", "my-app", "--image", "nginx:latest", "--plan", "plan-abc",
		"--workspace", "11111111-1111-1111-1111-111111111111", "--project", "22222222-2222-2222-2222-222222222222")
	assertNoError(t, err)
	assertContains(t, out, "created")

	if received["name"] != "my-app" {
		t.Errorf("expected name=my-app, got %v", received["name"])
	}
	if received["image"] != "nginx:latest" {
		t.Errorf("expected image=nginx:latest, got %v", received["image"])
	}
	if received["serverletPlanId"] != "plan-abc" {
		t.Errorf("expected serverletPlanId=plan-abc, got %v", received["serverletPlanId"])
	}
	if received["workspaceId"] != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("expected workspaceId to be resolved, got %v", received["workspaceId"])
	}
	if received["projectId"] != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("expected projectId to be resolved, got %v", received["projectId"])
	}
}

func TestServerletsCreate_requiresPlan(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "create", "--name", "my-app", "--image", "nginx:latest")
	assertError(t, err)
}

func TestServerletsCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "serverlets", "create", "--name", "x", "--image", "y")
	assertError(t, err)
}

func TestServerletsUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/serverlets/sl-123", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureServerlet())(w, r)
	})

	out, err := runArgs(t, ts, "serverlets", "update", "--name", "new-name", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "updated")

	if received["name"] != "new-name" {
		t.Errorf("expected name=new-name, got %v", received["name"])
	}
}

func TestServerletsUpdate_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "update")
	assertError(t, err)
}

func TestServerletsUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "update", "sl-123")
	assertError(t, err)
}

func TestServerletsUpdate_image(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/serverlets/sl-123", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureServerlet())(w, r)
	})

	_, err := runArgs(t, ts, "serverlets", "update", "--image", "alpine:3", "sl-123")
	assertNoError(t, err)
	if received["image"] != "alpine:3" {
		t.Errorf("expected image=alpine:3, got %v", received["image"])
	}
}

func TestServerletsDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/serverlets/sl-123", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "serverlets", "delete", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestServerletsDelete_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "delete")
	assertError(t, err)
}

func TestServerletsDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/serverlets/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "serverlets", "delete", "bad-id")
	assertError(t, err)
}

func TestServerletsStart(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("POST", "/v2/serverlets/sl-123/start", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "serverlets", "start", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "start")
	if !called {
		t.Error("POST /start was not called")
	}
}

func TestServerletsStop(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/serverlets/sl-123/stop", respondEmpty)

	out, err := runArgs(t, ts, "serverlets", "stop", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "stop")
}

func TestServerletsRestart(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/serverlets/sl-123/restart", respondEmpty)

	out, err := runArgs(t, ts, "serverlets", "restart", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "restart")
}

func TestServerletsLogs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-123/logs", respond(200, []interface{}{
		map[string]interface{}{"ts": "2026-01-01T00:00:00Z", "msg": "started"},
	}))

	out, err := runArgs(t, ts, "serverlets", "logs", "--live=false", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "started")
}

func TestServerletsInstances(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-123/instances", respond(200, []interface{}{
		map[string]interface{}{"id": "inst-1", "state": "running", "cpu": 0.1, "ram": 128},
	}))

	out, err := runArgs(t, ts, "serverlets", "instances", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "inst-1")
}

func TestServerletsStart_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "serverlets", "start")
	assertError(t, err)
}

func TestServerletsLogsAndMetrics(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("expected limit=5, got %q", r.URL.RawQuery)
		}
		respond(200, []interface{}{map[string]interface{}{"message": "hello-log"}})(w, r)
	})
	ts.on("GET", "/v2/serverlets/sl-1/metrics", respond(200, map[string]interface{}{"cpu": "metric-value"}))
	ts.on("GET", "/v2/serverlets/sl-1/instances/i-1/logs", respond(200, []interface{}{map[string]interface{}{"message": "inst-log"}}))
	ts.on("GET", "/v2/serverlets/sl-1/instances/i-1/metrics", respond(200, map[string]interface{}{"cpu": "inst-metric"}))

	out, err := runArgs(t, ts, "serverlet", "logs", "--live=false", "--limit", "5", "sl-1")
	assertNoError(t, err)
	assertContains(t, out, "hello-log")
	out, err = runArgs(t, ts, "serverlet", "metrics", "sl-1")
	assertNoError(t, err)
	assertContains(t, out, "metric-value")
	out, err = runArgs(t, ts, "serverlet", "instance", "logs", "--live=false", "sl-1", "i-1")
	assertNoError(t, err)
	assertContains(t, out, "inst-log")
	out, err = runArgs(t, ts, "serverlet", "instance", "metrics", "sl-1", "i-1")
	assertNoError(t, err)
	assertContains(t, out, "inst-metric")
}

func TestServerletsLifecycle(t *testing.T) {
	ts := newTestServer(t)
	for _, action := range []string{"start", "stop", "restart", "wake"} {
		ts.on("POST", "/v2/serverlets/sl-1/"+action, respondEmpty)
		out, err := runArgs(t, ts, "serverlet", action, "sl-1")
		assertNoError(t, err)
		assertContains(t, out, action)
	}
	for _, action := range []string{"start", "stop", "restart"} {
		ts.on("POST", "/v2/serverlets/sl-1/instances/i-1/"+action, respondEmpty)
		out, err := runArgs(t, ts, "serverlet", "instance", action, "sl-1", "i-1")
		assertNoError(t, err)
		assertContains(t, out, action)
	}
}

func TestServerletInstanceCommandsRequireIDs(t *testing.T) {
	ts := newTestServer(t)
	for _, args := range [][]string{
		{"serverlet", "instance", "ls"}, {"serverlet", "instance"}, {"serverlet", "instance", "logs", "sl-1"},
		{"serverlet", "instance", "metrics", "sl-1"}, {"serverlet", "instance", "start", "sl-1"},
		{"serverlet", "logs"}, {"serverlet", "metrics"}, {"serverlet", "wake"},
	} {
		_, err := runArgs(t, ts, args...)
		assertError(t, err)
	}
}

func TestServerletInstanceLsAndErrors(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-1/instances", respond(200, []any{map[string]any{"id": "inst-9", "status": "running"}}))
	for _, args := range [][]string{{"serverlet", "instance", "ls", "sl-1"}, {"serverlet", "instance", "sl-1"}} {
		out, err := runArgs(t, ts, args...)
		assertNoError(t, err)
		assertContains(t, out, "inst-9")
	}

	ts.on("POST", "/v2/serverlets/sl-1/instances/i-1/stop", respond(404, fixtureAPIError("gone")))
	ts.on("GET", "/v2/serverlets/sl-1/metrics", respond(500, fixtureAPIError("boom")))
	ts.on("POST", "/v2/serverlets/sl-1/start", respond(409, fixtureAPIError("busy")))
	for _, args := range [][]string{
		{"serverlet", "instance", "stop", "sl-1", "i-1"}, {"serverlet", "metrics", "sl-1"}, {"serverlet", "start", "sl-1"},
	} {
		_, err := runArgs(t, ts, args...)
		assertError(t, err)
	}
}

func TestServerletLifecycleJSON(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/serverlets/sl-1/restart", respondEmpty)
	ts.on("POST", "/v2/serverlets/sl-1/instances/i-1/restart", respondEmpty)
	out, err := runArgs(t, ts, "--log-format", "json", "serverlet", "restart", "sl-1")
	assertNoError(t, err)
	assertContains(t, out, "restart")
	out, err = runArgs(t, ts, "--log-format", "json", "serverlet", "instance", "restart", "sl-1", "i-1")
	assertNoError(t, err)
	assertContains(t, out, "restart")
}
