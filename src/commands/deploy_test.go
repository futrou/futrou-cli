package commands

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployCreatesEveryDeclaredResourceWithYes(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{
  "serverlets": [{"name":"web","image":"nginx:latest","serverletPlanId":"plan-1"}],
  "storages": [{"name":"data","storagePlanId":"storage-plan-1"}]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	var created []string
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/storages", respond(http.StatusOK, []any{}))
	ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeBody(r, &body)
		if body["name"] != "web" || body["image"] != "nginx:latest" {
			t.Errorf("unexpected serverlet payload: %#v", body)
		}
		created = append(created, "serverlet")
		writeJSON(w, map[string]any{"id": "sl-1"})
	})
	ts.on("POST", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeBody(r, &body)
		if body["name"] != "data" || body["storagePlanId"] != "storage-plan-1" {
			t.Errorf("unexpected storage payload: %#v", body)
		}
		created = append(created, "storage")
		writeJSON(w, map[string]any{"id": "storage-1"})
	})

	out, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Changes:")
	assertContains(t, out, "Applied 2 change(s).")
	if strings.Join(created, ",") != "serverlet,storage" {
		t.Fatalf("expected both creates, got %#v", created)
	}
}

func TestDeployReportsUpToDateWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web","image":"nginx:latest","serverletPlanId":"plan-1"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{
		map[string]any{"id": "sl-1", "name": "web", "image": "nginx:latest", "serverletPlanId": "plan-1"},
	}))

	out, err := runArgs(t, ts, "deploy")
	assertNoError(t, err)
	assertContains(t, out, "No changes. Infrastructure is up to date.")
	if strings.Contains(out, "Apply these changes?") {
		t.Fatalf("must not prompt when there are no changes: %s", out)
	}
}

func TestDeployDestroyDeletesMatchingDeclaredResources(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{map[string]any{"id": "sl-1", "name": "web"}}))
	deleted := false
	ts.on("DELETE", "/v2/serverlets/sl-1", func(w http.ResponseWriter, r *http.Request) {
		deleted = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "deploy", "--destroy", "-y")
	assertNoError(t, err)
	assertContains(t, out, "Applied 1 change(s).")
	if !deleted {
		t.Fatal("expected declared serverlet to be deleted")
	}
}

func TestDeploy_ciWithoutTokenFailsFast(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CI", "true")
	t.Setenv("FUTROU_API_TOKEN", "")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	browserOpened := false
	prevOpen := openBrowserFunc
	openBrowserFunc = func(string) { browserOpened = true }
	t.Cleanup(func() { openBrowserFunc = prevOpen })

	_, err := captureRun([]string{"futrou", "--api-url", "https://unused.example.com", "deploy", "--yes"})
	if err == nil {
		t.Fatal("expected error when CI=true and no token is available")
	}
	if !strings.Contains(err.Error(), "CI environment requires") {
		t.Fatalf("error = %v, want mention of CI requiring a token", err)
	}
	if browserOpened {
		t.Fatal("deploy must not attempt an interactive login under CI")
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
}

// withStdin replaces os.Stdin with the given text for the test.
func withStdin(t *testing.T, text string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig; f.Close() })
}

func writeDeployConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"storages":[{"name":"data","storagePlanId":"plan-1"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
}

func deployServer(t *testing.T) (*testServer, *int) {
	ts := newTestServer(t)
	creates := 0
	ts.on("GET", "/v2/storages", respond(http.StatusOK, []any{}))
	ts.on("POST", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		creates++
		writeJSON(w, map[string]any{"id": "storage-1"})
	})
	return ts, &creates
}

func TestDeployConfirmation(t *testing.T) {
	for name, tc := range map[string]struct {
		stdin   string
		applied bool
	}{
		"y applies":        {"y\n", true},
		"yes applies":      {"yes\n", true},
		"YES applies":      {"YES\n", true},
		"enter cancels":    {"\n", false},
		"n cancels":        {"n\n", false},
		"no input cancels": {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			writeDeployConfig(t)
			t.Setenv("CI", "")
			withStdin(t, tc.stdin)
			ts, creates := deployServer(t)

			out, err := runArgs(t, ts, "deploy")
			assertNoError(t, err)
			assertContains(t, out, "Apply these changes? [y/N]")
			if tc.applied && *creates != 1 {
				t.Fatalf("expected the change to be applied, got %d creates", *creates)
			}
			if !tc.applied {
				assertContains(t, out, "Cancelled.")
				if *creates != 0 {
					t.Fatalf("nothing should be applied, got %d creates", *creates)
				}
			}
		})
	}
}

