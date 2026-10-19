package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// testServer is a programmable HTTP test server for mocking the Futrou API.
type testServer struct {
	*httptest.Server
	routes map[string]http.HandlerFunc
	// fallback answers requests no route matches; it reports whether it did.
	fallback func(w http.ResponseWriter, r *http.Request) bool
}

func init() {
	openBrowserFunc = func(string) {} // never open a real browser during tests
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{routes: make(map[string]http.HandlerFunc)}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if h, ok := ts.routes[key]; ok {
			h(w, r)
			return
		}
		if ts.fallback != nil && ts.fallback(w, r) {
			return
		}
		// The CLI optimistically probes /v2/auth/cli/<challenge>/<redirect_uri>
		// during login to see if the server supports the short-link login flow;
		// unless a test explicitly mocks it, treat it as unsupported (404) rather
		// than a hard test failure, matching how a real server without that route
		// behaves.
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v2/auth/cli/") {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]string{"message": "not found"})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]string{"message": "not found"})
	}))
	t.Cleanup(ts.Server.Close)
	return ts
}

// on registers a handler for "METHOD /path".
func (ts *testServer) on(method, path string, handler http.HandlerFunc) {
	ts.routes[method+" "+path] = handler
}

// respond writes JSON with the given status.
func respond(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		writeJSON(w, body)
	}
}

// respondEmpty writes 204 No Content.
func respondEmpty(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// cloud is a stateful stand-in for the Futrou API on top of testServer: every
// collection lists, creates, patches and deletes like the real service, so a
// command can be run several times against the state the previous run left.
type cloud struct {
	mu       sync.Mutex
	items    map[string][]map[string]any // collection path -> items
	records  map[string][]map[string]any // dns zone ID -> records
	failures map[string]int              // "METHOD /path" -> status to answer with
	log      []string                    // every request, "METHOD /path"
	nextID   int
}

var cloudCollections = []string{"/v2/workspaces", "/v2/projects", "/v2/regions", "/v2/networks", "/v2/serverlets", "/v2/dns", "/v2/proxies", "/v2/storages", "/v2/crons"}

// newCloudServer returns a test server backed by a cloud that already holds
// the workspace "acme", its project "demo" and the region "europe".
func newCloudServer(t *testing.T) (*testServer, *cloud) {
	t.Helper()
	c := &cloud{items: map[string][]map[string]any{}, records: map[string][]map[string]any{}, failures: map[string]int{}}
	for _, path := range cloudCollections {
		c.items[path] = []map[string]any{}
	}
	c.add("/v2/workspaces", map[string]any{"id": "ws-1", "name": "acme"})
	c.add("/v2/projects", map[string]any{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"})
	c.add("/v2/regions", map[string]any{"id": "reg-eu", "name": "europe"})
	ts := newTestServer(t)
	ts.fallback = c.handle
	return ts, c
}

func (c *cloud) add(path string, item map[string]any) map[string]any {
	if item["id"] == nil {
		c.nextID++
		item["id"] = fmt.Sprintf("id-%d", c.nextID)
	}
	c.items[path] = append(c.items[path], item)
	return item
}

// fail makes every "METHOD path" request answer with status.
func (c *cloud) fail(method, path string, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures[method+" "+path] = status
}

// count returns how many requests "METHOD path" were made.
func (c *cloud) count(call string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, entry := range c.log {
		if entry == call {
			n++
		}
	}
	return n
}

// mutations returns the requests that changed something, in order.
func (c *cloud) mutations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, entry := range c.log {
		if !strings.HasPrefix(entry, "GET ") {
			out = append(out, entry)
		}
	}
	return out
}

// named returns the names (or domains) in a collection.
func (c *cloud) named(path string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, item := range c.items[path] {
		name, _ := item["name"].(string)
		if domain, _ := item["domain"].(string); domain != "" {
			name = domain
		}
		out = append(out, name)
	}
	return out
}

func (c *cloud) find(path, id string) (int, map[string]any) {
	for i, item := range c.items[path] {
		if item["id"] == id {
			return i, item
		}
	}
	return -1, nil
}

func (c *cloud) handle(w http.ResponseWriter, r *http.Request) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v2" {
		return false
	}
	base := "/v2/" + parts[1]
	if _, ok := c.items[base]; !ok {
		return false
	}
	c.log = append(c.log, r.Method+" "+r.URL.Path)
	reply := func(status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		writeJSON(w, body)
	}
	if status, ok := c.failures[r.Method+" "+r.URL.Path]; ok {
		reply(status, map[string]string{"message": "forced failure"})
		return true
	}
	if base == "/v2/dns" && len(parts) >= 4 && parts[3] == "records" {
		c.handleRecords(w, r, parts, reply)
		return true
	}
	var body map[string]any
	if r.Method == "POST" || r.Method == "PATCH" {
		decodeBody(r, &body)
	}
	switch {
	case len(parts) == 2 && r.Method == "GET":
		list := []map[string]any{}
		for _, item := range c.items[base] {
			if want := r.URL.Query().Get("projectId"); want != "" {
				if got, ok := item["projectId"].(string); ok && got != want {
					continue
				}
			}
			if want := r.URL.Query().Get("workspaceId"); want != "" {
				if got, ok := item["workspaceId"].(string); ok && got != want {
					continue
				}
			}
			list = append(list, item)
		}
		reply(200, list)
	case len(parts) == 2 && r.Method == "POST":
		c.nextID++
		body["id"] = fmt.Sprintf("id-%d", c.nextID)
		c.items[base] = append(c.items[base], body)
		reply(201, body)
	case len(parts) == 3 && r.Method == "GET":
		if _, item := c.find(base, parts[2]); item != nil {
			reply(200, item)
		} else {
			reply(404, map[string]string{"message": "not found"})
		}
	case len(parts) == 3 && r.Method == "PATCH":
		if _, item := c.find(base, parts[2]); item != nil {
			for key, value := range body {
				item[key] = value
			}
			reply(200, item)
		} else {
			reply(404, map[string]string{"message": "not found"})
		}
	case len(parts) == 3 && r.Method == "DELETE":
		if i, item := c.find(base, parts[2]); item != nil {
			c.items[base] = append(c.items[base][:i:i], c.items[base][i+1:]...)
			w.WriteHeader(http.StatusNoContent)
		} else {
			reply(404, map[string]string{"message": "not found"})
		}
	default:
		reply(405, map[string]string{"message": "method not allowed"})
	}
	return true
}

