package deployer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"futrou-cli/src/api"
	"futrou-cli/src/config"
	"futrou-cli/src/services"
)

func TestBuildPlanServerletCreateUpdateDeleteAndNoop(t *testing.T) {
	cfg := &config.Config{Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx:latest", Ram: 128, Cpu: 100, MinInstances: 1, MaxInstances: 1}}}}
	for name, remote := range map[string][]map[string]interface{}{
		"create": {},
		"update": {{"id": "sl-1", "name": "web", "image": "nginx:old", "ram": 128, "cpu": 100, "minInstances": 1, "maxInstances": 1}},
		"noop":   {{"id": "sl-1", "name": "web", "image": "nginx:latest", "ram": 128, "cpu": 100, "minInstances": 1, "maxInstances": 1}},
	} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/v2/serverlets" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(remote); err != nil {
					t.Fatal(err)
				}
			}))
			defer ts.Close()
			plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "create":
				if len(plan.Actions) != 1 || plan.Actions[0].Type != Create {
					t.Fatalf("unexpected plan: %#v", plan.Actions)
				}
			case "update":
				if len(plan.Actions) != 1 || plan.Actions[0].Type != Update || plan.Actions[0].Changes["image"] != "nginx:latest" {
					t.Fatalf("unexpected plan: %#v", plan.Actions)
				}
			case "noop":
				if len(plan.Actions) != 0 {
					t.Fatalf("expected no actions, got %#v", plan.Actions)
				}
			}
		})
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"sl-1","name":"web"}]`))
	}))
	defer ts.Close()
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != Delete || plan.Actions[0].ID != "sl-1" {
		t.Fatalf("unexpected destroy plan: %#v", plan.Actions)
	}
}