func TestDeployYesAndCIApplyWithoutPrompt(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		ci   string
	}{
		"--yes": {[]string{"deploy", "--yes"}, ""},
		"-y":    {[]string{"deploy", "-y"}, ""},
		"CI":    {[]string{"deploy"}, "true"},
		"up":    {[]string{"up", "-y"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			writeDeployConfig(t)
			t.Setenv("CI", tc.ci)
			withStdin(t, "")
			ts, creates := deployServer(t)

			out, err := runArgs(t, ts, tc.args...)
			assertNoError(t, err)
			if strings.Contains(out, "[y/N]") {
				t.Fatalf("must not prompt: %s", out)
			}
			if *creates != 1 {
				t.Fatalf("expected the change to be applied, got %d creates", *creates)
			}
		})
	}
}

func TestDeployDryRunOnlyShowsPlan(t *testing.T) {
	writeDeployConfig(t)
	ts, creates := deployServer(t)

	out, err := runArgs(t, ts, "deploy", "--dry-run", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Create (1)")
	if *creates != 0 || strings.Contains(out, "Applied") {
		t.Fatalf("dry run must not apply anything: %s", out)
	}
}

func TestDeployRejectsInvalidConfigBeforeCallingAPI(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web","ports":[{"port":70000}],"networks":[1]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t) // any request fails the test

	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "serverlets")
}

func TestDeployShowsUpdateAndDeleteDetails(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web","image":"nginx:2","serverletPlanId":"plan-1"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	var patched map[string]any
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{map[string]any{"id": "sl-1", "name": "web", "image": "nginx:1", "serverletPlanId": "plan-1"}}))
	ts.on("PATCH", "/v2/serverlets/sl-1", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &patched)
		respond(http.StatusOK, map[string]any{"id": "sl-1"})(w, r)
	})

	out, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Update (1)")
	assertContains(t, out, `image: "nginx:1" → "nginx:2"`)
	if patched["image"] != "nginx:2" || len(patched) != 1 {
		t.Fatalf("only the changed field must be sent, got %#v", patched)
	}
}

func TestDeployJSONPrintsAppliedPlan(t *testing.T) {
	writeDeployConfig(t)
	ts, creates := deployServer(t)
	out, err := runArgs(t, ts, "--log-format", "json", "deploy", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "storage")
	if *creates != 1 {
		t.Fatalf("expected one create, got %d", *creates)
	}
}

func TestDeployApplyFailureIsReported(t *testing.T) {
	writeDeployConfig(t)
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages", respond(http.StatusOK, []any{}))
	ts.on("POST", "/v2/storages", respond(http.StatusUnprocessableEntity, fixtureAPIError("quota exceeded")))
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "quota exceeded")
}

func TestDeployReadsYAMLConfig(t *testing.T) {
	for _, name := range []string{"futrou.yaml", "futrou.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDirectory(t, dir)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("networks:\n  - name: backend\n    ipv4: 10.0.0.0/24\nserverlets:\n  - name: web\n    image: nginx:latest\n    networks: [backend]\n    ports:\n      - port: 8080\n"), 0644); err != nil {
				t.Fatal(err)
			}
			ts := newTestServer(t)
			var created []string
			var serverlet map[string]any
			ts.on("GET", "/v2/networks", respond(http.StatusOK, []any{}))
			ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
			ts.on("POST", "/v2/networks", func(w http.ResponseWriter, r *http.Request) {
				created = append(created, "network")
				writeJSON(w, map[string]any{"id": "net-1"})
			})
			ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
				created = append(created, "serverlet")
				decodeBody(r, &serverlet)
				writeJSON(w, map[string]any{"id": "sl-1"})
			})

			out, err := runArgs(t, ts, "deploy", "--yes")
			assertNoError(t, err)
			assertContains(t, out, "Using config: "+name)
			if strings.Join(created, ",") != "network,serverlet" {
				t.Fatalf("created = %v", created)
			}
			if nets, _ := serverlet["networks"].([]any); len(nets) != 1 || nets[0] != "backend" {
				t.Fatalf("unexpected serverlet payload: %#v", serverlet)
			}
			if data, _ := os.ReadFile(filepath.Join(dir, name)); !strings.HasPrefix(string(data), "networks:") {
				t.Fatalf("a config without a project has nothing to refresh and must stay untouched, got: %s", data)
			}
		})
	}
}

func TestDeployInvalidYAMLFailsBeforeCallingAPI(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.yaml"), []byte("serverlets: [unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t) // any request fails the test
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "futrou.yaml")
}

