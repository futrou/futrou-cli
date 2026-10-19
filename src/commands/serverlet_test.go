package commands

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"futrou-cli/src/services"

	"github.com/gorilla/websocket"
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

// shellServer is a websocket stand-in for the serverlet shell endpoint. The
// handler gets the connection after the upgrade; cookie records the
// authorization the client sent.
type shellServer struct {
	*httptest.Server
	mu     sync.Mutex
	cookie string
	path   string
}

func newShellServer(t *testing.T, handler func(*websocket.Conn)) *shellServer {
	t.Helper()
	s := &shellServer{}
	upgrader := websocket.Upgrader{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.cookie, s.path = r.Header.Get("Cookie"), r.URL.Path
		s.mu.Unlock()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	t.Cleanup(s.Server.Close)
	return s
}

func (s *shellServer) wsURL() string {
	return "ws" + strings.TrimPrefix(s.URL, "http") + "/v2/serverlets/sl-1/shell"
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = original
	return <-done
}

func TestShellWebsocketURL(t *testing.T) {
	for name, tc := range map[string]struct {
		apiURL, path, want, wantErr string
	}{
		"https becomes wss":  {"https://api.futrou.com", "/v2/x/shell", "wss://api.futrou.com/v2/x/shell", ""},
		"http becomes ws":    {"http://localhost:4000", "/v2/x/shell", "ws://localhost:4000/v2/x/shell", ""},
		"trailing slash":     {"https://api.futrou.com/", "/v2/x/shell", "wss://api.futrou.com/v2/x/shell", ""},
		"base path is kept":  {"https://example.com/api", "/v2/x/shell", "wss://example.com/api/v2/x/shell", ""},
		"unsupported scheme": {"ftp://example.com", "/v2/x", "", "unsupported API URL scheme"},
		"no scheme":          {"example.com", "/v2/x", "", "unsupported API URL scheme"},
		"unparsable URL":     {"http://[::1", "/v2/x", "", "parsing API URL"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := shellWebsocketURL(tc.apiURL, tc.path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestShellWebsocketURLForTargetsAnInstance(t *testing.T) {
	client := services.NewApiClientWithToken("https://api.futrou.com", "token")
	if got, _ := shellWebsocketURLFor(client, "sl-1", ""); got != "wss://api.futrou.com/v2/serverlets/sl-1/shell" {
		t.Fatalf("url = %s", got)
	}
	if got, _ := shellWebsocketURLFor(client, "sl-1", "i-2"); got != "wss://api.futrou.com/v2/serverlets/sl-1/instances/i-2/shell" {
		t.Fatalf("url = %s", got)
	}
}

func TestReconnectBackoffGrowsWithEachAttempt(t *testing.T) {
	if reconnectBackoff(1) != time.Second || reconnectBackoff(3) != 3*time.Second {
		t.Fatalf("backoff = %v %v", reconnectBackoff(1), reconnectBackoff(3))
	}
}

func TestParseSentinelLine(t *testing.T) {
	for name, tc := range map[string]struct {
		line string
		code int
		ok   bool
	}{
		"exit zero":          {"__S__:0", 0, true},
		"non-zero":           {"__S__:127", 127, true},
		"after a prompt":     {"user@host:~$ __S__:2", 2, true},
		"padded":             {"__S__: 5 ", 5, true},
		"not a number":       {"__S__:abc", 0, false},
		"other line":         {"hello", 0, false},
		"the echoed command": {"echo __S__:$?", 0, false},
		"empty":              {"", 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			code, ok := parseSentinelLine(tc.line, "__S__")
			if code != tc.code || ok != tc.ok {
				t.Fatalf("got %d, %v; want %d, %v", code, ok, tc.code, tc.ok)
			}
		})
	}
}

func TestDialShellSendsTheTokenAndRetries(t *testing.T) {
	reconnectUnit = time.Millisecond
	t.Cleanup(func() { reconnectUnit = time.Second })

	server := newShellServer(t, func(conn *websocket.Conn) {})
	conn, err := dialShellWithRetry(server.wsURL(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if server.cookie != "authorization=secret" {
		t.Fatalf("cookie = %q", server.cookie)
	}

	// nothing listens: every attempt fails and the error says how often it tried
	dead := httptest.NewServer(http.NotFoundHandler())
	url := "ws" + strings.TrimPrefix(dead.URL, "http")
	dead.Close()
	_, err = dialShellWithRetry(url, "secret")
	if err == nil || !strings.Contains(err.Error(), "after 4 attempts") {
		t.Fatalf("error = %v", err)
	}

	// an endpoint that refuses the upgrade fails the same way
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 403) }))
	defer refuse.Close()
	if _, err := dialShellWithRetry("ws"+strings.TrimPrefix(refuse.URL, "http"), "x"); err == nil {
		t.Fatal("a refused upgrade must fail")
	}
}

func TestRunExecReturnsTheExitCodeAndStreamsOutput(t *testing.T) {
	for name, tc := range map[string]struct {
		output   func(sentinel string) []string // websocket messages the server sends back
		wantCode int
		wantOut  string
	}{
		"success": {func(s string) []string { return []string{"hello\r\n", s + ":0\r\n"} }, 0, "hello\n"},
		"failure": {func(s string) []string { return []string{"boom\r\n", s + ":3\r\n"} }, 3, "boom\n"},
		"split across messages": {func(s string) []string {
			return []string{"par", "tial\r\nnext\r\n" + s[:8], s[8:] + ":9\r\n"}
		}, 9, "partial\nnext\n"},
		"echoed command is shown but not the sentinel result": {func(s string) []string {
			return []string{"$ ls; echo " + s + ":$?\r\n", "file\r\n", s + ":0\r\n"}
		}, 0, "$ ls; echo"},
		"trailing text after the sentinel": {func(s string) []string { return []string{s + ":1\r\nprompt$ "} }, 1, "prompt$ "},
	} {
		t.Run(name, func(t *testing.T) {
			server := newShellServer(t, func(conn *websocket.Conn) {
				_, payload, err := conn.ReadMessage()
				if err != nil {
					return
				}
				sentinel := string(payload[strings.Index(string(payload), "__FUTROU_EXIT_"):])
				sentinel = sentinel[:strings.Index(sentinel, ":")]
				for _, message := range tc.output(sentinel) {
					conn.WriteMessage(websocket.TextMessage, []byte(message))
				}
			})
			var code int
			var err error
			out := captureStdout(t, func() { code, err = runExec(server.wsURL(), "tok", "ls -la") })
			if err != nil || code != tc.wantCode {
				t.Fatalf("code = %d, err = %v; want %d", code, err, tc.wantCode)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output = %q, want it to contain %q", out, tc.wantOut)
			}
			if strings.Contains(out, "__FUTROU_EXIT_") && name != "echoed command is shown but not the sentinel result" {
				t.Fatalf("the sentinel must never be shown: %q", out)
			}
		})
	}
}

func TestRunExecSendsTheCommandWithASentinel(t *testing.T) {
	got := make(chan string, 1)
	server := newShellServer(t, func(conn *websocket.Conn) {
		_, payload, _ := conn.ReadMessage()
		got <- string(payload)
	})
	captureStdout(t, func() { runExec(server.wsURL(), "tok", "echo hi") })
	payload := <-got
	if !strings.HasPrefix(payload, "echo hi; echo __FUTROU_EXIT_") || !strings.HasSuffix(payload, ":$?\n") {
		t.Fatalf("payload = %q", payload)
	}
}

func TestRunExecFailsWhenTheConnectionDropsBeforeTheExitCode(t *testing.T) {
	server := newShellServer(t, func(conn *websocket.Conn) {
		conn.ReadMessage()
		conn.WriteMessage(websocket.TextMessage, []byte("partial output"))
	})
	var err error
	captureStdout(t, func() { _, err = runExec(server.wsURL(), "tok", "sleep 100") })
	if err == nil || !strings.Contains(err.Error(), "command did not complete") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunExecIgnoresNonDataFrames(t *testing.T) {
	server := newShellServer(t, func(conn *websocket.Conn) {
		_, payload, _ := conn.ReadMessage()
		s := string(payload[strings.Index(string(payload), "__FUTROU_EXIT_"):])
		conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
		conn.WriteMessage(websocket.TextMessage, []byte(s[:strings.Index(s, ":")]+":0\n"))
	})
	var code int
	var err error
	captureStdout(t, func() { code, err = runExec(server.wsURL(), "tok", "true") })
	if err != nil || code != 0 {
		t.Fatalf("code = %d, err = %v", code, err)
	}
}

func TestRunExecFailsWhenItCannotConnect(t *testing.T) {
	reconnectUnit = time.Millisecond
	t.Cleanup(func() { reconnectUnit = time.Second })
	dead := httptest.NewServer(http.NotFoundHandler())
	url := "ws" + strings.TrimPrefix(dead.URL, "http")
	dead.Close()
	if _, err := runExec(url, "tok", "ls"); err == nil || !strings.Contains(err.Error(), "could not connect") {
		t.Fatalf("error = %v", err)
	}
}

func TestShellAndExecCommandsValidateTheirArguments(t *testing.T) {
	ts := newTestServer(t)
	for args, want := range map[string]string{
		"serverlet shell":        "serverlet ID required",
		"serverlet exec":         "serverlet ID required",
		"serverlet exec sl-1":    "command required",
		"serverlet exec sl-1 --": "command required",
	} {
		_, err := runArgs(t, ts, strings.Fields(args)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want %q", args, err, want)
		}
	}
	// without a terminal an interactive shell cannot start
	if err := runShell("ws://127.0.0.1:1/x", "tok"); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("runShell without a terminal: %v", err)
	}
}

func withPipedStdin(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		// pumpShell leaves its stdin reader running; closing the pipe ends it,
		// and it needs a moment to notice before os.Stdin is put back.
		w.Close()
		time.Sleep(50 * time.Millisecond)
		os.Stdin = original
		r.Close()
	})
	return w
}

