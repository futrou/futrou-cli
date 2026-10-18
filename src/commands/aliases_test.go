package commands

import (
	"strings"
	"testing"
)

func TestSingularNamesAndHiddenPlurals(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{}))
	ts.on("GET", "/v2/proxies", respond(200, []interface{}{}))
	for _, args := range [][]string{{"serverlet", "ls"}, {"serverlets", "ls"}, {"serverlet", "list"}, {"proxy", "ls"}, {"proxies", "list"}} {
		if _, err := runArgs(t, ts, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	out, _ := runArgs(t, ts, "--help")
	if strings.Contains(out, "\n   serverlets") || strings.Contains(out, "\n   proxies") {
		t.Fatalf("plural aliases should be hidden:\n%s", out)
	}
}

func TestShowIsCanonicalWithGetAndInspectAliases(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-1", respond(200, map[string]any{"id": "sl-1", "name": "web-detail"}))
	ts.on("GET", "/v2/networks/net-1", respond(200, map[string]any{"id": "net-1", "name": "net-detail"}))
	ts.on("GET", "/v2/servers/srv-1", respond(200, map[string]any{"id": "srv-1", "name": "srv-detail"}))
	ts.on("GET", "/v2/dns/dz-1/records/r-1", respond(200, map[string]any{"id": "r-1", "name": "rec-detail"}))

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"serverlet", "show", "sl-1"}, "web-detail"},
		{[]string{"serverlet", "inspect", "sl-1"}, "web-detail"},
		{[]string{"serverlet", "get", "sl-1"}, "web-detail"},
		{[]string{"serverlets", "inspect", "sl-1"}, "web-detail"},
		{[]string{"network", "show", "net-1"}, "net-detail"},
		{[]string{"network", "inspect", "net-1"}, "net-detail"},
		{[]string{"server", "show", "srv-1"}, "srv-detail"},
		{[]string{"server", "inspect", "srv-1"}, "srv-detail"},
		{[]string{"dns", "record", "show", "dz-1", "r-1"}, "rec-detail"},
		{[]string{"dns", "record", "inspect", "dz-1", "r-1"}, "rec-detail"},
	} {
		out, err := runArgs(t, ts, c.args...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		assertContains(t, out, c.want)
	}

	out, _ := runArgs(t, ts, "serverlet", "--help")
	assertContains(t, out, "show")
	if strings.Contains(out, "\n   get") || strings.Contains(out, "\n   inspect") {
		t.Fatalf("get and inspect should be hidden aliases:\n%s", out)
	}
}
