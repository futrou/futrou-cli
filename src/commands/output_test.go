package commands

import (
	"net/http"
	"testing"
)

func TestMetricsAction(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dz-1/metrics", respond(200, map[string]any{"queries": "dns-metric"}))
	ts.on("GET", "/v2/storages/st-1/metrics", respond(200, map[string]any{"bytes": "storage-metric"}))
	ts.on("GET", "/v2/proxies/px-1/metrics", respond(200, map[string]any{"requests": "proxy-metric"}))

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"dns", "metrics", "dz-1"}, "dns-metric"},
		{[]string{"storage", "metrics", "st-1"}, "storage-metric"},
		{[]string{"proxy", "metrics", "px-1"}, "proxy-metric"},
	} {
		out, err := runArgs(t, ts, c.args...)
		assertNoError(t, err)
		assertContains(t, out, c.want)
	}
	for _, resource := range []string{"dns", "storage"} {
		_, err := runArgs(t, ts, resource, "metrics")
		assertError(t, err)
	}
}

func TestLogQuery_allParams(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies/px-1/logs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		want := map[string]string{"offset": "10", "limit": "50", "search": "error", "startAt": "2026-01-01", "endAt": "2026-01-02"}
		for k, v := range want {
			if q.Get(k) != v {
				t.Errorf("expected %s=%s, got %q", k, v, q.Get(k))
			}
		}
		respond(200, []any{})(w, r)
	})
	_, err := runArgs(t, ts, "proxy", "logs", "--live=false", "--offset", "10", "--limit", "50", "--search", "error",
		"--start-at", "2026-01-01", "--end-at", "2026-01-02", "px-1")
	assertNoError(t, err)
}

func TestMetricsAction_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dz-1/metrics", respond(500, fixtureAPIError("boom")))
	_, err := runArgs(t, ts, "dns", "metrics", "dz-1")
	assertError(t, err)
}

func TestLogFlagsDefaultsAndQuery(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/st-1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("no log flags set, expected no query, got %q", r.URL.RawQuery)
		}
		respond(200, []any{})(w, r)
	})
	_, err := runArgs(t, ts, "storage", "logs", "--live=false", "st-1")
	assertNoError(t, err)
}