func TestDeployAppliesProjectRegion(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.yaml"), []byte("region: europe\nserverlets:\n  - name: web\n    image: nginx\nstorages:\n  - name: data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	bodies := map[string]map[string]any{}
	ts.on("GET", "/v2/regions", respond(http.StatusOK, []any{map[string]any{"id": "reg-eu", "name": "europe"}}))
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/storages", respond(http.StatusOK, []any{}))
	for _, resource := range []string{"serverlets", "storages"} {
		ts.on("POST", "/v2/"+resource, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			decodeBody(r, &body)
			bodies[resource] = body
			writeJSON(w, map[string]any{"id": resource + "-1"})
		})
	}

	_, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	for _, resource := range []string{"serverlets", "storages"} {
		if bodies[resource]["regionId"] != "reg-eu" {
			t.Errorf("%s payload = %#v, want regionId reg-eu", resource, bodies[resource])
		}
	}
}

func TestDeployUnknownRegionStopsBeforeChanges(t *testing.T) {
	writeDeployConfigWithRegion(t, "mars")
	ts := newTestServer(t)
	ts.on("GET", "/v2/regions", respond(http.StatusOK, []any{map[string]any{"id": "reg-eu", "name": "europe"}}))
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), `region "mars" not found`)
}

func writeDeployConfigWithRegion(t *testing.T, region string) {
	t.Helper()
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"region":"`+region+`","storages":[{"name":"data"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
}

// projectMocks answers the API calls a config pull makes for project "demo".
func projectMocks(ts *testServer, serverlets []any) {
	ts.on("GET", "/v2/workspaces", respond(http.StatusOK, []any{map[string]any{"id": "ws-1", "name": "acme"}}))
	ts.on("GET", "/v2/workspaces/ws-1", respond(http.StatusOK, map[string]any{"id": "ws-1", "name": "acme"}))
	ts.on("GET", "/v2/projects", respond(http.StatusOK, []any{map[string]any{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}}))
	for _, path := range []string{"/v2/networks", "/v2/dns", "/v2/proxies", "/v2/storages", "/v2/crons"} {
		ts.on("GET", path, respond(http.StatusOK, []any{}))
	}
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, serverlets))
}

