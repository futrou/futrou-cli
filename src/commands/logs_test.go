package commands

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogsByName(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{
		map[string]interface{}{"id": "sl-1", "name": "web"},
		map[string]interface{}{"id": "sl-2", "name": "worker"},
	}))
	ts.on("GET", "/v2/serverlets/sl-1/logs", respond(200, []interface{}{
		map[string]interface{}{"timestamp": "2026-01-01T00:00:00Z", "message": "boot-ok"},
	}))

	out, err := runArgs(t, ts, "logs", "--live=false", "web")
	assertNoError(t, err)
	assertContains(t, out, "2026-01-01T00:00:00Z boot-ok")
}

func TestLogsOtherTypeAndInstance(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies", respond(200, []interface{}{map[string]interface{}{"id": "px-1", "domain": "app.example.com"}}))
	ts.on("GET", "/v2/proxies/px-1/logs", respond(200, []interface{}{map[string]interface{}{"message": "proxy-line"}}))
	out, err := runArgs(t, ts, "logs", "--live=false", "--type", "proxy", "app.example.com")
	assertNoError(t, err)
	assertContains(t, out, "proxy-line")

	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{map[string]interface{}{"id": "sl-1", "name": "web"}}))
	ts.on("GET", "/v2/serverlets/sl-1/instances/i-1/logs", respond(200, []interface{}{map[string]interface{}{"message": "inst-line"}}))
	out, err = runArgs(t, ts, "logs", "--live=false", "--instance", "i-1", "web")
	assertNoError(t, err)
	assertContains(t, out, "inst-line")
}

func TestLogsErrors(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{map[string]interface{}{"id": "sl-1", "name": "web"}}))
	_, err := runArgs(t, ts, "logs", "--live=false", "missing")
	assertError(t, err)
	_, err = runArgs(t, ts, "logs")
	assertError(t, err)
	_, err = runArgs(t, ts, "logs", "--type", "nope", "web")
	assertError(t, err)
}

func TestLogsLs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{map[string]interface{}{"id": "sl-1", "name": "web"}}))
	out, err := runArgs(t, ts, "logs", "--ls")
	assertNoError(t, err)
	assertContains(t, out, "web")
}

func TestLogsTailUsesTailEndpoint(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies/px-1/logs/tail", respond(200, []interface{}{map[string]interface{}{"message": "recent-proxy"}}))
	ts.on("GET", "/v2/dns/dz-1/logs/tail", respond(200, []interface{}{map[string]interface{}{"message": "recent-dns"}}))
	ts.on("GET", "/v2/storages/st-1/logs/tail", respond(200, []interface{}{map[string]interface{}{"message": "recent-storage"}}))

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"proxy", "logs", "--live=false", "--tail", "px-1"}, "recent-proxy"},
		{[]string{"proxy", "logs", "tail", "px-1"}, "recent-proxy"},
		{[]string{"dns", "logs", "--live=false", "--tail", "dz-1"}, "recent-dns"},
		{[]string{"dns", "logs", "tail", "dz-1"}, "recent-dns"},
		{[]string{"storage", "logs", "--live=false", "--tail", "st-1"}, "recent-storage"},
	} {
		out, err := runArgs(t, ts, c.args...)
		assertNoError(t, err)
		assertContains(t, out, c.want)
	}
}

func TestServerletLogsTailKeepsRecentEntries(t *testing.T) {
	ts := newTestServer(t)
	var entries []interface{}
	for i := 0; i < 150; i++ {
		entries = append(entries, map[string]interface{}{"message": fmt.Sprintf("line-%03d", i)})
	}
	ts.on("GET", "/v2/serverlets/sl-1/logs", respond(200, entries))

	out, err := runArgs(t, ts, "serverlet", "logs", "--live=false", "--tail", "sl-1")
	assertNoError(t, err)
	assertContains(t, out, "line-149")
	assertContains(t, out, "line-050")
	if strings.Contains(out, "line-049") {
		t.Fatalf("--tail should keep only the last %d entries", recentLogEntries)
	}
}

func TestLogsFollowPrintsOnlyNewEntries(t *testing.T) {
	old := logsPollInterval
	logsPollInterval = time.Millisecond
	defer func() { logsPollInterval = old }()

	ts := newTestServer(t)
	calls := 0
	ts.on("GET", "/v2/servers/srv-1/logs", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 2 {
			// stop the follow loop once both polls have been observed
			respond(500, fixtureAPIError("stop"))(w, r)
			return
		}
		entries := []interface{}{map[string]interface{}{"message": "first"}}
		if calls == 2 {
			entries = append(entries, map[string]interface{}{"message": "second"})
		}
		respond(200, entries)(w, r)
	})

	out, err := runArgs(t, ts, "server", "logs", "srv-1")
	assertError(t, err)
	if strings.Count(out, "first") != 1 || strings.Count(out, "second") != 1 {
		t.Fatalf("expected each entry once, got:\n%s", out)
	}
}

