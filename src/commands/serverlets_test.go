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

	out, err := runArgs(t, ts, "serverlets", "create", "--name", "my-app", "--image", "nginx:latest")
	assertNoError(t, err)
	assertContains(t, out, "created")

	if received["name"] != "my-app" {
		t.Errorf("expected name=my-app, got %v", received["name"])
	}
	if received["image"] != "nginx:latest" {
		t.Errorf("expected image=nginx:latest, got %v", received["image"])
	}
}

func TestServerletsCreate_withPlanAndInstances(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureServerlet())(w, r)
	})

	_, err := runArgs(t, ts, "serverlets", "create",
		"--name", "my-app",
		"--image", "nginx:latest",
		"--plan", "plan-abc",
		"--min", "2",
		"--max", "5",
	)
	assertNoError(t, err)

	if received["serverletPlanId"] != "plan-abc" {
		t.Errorf("expected serverletPlanId=plan-abc, got %v", received["serverletPlanId"])
	}
	if received["minInstances"].(float64) != 2 {
		t.Errorf("expected minInstances=2, got %v", received["minInstances"])
	}
	if received["maxInstances"].(float64) != 5 {
		t.Errorf("expected maxInstances=5, got %v", received["maxInstances"])
	}
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

	out, err := runArgs(t, ts, "serverlets", "logs", "sl-123")
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