func (c *cloud) handleRecords(w http.ResponseWriter, r *http.Request, parts []string, reply func(int, any)) {
	zone := parts[2]
	var body map[string]any
	if r.Method == "POST" || r.Method == "PATCH" {
		decodeBody(r, &body)
	}
	switch {
	case len(parts) == 4 && r.Method == "GET":
		list := c.records[zone]
		if list == nil {
			list = []map[string]any{}
		}
		reply(200, list)
	case len(parts) == 4 && r.Method == "POST":
		c.nextID++
		body["id"] = fmt.Sprintf("rec-%d", c.nextID)
		body["dnsId"] = zone
		c.records[zone] = append(c.records[zone], body)
		reply(201, body)
	case len(parts) == 5 && r.Method == "PATCH":
		for _, item := range c.records[zone] {
			if item["id"] == parts[4] {
				for key, value := range body {
					item[key] = value
				}
				reply(200, item)
				return
			}
		}
		reply(404, map[string]string{"message": "not found"})
	case len(parts) == 5 && r.Method == "DELETE":
		for i, item := range c.records[zone] {
			if item["id"] == parts[4] {
				c.records[zone] = append(c.records[zone][:i:i], c.records[zone][i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		reply(404, map[string]string{"message": "not found"})
	default:
		reply(405, map[string]string{"message": "method not allowed"})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	w.Write(data)
}

// decodeBody parses JSON request body.
func decodeBody(r *http.Request, v any) {
	json.NewDecoder(r.Body).Decode(v)
}

// runArgs runs the app with test server URL + fake token injected.
// Returns combined logger output and the run error.
func runArgs(t *testing.T, ts *testServer, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"futrou", "--api-url", ts.URL, "--api-key", "test-token"}, args...)
	return captureRun(fullArgs)
}

// runArgsNoAuth runs without injecting credentials (tests "not logged in"
// path). CI=true is set so requireAuth's auto-login short-circuits to
// projectconfig.ErrCIRequiresToken instead of attempting a real interactive
// OAuth flow against the test server.
func runArgsNoAuth(t *testing.T, ts *testServer, args ...string) (string, error) {
	t.Helper()
	t.Setenv("FUTROU_API_TOKEN", "")
	t.Setenv("CI", "true")
	if os.Getenv("HOME") == "" || strings.HasPrefix(os.Getenv("HOME"), "/home") {
		t.Setenv("HOME", t.TempDir())
	}
	fullArgs := append([]string{"futrou", "--api-url", ts.URL}, args...)
	return captureRun(fullArgs)
}

// captureRun redirects logger output into a buffer, runs the app, returns the captured output.
func captureRun(args []string) (string, error) {
	var buf bytes.Buffer

	// Redirect logger to our buffer
	logger.SetOutput(&buf, &buf)
	defer func() {
		logger.SetOutput(os.Stdout, os.Stderr)
	}()

	// Also capture direct fmt.Print* calls (deploy, init, etc.)
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	app := newApp()
	err := app.Run(args)

	w.Close()
	os.Stdout = origStdout

	var fmtOut bytes.Buffer
	io.Copy(&fmtOut, r)

	combined := buf.String() + fmtOut.String()
	return strings.TrimSpace(combined), err
}

// fixtures

func fixtureServerlet() map[string]any {
	return map[string]any{
		"id":              "sl-123",
		"name":            "my-app",
		"image":           "nginx:latest",
		"status":          "ready",
		"serverletPlanId": "plan-1",
		"createdAt":       "2026-01-01T00:00:00Z",
		"updatedAt":       "2026-01-01T00:00:00Z",
	}
}

func fixtureProxy() map[string]any {
	return map[string]any{
		"id":        "px-456",
		"domain":    "example.com",
		"type":      "http",
		"target":    "localhost",
		"port":      8080,
		"status":    "active",
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureDNSZone() map[string]any {
	return map[string]any{
		"id":        "dns-789",
		"name":      "example.com",
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureDNSRecord() map[string]any {
	return map[string]any{
		"id":        "rec-001",
		"dnsId":     "dns-789",
		"name":      "www",
		"type":      "A",
		"value":     "1.2.3.4",
		"ttl":       300,
		"priority":  0,
		"createdAt": "2026-01-01T00:00:00Z",
	}
}

func fixtureProject() map[string]any {
	return map[string]any{
		"id":          "proj-abc",
		"name":        "my-project",
		"displayName": "My Project",
		"workspaceId": "ws-xyz",
		"createdAt":   "2026-01-01T00:00:00Z",
	}
}

func fixtureWorkspace() map[string]any {
	return map[string]any{
		"id":          "ws-abc",
		"name":        "my-workspace",
		"displayName": "My Workspace",
		"createdAt":   "2026-01-01T00:00:00Z",
	}
}

func fixtureStorage() map[string]any {
	return map[string]any{
		"id":            "storage-def",
		"name":          "my-storage",
		"storagePlanId": "storage-plan-1",
		"createdAt":     "2026-01-01T00:00:00Z",
	}
}

func fixtureAPIError(msg string) map[string]any {
	return map[string]any{"message": msg, "statusCode": 401}
}

// assertions

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q\ngot: %s", substr, s)
	}
}

func TestGlobalFlags_apiUrl(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	out, err := runArgs(t, ts, "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_apiToken_env(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	t.Setenv("FUTROU_API_TOKEN", "env-token")
	t.Setenv("HOME", t.TempDir())

	// run without --api-key flag; token comes from env
	args := []string{"futrou", "--api-url", ts.URL, "serverlets", "list"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_logLevel(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	out, err := runArgs(t, ts, "--log-level", "debug", "serverlets", "list")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_logFormat_json(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets/sl-123", respond(200, fixtureServerlet()))

	out, err := runArgs(t, ts, "--log-format", "json", "serverlets", "get", "sl-123")
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

func TestGlobalFlags_apiKey_alias(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(200, []interface{}{fixtureServerlet()}))

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	// --api-token alias should work the same as --api-key
	args := []string{"futrou", "--api-url", ts.URL, "--api-token", "test-token", "serverlets", "list"}
	out, err := captureRun(args)
	assertNoError(t, err)
	assertContains(t, out, "sl-123")
}

// leafCommands returns the argument path and required flags of every visible
// leaf command below cmds, with placeholder values for what it requires.
func leafCommands(cmds []*cli.Command, prefix []string) [][]string {
	var out [][]string
	for _, cmd := range cmds {
		if cmd.Hidden || cmd.Name == "help" {
			continue
		}
		path := append(append([]string{}, prefix...), cmd.Name)
		if len(cmd.Subcommands) > 0 {
			out = append(out, leafCommands(cmd.Subcommands, path)...)
			// "instance <id>" style commands are leaves too
			if cmd.Action == nil {
				continue
			}
		}
		args := append([]string{}, path...)
		for _, flag := range cmd.Flags {
			switch f := flag.(type) {
			case *cli.StringFlag:
				if f.Required {
					args = append(args, "--"+f.Name, "x")
				}
			case *cli.IntFlag:
				if f.Required {
					args = append(args, "--"+f.Name, "1")
				}
			}
		}
		out = append(out, append(args, "id-1", "id-2", "id-3"))
	}
	return out
}

// Commands that neither need credentials nor call the API, or that need more
// than an ID to run (a config file, a terminal, a real binary to upgrade).
var commandsWithoutAPIFailures = map[string]bool{
	"login": true, "logout": true, "version": true, "license": true, "upgrade": true, "init": true,
	"config": true, "schema": true, "deploy": true, "up": true, "down": true,
	"whoami": true, "shell": true, "exec": true, "logs": true,
}

func TestEveryCommandFailsWhenTheAPIFails(t *testing.T) {
	reconnectUnit = time.Millisecond
	t.Cleanup(func() { reconnectUnit = time.Second })
	app := newApp()
	for _, args := range leafCommands(app.Commands, nil) {
		if commandsWithoutAPIFailures[args[0]] {
			continue
		}
		name := strings.Join(args[:len(args)-3], " ")
		t.Run(name, func(t *testing.T) {
			withWorkingDirectory(t, t.TempDir())
			ts := newTestServer(t)
			ts.fallback = func(w http.ResponseWriter, r *http.Request) bool {
				respond(500, map[string]string{"message": "boom"})(w, r)
				return true
			}
			if _, err := runArgs(t, ts, args...); err == nil {
				t.Fatalf("%v must fail when every API call fails", args)
			}
		})
	}
}

func TestEveryCommandNeedsCredentials(t *testing.T) {
	app := newApp()
	for _, args := range leafCommands(app.Commands, nil) {
		if commandsWithoutAPIFailures[args[0]] {
			continue
		}
		name := strings.Join(args[:len(args)-3], " ")
		t.Run(name, func(t *testing.T) {
			withWorkingDirectory(t, t.TempDir())
			ts := newTestServer(t)
			ts.fallback = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("Authorization") == "" && r.Header.Get("Cookie") == "" && !strings.Contains(r.Header.Get("Authorization"), "Bearer") {
					t.Errorf("%v sent an unauthenticated request: %s %s", args, r.Method, r.URL.Path)
				}
				respond(500, map[string]string{"message": "boom"})(w, r)
				return true
			}
			if _, err := runArgsNoAuth(t, ts, args...); err == nil {
				t.Fatalf("%v must fail without credentials", args)
			}
		})
	}
}

// probeApp runs the real root app with one extra command that reports what
// the global helpers resolve to.
func probeApp(t *testing.T, args ...string) (apiURL, apiKey string, err error) {
	t.Helper()
	// The flag structs are package-level and urfave stores parsed values in
	// them, so a value read from the environment would leak into the next run.
	for _, flag := range append(append([]cli.Flag{}, appLevelFlags...), perCommandFlags...) {
		if f, ok := flag.(*cli.StringFlag); ok {
			f.Value = map[string]string{"api-url": constants.ApiUrl, "log-format": constants.DefaultLogFormat, "log-level": constants.DefaultLogLevel}[f.Name]
			f.HasBeenSet = false
		}
		if f, ok := flag.(*cli.BoolFlag); ok {
			f.Value, f.HasBeenSet = false, false
		}
	}
	app := buildApp()
	app.Commands = []*cli.Command{withGlobalFlags(&cli.Command{Name: "probe", Action: func(c *cli.Context) error {
		apiURL, apiKey = globalApiUrl(c), globalApiKey(c)
		return nil
	}})}
	err = app.Run(append([]string{"futrou"}, args...))
	return
}

func TestGlobalFlagShorthandsResolveTheAPIURL(t *testing.T) {
	unsetEnv(t, "FUTROU_API_URL")
	unsetEnv(t, "FUTROU_API_TOKEN")
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"default":                      {[]string{"probe"}, services.NormalizeApiUrl(constants.ApiUrl)},
		"--local":                      {[]string{"--local", "probe"}, services.NormalizeApiUrl(constants.LocalApiUrl)},
		"--dev":                        {[]string{"--dev", "probe"}, services.NormalizeApiUrl(constants.DevApiUrl)},
		"--local after the command":    {[]string{"probe", "--local"}, services.NormalizeApiUrl(constants.LocalApiUrl)},
		"--dev after the command":      {[]string{"probe", "--dev"}, services.NormalizeApiUrl(constants.DevApiUrl)},
		"explicit url beats --local":   {[]string{"--api-url", "https://example.com/v2", "--local", "probe"}, "https://example.com"},
		"bare host gets https":         {[]string{"--api-url", "api.example.com", "probe"}, "https://api.example.com"},
		"loopback host:port gets http": {[]string{"--api-url", "localhost:4000", "probe"}, "http://localhost:4000"},
	} {
		t.Run(name, func(t *testing.T) {
			got, _, err := probeApp(t, tc.args...)
			if err != nil || got != tc.want {
				t.Fatalf("api url = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// unsetEnv removes a variable for the test; t.Setenv(name, "") would leave it
// set-but-empty, which urfave treats as a value.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	os.Unsetenv(name)
}

func TestShorthandsStillWorkWhenTheURLVariableIsEmpty(t *testing.T) {
	t.Setenv("FUTROU_API_URL", "")
	t.Setenv("FUTROU_API_TOKEN", "")
	if got, _, err := probeApp(t, "--local", "probe"); err != nil || got != services.NormalizeApiUrl(constants.LocalApiUrl) {
		t.Fatalf("an empty FUTROU_API_URL must not disable --local: %q, %v", got, err)
	}
	if got, _, _ := probeApp(t, "--local", "--dev", "probe"); got != services.NormalizeApiUrl(constants.LocalApiUrl) {
		t.Fatalf("the first shorthand wins: %q", got)
	}
}

func TestGlobalAPIURLAndKeyComeFromTheEnvironment(t *testing.T) {
	t.Setenv("FUTROU_API_URL", "https://env.example.com")
	t.Setenv("FUTROU_API_TOKEN", "env-token")
	url, key, err := probeApp(t, "probe")
	if err != nil || url != "https://env.example.com" || key != "env-token" {
		t.Fatalf("got %q %q, %v", url, key, err)
	}
	// flags win over the environment
	url, key, _ = probeApp(t, "--api-url", "https://flag.example.com", "--api-token", "flag-token", "probe")
	if url != "https://flag.example.com" || key != "flag-token" {
		t.Fatalf("got %q %q", url, key)
	}
}

func TestGlobalAPIKeyAliasAndPrecedence(t *testing.T) {
	unsetEnv(t, "FUTROU_API_URL")
	unsetEnv(t, "FUTROU_API_TOKEN")
	if _, key, _ := probeApp(t, "--api-key", "alias", "probe"); key != "alias" {
		t.Fatalf("--api-key is an alias of --api-token: %q", key)
	}
	if _, key, _ := probeApp(t, "--api-token", "token", "--api-key", "alias", "probe"); key != "token" {
		t.Fatalf("--api-token wins over its alias: %q", key)
	}
	if _, key, _ := probeApp(t, "probe"); key != "" {
		t.Fatalf("no key configured: %q", key)
	}
}

func TestBuildAppPrintsTheVersionAndHelpWithoutACommand(t *testing.T) {
	out, err := captureRun([]string{"futrou", "--version"})
	assertNoError(t, err)
	assertContains(t, out, constants.Version)
	out, err = captureRun([]string{"futrou"})
	assertNoError(t, err)
	assertContains(t, out, "Commands:")
}
