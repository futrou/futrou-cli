package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployCreatesEveryDeclaredResourceWithYes(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{
  "serverlets": [{"name":"web","image":"nginx:latest","ram":128,"cpu":100,"minInstances":1,"maxInstances":1}],
  "volumes": [{"name":"data","sizeGb":10,"type":"ssd"}]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	var created []string
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{}))
	ts.on("GET", "/v2/volumes", respond(http.StatusOK, []any{}))
	ts.on("POST", "/v2/serverlets", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeBody(r, &body)
		if body["name"] != "web" || body["image"] != "nginx:latest" {
			t.Errorf("unexpected serverlet payload: %#v", body)
		}
		created = append(created, "serverlet")
		writeJSON(w, map[string]any{"id": "sl-1"})
	})
	ts.on("POST", "/v2/volumes", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		decodeBody(r, &body)
		if body["name"] != "data" || body["sizeGb"] != float64(10) {
			t.Errorf("unexpected volume payload: %#v", body)
		}
		created = append(created, "volume")
		writeJSON(w, map[string]any{"id": "vol-1"})
	})

	out, err := runArgs(t, ts, "deploy", "--yes")
	assertNoError(t, err)
	assertContains(t, out, "Changes:")
	assertContains(t, out, "Applied 2 change(s).")
	if strings.Join(created, ",") != "serverlet,volume" {
		t.Fatalf("expected both creates, got %#v", created)
	}
}

func TestDeployReportsUpToDateWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	withWorkingDirectory(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "futrou.json"), []byte(`{"serverlets":[{"name":"web","image":"nginx:latest","ram":128,"cpu":100,"minInstances":1,"maxInstances":1}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t)
	ts.on("GET", "/v2/serverlets", respond(http.StatusOK, []any{
		map[string]any{"id": "sl-1", "name": "web", "image": "nginx:latest", "ram": 128, "cpu": 100, "minInstances": 1, "maxInstances": 1},
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