func TestDeployRefreshesTheConfigInItsOwnFormat(t *testing.T) {
	for name, content := range map[string]string{
		"futrou.json": `{"workspace":"acme","project":"demo","region":"europe","serverlets":[{"name":"web","image":"nginx"}]}`,
		"futrou.yaml": "workspace: acme\nproject: demo\nregion: europe\nserverlets:\n  - name: web\n    image: nginx\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDirectory(t, dir)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			ts := newTestServer(t)
			created := false
			projectMocks(ts, nil)
			ts.on("GET", "/v2/regions", respond(http.StatusOK, []any{map[string]any{"id": "reg-eu", "name": "europe"}}))
			ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
				created = true
				// From now on the serverlet exists, so the refresh pulls it.
				projectMocks(ts, []any{map[string]any{"id": "sl-1", "name": "web", "image": "nginx", "regionId": "reg-eu"}})
				writeJSON(w, map[string]any{"id": "sl-1"})
			})

			_, err := runArgs(t, ts, "deploy", "--yes")
			assertNoError(t, err)
			if !created {
				t.Fatal("the serverlet should have been created")
			}
			data, _ := os.ReadFile(filepath.Join(dir, name))
			text := string(data)
			if name == "futrou.json" && !strings.HasPrefix(text, "{") {
				t.Fatalf("a .json config must stay JSON: %s", text)
			}
			if name == "futrou.yaml" && strings.HasPrefix(text, "{") {
				t.Fatalf("a .yaml config must stay YAML: %s", text)
			}
			for _, want := range []string{"locks", "serverlets.web", "region", "europe"} {
				if !strings.Contains(text, want) {
					t.Errorf("the refreshed config should contain %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestDeployDoesNotRewriteJavaScriptConfigs(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	source := `export default { workspace: "acme", project: "demo", storages: [{ name: "data" }] };`
	if err := os.WriteFile(filepath.Join(dir, "futrou.mjs"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	projectMocks(ts, nil)
	ts.on("POST", "/v2/storages", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{"id": "st-1"}) })
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	if data, _ := os.ReadFile(filepath.Join(dir, "futrou.mjs")); string(data) != source {
		t.Fatalf("a JavaScript config is code and must not be rewritten: %s", data)
	}
}

func TestDeployShowsDNSZoneAndRecordChanges(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	config := `{"dns":[{"domain":"example.com","name":"example.com","ttl":0,"priority":0,"records":[` +
		`{"name":"www","type":"A","value":"192.0.2.2","ttl":300,"priority":0},{"name":"api","type":"A","value":"192.0.2.3","ttl":300,"priority":0}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns", respond(http.StatusOK, []any{map[string]any{"id": "dz-1", "domain": "example.com", "name": "example.com", "ttl": 0, "priority": 0}}))
	ts.on("GET", "/v2/dns/dz-1/records", respond(http.StatusOK, []any{
		map[string]any{"id": "r-1", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0},
		map[string]any{"id": "r-2", "name": "old", "type": "A", "value": "192.0.2.9", "ttl": 300, "priority": 0},
	}))

	out, err := runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	assertContains(t, out, "dns zone")
	assertContains(t, out, "dns record")
	assertContains(t, out, `value: "192.0.2.1" → "192.0.2.2"`)
}

func TestDeployCreatesNeededServerletsFirst(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	config := "serverlets:\n  - name: web\n    image: nginx\n    needs: [db:healthy]\n  - name: db\n    image: postgres\n    healthcheck: pg_isready\n"
	if err := os.WriteFile(filepath.Join(dir, "futrou.yaml"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	var created []string
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
	ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeBody(r, &body)
		created = append(created, body["name"].(string))
		writeJSON(w, map[string]any{"id": "sl-" + body["name"].(string)})
	})
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	if strings.Join(created, ",") != "db,web" {
		t.Fatalf("created = %v, want db before web", created)
	}
}

func TestDeployRejectsInvalidNeedsBeforeCallingAPI(t *testing.T) {
	for name, config := range map[string]string{
		"undeclared": "serverlets:\n  - name: web\n    needs: [db]\n",
		"cycle":      "serverlets:\n  - name: a\n    needs: [b]\n  - name: b\n    needs: [a]\n",
		"status":     "serverlets:\n  - name: db\n  - name: web\n    needs: [\"db:ready\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDirectory(t, dir)
			if err := os.WriteFile(filepath.Join(dir, "futrou.yaml"), []byte(config), 0644); err != nil {
				t.Fatal(err)
			}
			ts := newTestServer(t) // any request fails the test
			_, err := runArgs(t, ts, "deploy", "--yes")
			assertError(t, err)
			assertContains(t, err.Error(), "serverlets")
		})
	}
}

const composeProject = `# no name: the project is not set
services:
  db:
    image: postgres:17
    x-futrou: {serverletPlanId: plan-db}
  web:
    image: nginx:latest
    restart: always
    depends_on:
      db: {condition: service_healthy}
    ports: ["80:8080"]
    x-futrou: {serverletPlanId: plan-web}
`

func TestDeployDockerComposeProject(t *testing.T) {
	for _, name := range []string{"docker-compose.yml", "compose.yaml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDirectory(t, dir)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(composeProject), 0644); err != nil {
				t.Fatal(err)
			}
			ts := newTestServer(t)
			var created []string
			bodies := map[string]map[string]any{}
			ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
			ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				decodeBody(r, &body)
				created = append(created, body["name"].(string))
				bodies[body["name"].(string)] = body
				writeJSON(w, map[string]any{"id": "sl-1"})
			})

			out, err := runArgs(t, ts, "deploy", "--yes")
			assertNoError(t, err)
			assertContains(t, out, "Using config: "+name)
			assertContains(t, out, "restart")
			if strings.Join(created, ",") != "db,web" {
				t.Fatalf("created = %v, want db before web", created)
			}
			web := bodies["web"]
			if web["image"] != "nginx:latest" || web["serverletPlanId"] != "plan-web" {
				t.Errorf("unexpected web payload: %#v", web)
			}
			if needs, _ := web["needs"].([]any); len(needs) != 1 || needs[0] != "db:healthy" {
				t.Errorf("needs = %#v", web["needs"])
			}
			if data, _ := os.ReadFile(filepath.Join(dir, name)); string(data) != composeProject {
				t.Fatalf("the Compose file must never be rewritten:\n%s", data)
			}
		})
	}
}

func TestDeployPrefersFutrouConfigOverDockerCompose(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeProject), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "futrou.yaml"), []byte("storages:\n  - name: data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages", respond(http.StatusOK, []any{}))
	out, err := runArgs(t, ts, "deploy", "--dry-run")
	assertNoError(t, err)
	assertContains(t, out, "Using config: futrou.yaml")
	assertContains(t, out, "storage")
}

func TestDeployInvalidDockerComposeFailsBeforeCallingAPI(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: x\n    depends_on: [web]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t) // any request fails the test
	_, err := runArgs(t, ts, "deploy", "--yes")
	assertError(t, err)
	assertContains(t, err.Error(), "docker-compose.yml")
}

func TestDownWithDockerComposeProject(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	compose := "name: demo\nx-futrou: {workspace: \"" + downWorkspaceID + "\"}\nservices:\n  web:\n    image: x\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		t.Fatal(err)
	}
	ts, deleted := downServer(t)
	_, err := runArgs(t, ts, "down", "--yes")
	assertNoError(t, err)
	if len(*deleted) != 7 {
		t.Fatalf("expected the project to be torn down, got %v", *deleted)
	}
}
