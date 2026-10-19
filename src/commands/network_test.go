package commands

import (
	"net/http"
	"testing"
)

func TestNetworkLs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/networks", respond(200, []interface{}{map[string]interface{}{"id": "net-1", "name": "backend"}}))

	for _, args := range [][]string{{"network", "ls"}, {"networks", "list"}} {
		out, err := runArgs(t, ts, args...)
		assertNoError(t, err)
		assertContains(t, out, "net-1")
	}
}

func TestNetworkCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/networks", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, map[string]interface{}{"id": "net-1"})(w, r)
	})

	_, err := runArgs(t, ts, "network", "create", "--name", "backend", "--workspace", "11111111-1111-1111-1111-111111111111", "--project", "22222222-2222-2222-2222-222222222222")
	assertNoError(t, err)
	if received["name"] != "backend" {
		t.Fatalf("unexpected body: %v", received)
	}
}

func TestNetworkRm(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/networks/net-1", respondEmpty)

	_, err := runArgs(t, ts, "network", "rm", "net-1")
	assertNoError(t, err)
}

func TestNetworkGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "network", "get")
	assertError(t, err)
}

func TestNetworkCreateSendsAllFields(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/networks", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, map[string]interface{}{"id": "net-1"})(w, r)
	})
	_, err := runArgs(t, ts, "network", "create", "--name", "backend", "--ipv4", "10.0.0.0/24", "--ipv6", "fd00::/64",
		"--internal", "--region", "reg-1",
		"--workspace", "11111111-1111-1111-1111-111111111111", "--project", "22222222-2222-2222-2222-222222222222")
	assertNoError(t, err)
	want := map[string]interface{}{
		"name": "backend", "ipv4": "10.0.0.0/24", "ipv6": "fd00::/64", "internal": "true", "regionId": "reg-1",
		"workspaceId": "11111111-1111-1111-1111-111111111111", "projectId": "22222222-2222-2222-2222-222222222222",
	}
	for k, v := range want {
		if received[k] != v {
			t.Errorf("expected %s=%v, got %v", k, v, received[k])
		}
	}
	_, err = runArgs(t, ts, "network", "create")
	assertError(t, err)
}

func TestNetworkCreate_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("POST", "/v2/networks", respond(422, fixtureAPIError("invalid cidr")))
	_, err := runArgs(t, ts, "network", "create", "--name", "x",
		"--workspace", "11111111-1111-1111-1111-111111111111", "--project", "22222222-2222-2222-2222-222222222222")
	assertError(t, err)
}

func TestNetworkUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/networks/net-1", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, map[string]interface{}{"id": "net-1"})(w, r)
	})
	out, err := runArgs(t, ts, "network", "update", "--name", "renamed", "--ipv4", "10.1.0.0/24", "--ipv6", "fd01::/64", "--internal", "net-1")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	want := map[string]interface{}{"name": "renamed", "ipv4": "10.1.0.0/24", "ipv6": "fd01::/64", "internal": "true"}
	for k, v := range want {
		if received[k] != v {
			t.Errorf("expected %s=%v, got %v", k, v, received[k])
		}
	}
	_, err = runArgs(t, ts, "network", "update", "net-1")
	assertError(t, err)
	_, err = runArgs(t, ts, "network", "update", "--name", "x")
	assertError(t, err)
}

func TestNetworkShowDevicesAndMetrics(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/networks/net-1", respond(200, map[string]any{"id": "net-1", "name": "show-me"}))
	ts.on("GET", "/v2/networks/net-1/devices", respond(200, []any{map[string]any{"name": "device-1"}}))
	ts.on("GET", "/v2/networks/net-1/metrics", respond(200, map[string]any{"rx": "metric-1"}))
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"network", "show", "net-1"}, "show-me"},
		{[]string{"network", "devices", "net-1"}, "device-1"},
		{[]string{"network", "metrics", "net-1"}, "metric-1"},
	} {
		out, err := runArgs(t, ts, c.args...)
		assertNoError(t, err)
		assertContains(t, out, c.want)
	}
	for _, sub := range []string{"show", "devices", "metrics", "rm"} {
		_, err := runArgs(t, ts, "network", sub)
		assertError(t, err)
	}
}

func TestNetworkRequests_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/networks/bad", respond(404, fixtureAPIError("not found")))
	ts.on("DELETE", "/v2/networks/bad", respond(404, fixtureAPIError("not found")))
	ts.on("GET", "/v2/networks", respond(500, fixtureAPIError("boom")))
	for _, args := range [][]string{{"network", "show", "bad"}, {"network", "rm", "bad"}, {"network", "ls"}} {
		_, err := runArgs(t, ts, args...)
		assertError(t, err)
	}
}

func TestNetworkRmAndLsJSON(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/networks/net-1", respondEmpty)
	ts.on("GET", "/v2/networks", respond(200, []any{map[string]any{"id": "net-1", "name": "backend"}}))
	out, err := runArgs(t, ts, "--log-format", "json", "network", "rm", "net-1")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	out, err = runArgs(t, ts, "--log-format", "json", "network", "ls")
	assertNoError(t, err)
	assertContains(t, out, "backend")
}
