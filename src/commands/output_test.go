package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"futrou-cli/src/logger"
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

func TestJSONErrorHelpers(t *testing.T) {
	err := printJSONError(404, map[string]string{"message": "nope"})
	var jsonErr *jsonError
	if !errors.As(err, &jsonErr) || jsonErr.Status != 404 || err.Error() != "request failed with status 404" {
		t.Fatalf("error = %#v", err)
	}
	var buf bytes.Buffer
	writeJSONError(&buf, 422, map[string]string{"message": "bad"})
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil || decoded["status"] != float64(422) || decoded["error"].(map[string]any)["message"] != "bad" {
		t.Fatalf("output = %s (%v)", buf.String(), err)
	}
	buf.Reset()
	writeJSONError(&buf, 500, make(chan int)) // cannot be marshaled: still prints something
	if strings.TrimSpace(buf.String()) == "" {
		t.Fatal("an unmarshalable body must still be reported")
	}
}

func TestPrintJSONAndIsJSON(t *testing.T) {
	if isJSON(nil) {
		t.Fatal("a nil context is never JSON")
	}
	if err := printJSON(make(chan int)); err == nil {
		t.Fatal("an unmarshalable value must be an error")
	}
	var buf bytes.Buffer
	logger.SetOutput(&buf, &buf)
	defer logger.SetOutput(os.Stdout, os.Stderr)
	if err := printJSON(map[string]any{"a": 1}); err != nil || !strings.Contains(buf.String(), `"a": 1`) {
		t.Fatalf("printJSON wrote %q, %v", buf.String(), err)
	}
}

func TestPrintTableShapes(t *testing.T) {
	cols := []string{"id", "name"}
	for name, tc := range map[string]struct {
		data any
		want string
	}{
		"rows":            {[]any{map[string]any{"id": "1", "name": "web"}}, "web"},
		"wrapped in data": {map[string]any{"data": []any{map[string]any{"id": "2", "name": "api"}}}, "api"},
		"empty list":      {[]any{}, "No results."},
		"empty wrapper":   {map[string]any{"data": []any{}}, "data"},
		"not a table":     {"plain text", "plain text"},
		"missing column":  {[]any{map[string]any{"id": "3"}}, "3"},
		"unmarshalable":   {make(chan int), "0x"},
	} {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() { printTable(tc.data, cols) })
			if !strings.Contains(out, tc.want) {
				t.Fatalf("output = %q, want it to contain %q", out, tc.want)
			}
		})
	}
	out := captureStdout(t, func() {
		printTable([]any{map[string]any{"id": "1", "name": "a"}, map[string]any{"id": "22222", "name": "b"}}, cols)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "ID") {
		t.Fatalf("table = %q", out)
	}
}
