package commands

import (
	"net/http"
	"strings"
	"testing"
)

func TestServerLs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers", respond(200, []interface{}{map[string]interface{}{"id": "srv-1", "name": "edge-1"}}))
	for _, args := range [][]string{{"server", "ls"}, {"servers", "list"}} {
		out, err := runArgs(t, ts, args...)
		assertNoError(t, err)
		assertContains(t, out, "srv-1")
	}
}

func TestServerCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/servers", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, map[string]interface{}{"id": "srv-1"})(w, r)
	})
	out, err := runArgs(t, ts, "server", "create", "--name", "edge-1", "--region", "reg-1")
	assertNoError(t, err)
	assertContains(t, out, "created")
	if received["name"] != "edge-1" || received["regionId"] != "reg-1" {
		t.Fatalf("unexpected body: %v", received)
	}
}

func TestServerUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/servers/srv-1", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, map[string]interface{}{"id": "srv-1"})(w, r)
	})
	_, err := runArgs(t, ts, "server", "update", "--api-port", "8443", "srv-1")
	assertNoError(t, err)
	if received["apiPort"] != "8443" {
		t.Fatalf("unexpected body: %v", received)
	}
	_, err = runArgs(t, ts, "server", "update", "srv-1")
	assertError(t, err)
}

func TestServerRm(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/servers/srv-1", respondEmpty)
	_, err := runArgs(t, ts, "server", "rm", "srv-1")
	assertNoError(t, err)
	_, err = runArgs(t, ts, "server", "rm")
	assertError(t, err)
}

func TestServerLogsAndMetrics(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers/srv-1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("expected limit=5, got %q", r.URL.RawQuery)
		}
		respond(200, []interface{}{map[string]interface{}{"message": "hello-log"}})(w, r)
	})
	ts.on("GET", "/v2/servers/srv-1/metrics", respond(200, map[string]interface{}{"cpu": "metric-value"}))

	out, err := runArgs(t, ts, "server", "logs", "--live=false", "--limit", "5", "srv-1")
	assertNoError(t, err)
	assertContains(t, out, "hello-log")

	out, err = runArgs(t, ts, "server", "metrics", "srv-1")
	assertNoError(t, err)
	assertContains(t, out, "metric-value")
}

func TestServerLogsTypeIDAndInstanceParams(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers/srv-1/logs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("type") != "serverlet" || q.Get("id") != "sl-1" || q.Get("instanceId") != "i-1" {
			t.Errorf("unexpected query: %q", r.URL.RawQuery)
		}
		respond(200, []any{map[string]any{"message": "srv-line"}})(w, r)
	})
	out, err := runArgs(t, ts, "server", "logs", "--live=false", "--type", "serverlet", "--id", "sl-1", "--instance-id", "i-1", "srv-1")
	assertNoError(t, err)
	assertContains(t, out, "srv-line")
}

func TestServerShowAndRemovedSubcommands(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers/srv-1", respond(200, map[string]any{"id": "srv-1", "name": "edge-detail"}))
	out, err := runArgs(t, ts, "server", "show", "srv-1")
	assertNoError(t, err)
	assertContains(t, out, "edge-detail")
	help, _ := runArgs(t, ts, "server", "--help")
	for _, removed := range []string{"reset-join", "leave"} {
		if strings.Contains(help, removed) {
			t.Errorf("server %s was removed but is still listed:\n%s", removed, help)
		}
	}
	for _, sub := range []string{"show", "metrics", "logs", "update", "rm"} {
		_, err := runArgs(t, ts, "server", sub)
		assertError(t, err)
	}
}

func TestServerLsRegionFilterAndErrors(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("regionId") != "reg-1" {
			t.Errorf("expected regionId=reg-1, got %q", r.URL.RawQuery)
		}
		respond(200, []any{map[string]any{"id": "srv-1"}})(w, r)
	})
	_, err := runArgs(t, ts, "server", "ls", "--region", "reg-1")
	assertNoError(t, err)

	failing := newTestServer(t)
	failing.on("GET", "/v2/servers", respond(500, fixtureAPIError("boom")))
	failing.on("POST", "/v2/servers", respond(422, fixtureAPIError("invalid")))
	failing.on("PATCH", "/v2/servers/srv-1", respond(404, fixtureAPIError("nope")))
	failing.on("DELETE", "/v2/servers/srv-1", respond(404, fixtureAPIError("nope")))
	for _, args := range [][]string{{"server", "ls"}, {"server", "create", "--name", "x"}, {"server", "update", "--name", "x", "srv-1"}, {"server", "rm", "srv-1"}} {
		_, err := runArgs(t, failing, args...)
		assertError(t, err)
	}
}

func TestServerCreateWithWorkspaceAndJSON(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/servers", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, map[string]interface{}{"id": "srv-1"})(w, r)
	})
	out, err := runArgs(t, ts, "--log-format", "json", "server", "create", "--name", "edge", "--workspace", "11111111-1111-1111-1111-111111111111")
	assertNoError(t, err)
	assertContains(t, out, "srv-1")
	if received["workspaceId"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected body: %v", received)
	}
}
