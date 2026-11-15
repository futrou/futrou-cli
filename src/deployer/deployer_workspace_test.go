package deployer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/config"
	"futrou-cli/src/services"
)

func TestPlanWorkspaceUsesTheLoginDefaultWhenTheConfigNamesNone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FUTROU_API_URL", "")
	t.Setenv("FUTROU_API_TOKEN", "")
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()

	stored := &cliconfig.CliConfig{}
	stored.SetDefaultWorkspace(ts.URL, "ws-default")
	if err := cliconfig.Save(stored); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".futrou")); err != nil {
		t.Fatal(err)
	}

	w := &planWorkspace{client: services.NewApiClientWithToken(ts.URL, "tok"), cfg: &config.Config{}}
	if id, err := w.get(); err != nil || id != "ws-default" {
		t.Fatalf("workspace = %q, %v; want the login default", id, err)
	}
}

func TestPlanWorkspaceResolvesANameOnce(t *testing.T) {
	var lists int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&lists, 1)
		json.NewEncoder(w).Encode([]map[string]string{{"id": "ws-1", "name": "acme"}})
	}))
	defer ts.Close()

	w := &planWorkspace{client: services.NewApiClientWithToken(ts.URL, "tok"), cfg: &config.Config{Workspace: "acme"}}
	for i := 0; i < 5; i++ {
		if id, err := w.get(); err != nil || id != "ws-1" {
			t.Fatalf("workspace = %q, %v", id, err)
		}
	}
	if lists != 1 {
		t.Errorf("the workspace was looked up %d times, want 1", lists)
	}
}

func TestPlanWorkspaceIsEmptyWithoutAnyWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_URL", "")
	t.Setenv("FUTROU_API_TOKEN", "")
	w := &planWorkspace{client: services.NewApiClientWithToken("http://127.0.0.1:1", "tok"), cfg: &config.Config{}}
	if id, err := w.get(); err != nil || id != "" {
		t.Fatalf("workspace = %q, %v; want none", id, err)
	}
}