func TestPumpShellForwardsKeystrokesAndPrintsServerOutput(t *testing.T) {
	received := make(chan []byte, 1)
	server := newShellServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.BinaryMessage, []byte("welcome\r\n"))
		var typed []byte
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				received <- typed
				return
			}
			typed = append(typed, data...)
		}
	})
	stdin := withPipedStdin(t)
	conn, err := dialShellWithRetry(server.wsURL(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// "~x" at the start of a line is ordinary input; "~." then disconnects
	out := captureStdout(t, func() {
		go stdin.Write([]byte("ls\r~x\r~."))
		pumpShell(conn)
	})
	if !strings.Contains(out, "welcome") {
		t.Fatalf("server output was not printed: %q", out)
	}
	if typed := string(<-received); typed != "ls\r~x\r" {
		t.Fatalf("the server received %q, want %q (the ~. escape must not be forwarded)", typed, "ls\r~x\r")
	}
}

func TestPumpShellStopsWhenTheServerCloses(t *testing.T) {
	server := newShellServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.TextMessage, []byte("bye\n"))
	})
	withPipedStdin(t)
	conn, err := dialShellWithRetry(server.wsURL(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan struct{})
	var out string
	go func() {
		out = captureStdout(t, func() { pumpShell(conn) })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pumpShell must return when the server ends the session")
	}
	if !strings.Contains(out, "bye") {
		t.Fatalf("output = %q", out)
	}
}