func TestLogsStorageTailOmitsUnsupportedParams(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/st-1/logs/tail", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("offset") || q.Has("limit") {
			t.Errorf("storage /logs/tail does not accept offset or limit, got %q", r.URL.RawQuery)
		}
		if q.Get("search") != "err" {
			t.Errorf("expected search=err, got %q", r.URL.RawQuery)
		}
		respond(200, []any{map[string]any{"message": "ok-line"}})(w, r)
	})
	out, err := runArgs(t, ts, "storage", "logs", "--live=false", "--tail", "--offset", "5", "--limit", "5", "--search", "err", "st-1")
	assertNoError(t, err)
	assertContains(t, out, "ok-line")
}

func TestLogsAmbiguousNameRequiresID(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{
		map[string]interface{}{"id": "sl-1", "name": "web"},
		map[string]interface{}{"id": "sl-2", "name": "web"},
	}))
	ts.on("GET", "/v2/serverlets/sl-2/logs", respond(200, []interface{}{map[string]interface{}{"message": "by-id"}}))

	_, err := runArgs(t, ts, "logs", "--live=false", "web")
	assertError(t, err)
	assertContains(t, err.Error(), "sl-1, sl-2")
	out, err := runArgs(t, ts, "logs", "--live=false", "sl-2")
	assertNoError(t, err)
	assertContains(t, out, "by-id")
}

func TestLogsJSONFormatPrintsOneEntryPerLine(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{map[string]interface{}{"id": "sl-1", "name": "web"}}))
	ts.on("GET", "/v2/serverlets/sl-1/logs", respond(200, []interface{}{
		map[string]interface{}{"message": "one"}, map[string]interface{}{"message": "two"},
	}))
	out, err := runArgs(t, ts, "--log-format", "json", "logs", "--live=false", "web")
	assertNoError(t, err)
	assertContains(t, out, `{"message":"one"}`)
	assertContains(t, out, `{"message":"two"}`)
}

func TestLogsNonListResponseIsPrintedOnce(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/servers/srv-1/logs", respond(200, map[string]interface{}{"lines": "raw-body"}))
	out, err := runArgs(t, ts, "server", "logs", "--live=false", "srv-1")
	assertNoError(t, err)
	assertContains(t, out, "raw-body")
}

func TestLogsAPIErrorsAndTypeRestrictions(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(500, fixtureAPIError("boom")))
	_, err := runArgs(t, ts, "logs", "web")
	assertError(t, err)

	ts.on("GET", "/v2/proxies", respond(200, []interface{}{map[string]interface{}{"id": "px-1", "domain": "a.example.com"}}))
	_, err = runArgs(t, ts, "logs", "--type", "proxy", "--instance", "i-1", "a.example.com")
	assertError(t, err)

	ts.on("GET", "/v2/proxies/px-1/logs", respond(404, fixtureAPIError("gone")))
	_, err = runArgs(t, ts, "proxy", "logs", "--live=false", "px-1")
	assertError(t, err)
	_, err = runArgs(t, ts, "logs", "--ls", "--type", "nope")
	assertError(t, err)
}

func TestLogsLsJSONAndOtherTypes(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/monitors", respond(200, []interface{}{map[string]interface{}{"id": "m-1", "name": "uptime"}}))
	out, err := runArgs(t, ts, "--log-format", "json", "logs", "--ls", "--type", "monitor")
	assertNoError(t, err)
	assertContains(t, out, "uptime")
	ts.on("GET", "/v2/monitors/m-1/logs/tail", respond(200, []interface{}{map[string]interface{}{"message": "monitor-line"}}))
	out, err = runArgs(t, ts, "logs", "--live=false", "--tail", "--type", "monitor", "uptime")
	assertNoError(t, err)
	assertContains(t, out, "monitor-line")
}

func TestFormatLogEntry(t *testing.T) {
	for name, tc := range map[string]struct {
		entry interface{}
		want  string
	}{
		"timestamp and message": {map[string]interface{}{"timestamp": "T", "message": "m"}, "T m"},
		"line key":              {map[string]interface{}{"line": "only-line"}, "only-line"},
		"time key":              {map[string]interface{}{"time": "T2", "text": "t"}, "T2 t"},
		"no message":            {map[string]interface{}{"level": "info"}, `{"level":"info"}`},
		"plain string":          {"just text", "just text"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := formatLogEntry(tc.entry); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
