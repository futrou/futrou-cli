package deployer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"futrou-cli/src/api"
	"futrou-cli/src/config"
	"futrou-cli/src/services"
)

func TestBuildPlanServerletCreateUpdateDeleteAndNoop(t *testing.T) {
	cfg := &config.Config{Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx:latest"}, ServerletPlanId: "plan-1"}}}
	for name, remote := range map[string][]map[string]interface{}{
		"create": {},
		"update": {{"id": "sl-1", "name": "web", "image": "nginx:old", "serverletPlanId": "plan-1"}},
		"noop":   {{"id": "sl-1", "name": "web", "image": "nginx:latest", "serverletPlanId": "plan-1"}},
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

func TestBuildTeardownPlanDeletesDependentsFirstThenProject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/projects":
			json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "proj-1", "name": "demo"}})
		default:
			if r.URL.Query().Get("projectId") != "proj-1" {
				t.Errorf("%s must be scoped to the project", r.URL.Path)
			}
			json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "id" + r.URL.Path, "name": r.URL.Path, "domain": r.URL.Path}})
		}
	}))
	defer ts.Close()

	cfg := &config.Config{Workspace: "11111111-1111-1111-1111-111111111111", Project: "demo"}
	plan, err := BuildTeardownPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, action := range plan.Actions {
		if action.Type != Delete || action.ID == "" {
			t.Fatalf("unexpected action: %#v", action)
		}
		order = append(order, action.Resource)
	}
	want := []string{"cron", "proxy", "serverlet", "storage", "network", "dns zone", "project"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if last := plan.Actions[len(plan.Actions)-1]; last.Path != "/v2/projects" || last.ID != "proj-1" {
		t.Fatalf("the project must be deleted last: %#v", last)
	}
}

func TestBuildTeardownPlanRequiresProject(t *testing.T) {
	if _, err := BuildTeardownPlan(services.NewApiClientWithToken("http://unused.invalid", "token"), &config.Config{}); err == nil {
		t.Fatal("expected an error without a project")
	}
}

type recordedCall struct{ method, path string }

func recordingServer(t *testing.T, failPath string, calls *[]recordedCall) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, recordedCall{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == failPath {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"message": "in use"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": "new-1"})
	}))
}

func TestApplyExecutesActionsInOrder(t *testing.T) {
	var calls []recordedCall
	ts := recordingServer(t, "", &calls)
	defer ts.Close()

	plan := &Plan{Actions: []Action{
		{Type: Create, Resource: "network", Name: "backend", Payload: map[string]interface{}{"name": "backend"}, Path: "/v2/networks"},
		{Type: Update, Resource: "serverlet", Name: "web", ID: "sl-1", Changes: map[string]interface{}{"image": "nginx:2"}, Path: "/v2/serverlets"},
		{Type: Delete, Resource: "storage", Name: "data", ID: "st-1", Path: "/v2/storages"},
	}}
	if err := Apply(services.NewApiClientWithToken(ts.URL, "token"), plan); err != nil {
		t.Fatal(err)
	}
	want := []recordedCall{{"POST", "/v2/networks"}, {"PATCH", "/v2/serverlets/sl-1"}, {"DELETE", "/v2/storages/st-1"}}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestApplyStopsAtFirstFailure(t *testing.T) {
	var calls []recordedCall
	ts := recordingServer(t, "/v2/storages/st-1", &calls)
	defer ts.Close()

	plan := &Plan{Actions: []Action{
		{Type: Delete, Resource: "storage", Name: "data", ID: "st-1", Path: "/v2/storages"},
		{Type: Delete, Resource: "project", Name: "demo", ID: "proj-1", Path: "/v2/projects"},
	}}
	err := Apply(services.NewApiClientWithToken(ts.URL, "token"), plan)
	if err == nil || !strings.Contains(err.Error(), `delete storage "data"`) {
		t.Fatalf("expected a descriptive error, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("the project must not be deleted after a failure: %v", calls)
	}
}

func TestApplyResolvesMissingIDsByName(t *testing.T) {
	var calls []recordedCall
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, recordedCall{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode([]map[string]string{{"id": "net-9", "name": "backend"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer ts.Close()

	plan := &Plan{Actions: []Action{{Type: Delete, Resource: "network", Name: "backend", Path: "/v2/networks"}}}
	if err := Apply(services.NewApiClientWithToken(ts.URL, "token"), plan); err != nil {
		t.Fatal(err)
	}
	if last := calls[len(calls)-1]; last != (recordedCall{"DELETE", "/v2/networks/net-9"}) {
		t.Fatalf("calls = %v", calls)
	}

	missing := &Plan{Actions: []Action{{Type: Delete, Resource: "network", Name: "ghost", Path: "/v2/networks"}}}
	if err := Apply(services.NewApiClientWithToken(ts.URL, "token"), missing); err == nil {
		t.Fatal("expected an error for an unknown resource")
	}
}

func TestBuildPlanCreatesNetworksBeforeServerlets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer ts.Close()

	internal := true
	cfg := &config.Config{
		Networks:   []config.NetworkConfig{{Name: "backend", Ipv4: "10.0.0.0/24", Internal: &internal}},
		Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx", Networks: []string{"backend"}}}},
	}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Resource != "network" || plan.Actions[1].Resource != "serverlet" {
		t.Fatalf("networks must be created first: %#v", plan.Actions)
	}
	if plan.Actions[0].Payload["internal"] != true || plan.Actions[0].Payload["ipv4"] != "10.0.0.0/24" {
		t.Fatalf("unexpected network payload: %#v", plan.Actions[0].Payload)
	}
	nets, _ := plan.Actions[1].Payload["networks"].([]interface{})
	if len(nets) != 1 || nets[0] != "backend" {
		t.Fatalf("unexpected serverlet networks: %#v", plan.Actions[1].Payload["networks"])
	}

	destroy, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, true)
	if err != nil || len(destroy.Actions) != 0 {
		t.Fatalf("nothing exists remotely, so destroy has nothing to do: %#v, %v", destroy, err)
	}
}

func TestSortedActionsDoesNotChangeApplyOrder(t *testing.T) {
	plan := &Plan{Actions: []Action{{Resource: "serverlet", Name: "b"}, {Resource: "network", Name: "a"}}}
	sorted := SortedActions(plan)
	if sorted[0].Resource != "network" || plan.Actions[0].Resource != "serverlet" {
		t.Fatalf("sorted = %v, plan = %v", sorted, plan.Actions)
	}
}

func TestLockGroup(t *testing.T) {
	for resource, want := range map[string]string{
		"network": "networks", "serverlet": "serverlets", "dns zone": "dns", "proxy": "proxies",
		"storage": "storages", "cron": "crons", "project": "",
	} {
		if got := lockGroup(resource); got != want {
			t.Errorf("lockGroup(%q) = %q, want %q", resource, got, want)
		}
	}
}

// routedServer answers GET requests from a path-to-response table and fails
// the test for anything else.
func routedServer(t *testing.T, routes map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, ok := routes[r.URL.Path]
		if r.Method != "GET" || !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func TestBuildPlanDNSRecordsUpdateCreateAndDelete(t *testing.T) {
	zones := []map[string]interface{}{{"id": "dz-1", "domain": "example.com", "name": "example.com", "ttl": 0, "priority": 0}}
	for name, tc := range map[string]struct {
		remote  []map[string]interface{}
		desired []config.DNSRecordConfig
		want    map[ActionType]int
	}{
		"more desired than remote creates": {
			remote:  []map[string]interface{}{{"id": "r-1", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0}},
			desired: []config.DNSRecordConfig{{Name: "www", Type: "A", Value: "192.0.2.2", TTL: 300}, {Name: "api", Type: "A", Value: "192.0.2.3", TTL: 300}},
			want:    map[ActionType]int{Update: 1, Create: 1},
		},
		"fewer desired than remote deletes": {
			remote: []map[string]interface{}{
				{"id": "r-1", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0},
				{"id": "r-2", "name": "old", "type": "A", "value": "192.0.2.9", "ttl": 300, "priority": 0},
			},
			desired: []config.DNSRecordConfig{{Name: "www", Type: "A", Value: "192.0.2.1", TTL: 300}},
			want:    map[ActionType]int{Delete: 1},
		},
		"identical records need no changes": {
			remote:  []map[string]interface{}{{"id": "r-1", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0}},
			desired: []config.DNSRecordConfig{{Name: "www", Type: "A", Value: "192.0.2.1", TTL: 300}},
			want:    map[ActionType]int{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ts := routedServer(t, map[string]interface{}{"/v2/dns": zones, "/v2/dns/dz-1/records": tc.remote})
			defer ts.Close()
			cfg := &config.Config{DNS: []config.DNSConfig{{Domain: "example.com", Name: "example.com", Records: tc.desired}}}
			plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[ActionType]int{}
			for _, action := range plan.Actions {
				if action.Resource == "dns record" {
					counts[action.Type]++
				}
			}
			for kind, n := range tc.want {
				if counts[kind] != n {
					t.Errorf("%s actions = %d, want %d (%#v)", kind, counts[kind], n, plan.Actions)
				}
			}
			if len(counts) != len(tc.want) {
				t.Errorf("unexpected record actions: %v, want %v", counts, tc.want)
			}
		})
	}
}

func TestBuildPlanMatchesRenamedResourceThroughLocks(t *testing.T) {
	remoteLocks := config.LockHash("sl-1")
	ts := routedServer(t, map[string]interface{}{
		"/v2/workspaces":      []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":        []map[string]interface{}{{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}},
		"/v2/workspaces/ws-1": map[string]interface{}{"id": "ws-1", "name": "acme"},
		"/v2/networks":        []map[string]interface{}{},
		"/v2/serverlets":      []map[string]interface{}{{"id": "sl-1", "name": "web", "image": "nginx:1"}},
		"/v2/dns":             []map[string]interface{}{},
		"/v2/proxies":         []map[string]interface{}{},
		"/v2/storages":        []map[string]interface{}{},
		"/v2/crons":           []map[string]interface{}{},
	})
	defer ts.Close()

	cfg := &config.Config{
		Workspace: "acme", Project: "demo",
		Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web-renamed", Image: "nginx:1"}}},
		Locks:      map[string]string{"serverlets.web-renamed": remoteLocks},
	}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != Update || plan.Actions[0].RemoteName != "web" || plan.Actions[0].Changes["name"] != "web-renamed" {
		t.Fatalf("a locked resource must be renamed, not recreated: %#v", plan.Actions)
	}
}

func TestBuildPlanDestroyDeletesOnlyDeclaredResources(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{
		"/v2/serverlets": []map[string]interface{}{{"id": "sl-1", "name": "web"}, {"id": "sl-2", "name": "unrelated"}},
	})
	defer ts.Close()

	cfg := &config.Config{Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web"}}}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != Delete || plan.Actions[0].ID != "sl-1" {
		t.Fatalf("only the declared serverlet may be deleted: %#v", plan.Actions)
	}
}

func regionsServer(t *testing.T) *httptest.Server {
	return routedServer(t, map[string]interface{}{
		"/v2/regions": []map[string]interface{}{
			{"id": "reg-eu", "name": "europe", "displayName": "Europe (Frankfurt)"},
			{"id": "reg-us", "name": "us-east", "displayName": "US East"},
		},
		"/v2/networks": []map[string]interface{}{}, "/v2/serverlets": []map[string]interface{}{},
		"/v2/proxies": []map[string]interface{}{}, "/v2/storages": []map[string]interface{}{},
		"/v2/crons": []map[string]interface{}{}, "/v2/dns": []map[string]interface{}{},
	})
}

func TestResolveRegionID(t *testing.T) {
	ts := regionsServer(t)
	defer ts.Close()
	client := services.NewApiClientWithToken(ts.URL, "token")
	for region, want := range map[string]string{
		"reg-eu": "reg-eu", "europe": "reg-eu", "EUROPE": "reg-eu", "Europe (Frankfurt)": "reg-eu", "us-east": "reg-us",
	} {
		got, err := ResolveRegionID(client, region)
		if err != nil || got != want {
			t.Errorf("ResolveRegionID(%q) = %q, %v; want %q", region, got, err, want)
		}
	}
	_, err := ResolveRegionID(client, "mars")
	if err == nil || !strings.Contains(err.Error(), `"mars"`) || !strings.Contains(err.Error(), "europe, us-east") {
		t.Fatalf("expected an error listing the available regions, got %v", err)
	}
}

func TestBuildPlanAppliesProjectRegionToEveryRegionalResource(t *testing.T) {
	ts := regionsServer(t)
	defer ts.Close()

	cfg := &config.Config{
		Region:     "europe",
		Networks:   []config.NetworkConfig{{Name: "backend"}},
		Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}, {Serverlet: api.Serverlet{Name: "eu-only", Image: "nginx", RegionId: "reg-us"}}},
		Storages:   []config.StorageConfig{{Name: "data"}},
		Proxies:    []config.ProxyConfig{{Domain: "example.com"}},
		Crons:      []config.CronConfig{{Name: "job"}},
		DNS:        []config.DNSConfig{{Domain: "example.com", Name: "example.com"}},
	}
	// The DNS zone is looked up during planning; answer it from a shared table.
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	regions := map[string]interface{}{}
	for _, action := range plan.Actions {
		if action.Type == Create {
			regions[action.Resource+":"+action.Name] = action.Payload["regionId"]
		}
	}
	want := map[string]interface{}{
		"network:backend": "reg-eu", "serverlet:web": "reg-eu", "serverlet:eu-only": "reg-us",
		"storage:data": "reg-eu", "proxy:example.com": "reg-eu", "cron:job": "reg-eu", "dns zone:example.com": nil,
	}
	for key, region := range want {
		if got, ok := regions[key]; !ok || got != region {
			t.Errorf("%s regionId = %v (present %v), want %v", key, got, ok, region)
		}
	}
}

func TestBuildPlanWithoutRegionDoesNotListRegions(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{"/v2/serverlets": []map[string]interface{}{}}) // /v2/regions would fail the test
	defer ts.Close()
	cfg := &config.Config{Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := plan.Actions[0].Payload["regionId"]; present {
		t.Fatalf("no region configured, so no regionId: %#v", plan.Actions[0].Payload)
	}
}

func TestBuildPlanUnknownRegionFailsBeforePlanning(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{"/v2/regions": []map[string]interface{}{{"id": "reg-eu", "name": "europe"}}})
	defer ts.Close()
	cfg := &config.Config{Region: "mars", Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web"}}}}
	if _, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false); err == nil {
		t.Fatal("expected an unknown region error")
	}
}

func TestBuildPlanRegionAlreadyAppliedIsNoChange(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{
		"/v2/regions":    []map[string]interface{}{{"id": "reg-eu", "name": "europe"}},
		"/v2/serverlets": []map[string]interface{}{{"id": "sl-1", "name": "web", "image": "nginx", "regionId": "reg-eu"}},
	})
	defer ts.Close()
	cfg := &config.Config{Region: "europe", Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil || len(plan.Actions) != 0 {
		t.Fatalf("a serverlet already in the region needs no change: %#v, %v", plan.Actions, err)
	}
}

func TestBuildPlanFallsBackToUnscopedListing(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{
		"/v2/workspaces":      []map[string]interface{}{{"id": "ws-1", "name": "acme"}},
		"/v2/projects":        []map[string]interface{}{{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"}},
		"/v2/workspaces/ws-1": map[string]interface{}{"id": "ws-1", "name": "acme"},
		"/v2/networks":        []map[string]interface{}{},
		// The project-scoped call and the unscoped fallback hit the same path;
		// the endpoint ignores projectId, so the existing serverlet shows up.
		"/v2/serverlets": []map[string]interface{}{{"id": "sl-1", "name": "web", "image": "nginx"}, {"id": "sl-2", "name": "other", "image": "x"}},
		"/v2/dns":        []map[string]interface{}{}, "/v2/proxies": []map[string]interface{}{},
		"/v2/storages": []map[string]interface{}{}, "/v2/crons": []map[string]interface{}{},
	})
	defer ts.Close()
	cfg := &config.Config{Workspace: "acme", Project: "demo", Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if action.Type == Create && action.Resource == "serverlet" {
			t.Fatalf("an existing serverlet must not be recreated: %#v", plan.Actions)
		}
	}
}

func TestApplyResolvesDNSRecordIDsByNameAndType(t *testing.T) {
	var calls []recordedCall
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, recordedCall{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode([]map[string]string{{"id": "r-mx", "name": "@", "type": "MX"}, {"id": "r-a", "name": "@", "type": "A"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer ts.Close()
	plan := &Plan{Actions: []Action{{Type: Delete, Resource: "dns record", Name: "example.com / @ A", Path: "/v2/dns/dz-1/records"}}}
	if err := Apply(services.NewApiClientWithToken(ts.URL, "token"), plan); err != nil {
		t.Fatal(err)
	}
	if last := calls[len(calls)-1]; last != (recordedCall{"DELETE", "/v2/dns/dz-1/records/r-a"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func names(items []map[string]interface{}) string {
	var out []string
	for _, item := range items {
		out = append(out, item["name"].(string))
	}
	return strings.Join(out, ",")
}

func TestOrderByNeeds(t *testing.T) {
	items := []map[string]interface{}{
		{"name": "web", "needs": []interface{}{"db:healthy", "cache"}},
		{"name": "worker", "needs": []interface{}{"web"}},
		{"name": "db"},
		{"name": "cache", "needs": []interface{}{"db:started"}},
		{"name": "solo"},
	}
	if got := names(orderByNeeds(items, false)); got != "db,cache,web,worker,solo" {
		t.Errorf("create order = %s", got)
	}
	if got := names(orderByNeeds(items, true)); got != "solo,worker,web,cache,db" {
		t.Errorf("delete order = %s", got)
	}
	// Unknown and cyclic needs must not hang or drop serverlets.
	odd := []map[string]interface{}{
		{"name": "a", "needs": []interface{}{"b", "ghost", ":bad"}}, {"name": "b", "needs": []interface{}{"a"}},
	}
	if got := orderByNeeds(odd, false); len(got) != 2 {
		t.Errorf("no serverlet may be dropped, got %s", names(got))
	}
}

func TestBuildPlanCreatesServerletsAfterTheOnesTheyNeed(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{"/v2/serverlets": []map[string]interface{}{}})
	defer ts.Close()
	cfg := &config.Config{Serverlets: []config.ServerletConfig{
		{Serverlet: api.Serverlet{Name: "web", Image: "nginx", Needs: []string{"db:healthy"}}},
		{Serverlet: api.Serverlet{Name: "db", Image: "postgres"}},
	}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Name != "db" || plan.Actions[1].Name != "web" {
		t.Fatalf("db must be created before web: %#v", plan.Actions)
	}
	needs, _ := plan.Actions[1].Payload["needs"].([]interface{})
	if len(needs) != 1 || needs[0] != "db:healthy" {
		t.Fatalf("the needs must be sent as written: %#v", plan.Actions[1].Payload["needs"])
	}
}

func TestBuildPlanDestroyDeletesDependentsFirst(t *testing.T) {
	ts := routedServer(t, map[string]interface{}{"/v2/serverlets": []map[string]interface{}{
		{"id": "sl-db", "name": "db"}, {"id": "sl-web", "name": "web", "needs": []string{"db"}},
	}})
	defer ts.Close()
	cfg := &config.Config{Serverlets: []config.ServerletConfig{
		{Serverlet: api.Serverlet{Name: "db"}},
		{Serverlet: api.Serverlet{Name: "web", Needs: []string{"db"}}},
	}}
	plan, err := BuildPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Name != "web" || plan.Actions[1].Name != "db" {
		t.Fatalf("web must be deleted before db: %#v", plan.Actions)
	}
}

func TestBuildTeardownPlanDeletesServerletsThatOthersNeedLast(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/projects":
			json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "proj-1", "name": "demo"}})
		case "/v2/serverlets":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "sl-db", "name": "db"}, {"id": "sl-web", "name": "web", "needs": []string{"db:healthy"}}, {"id": "sl-job", "name": "job", "needs": []string{"web"}},
			})
		default:
			json.NewEncoder(w).Encode([]map[string]interface{}{})
		}
	}))
	defer ts.Close()
	cfg := &config.Config{Workspace: "11111111-1111-1111-1111-111111111111", Project: "demo"}
	plan, err := BuildTeardownPlan(services.NewApiClientWithToken(ts.URL, "token"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, action := range plan.Actions {
		if action.Resource == "serverlet" {
			order = append(order, action.Name)
		}
	}
	if strings.Join(order, ",") != "job,web,db" {
		t.Fatalf("serverlets must be deleted dependents first, got %v", order)
	}
}

// fakeAPI is a small stateful stand-in for the Futrou API: every collection
// supports list (filtered by projectId when given), create, patch and delete,
// so a plan can be applied and planned again like against the real service.
type fakeAPI struct {
	mu          sync.Mutex
	t           *testing.T
	collections map[string][]map[string]interface{}
	records     map[string][]map[string]interface{} // zone ID -> records
	calls       []string
	failOn      map[string]int // "METHOD /path" -> status
	nextID      int
	*httptest.Server
}

var fakeCollections = []string{"/v2/networks", "/v2/serverlets", "/v2/dns", "/v2/proxies", "/v2/storages", "/v2/crons", "/v2/regions", "/v2/workspaces", "/v2/projects"}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, collections: map[string][]map[string]interface{}{}, records: map[string][]map[string]interface{}{}, failOn: map[string]int{}}
	for _, path := range fakeCollections {
		f.collections[path] = []map[string]interface{}{}
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeAPI) client() *services.ApiClient {
	return services.NewApiClientWithToken(f.URL, "token")
}

// seed stores an item, assigning an ID when it has none.
func (f *fakeAPI) seed(path string, item map[string]interface{}) map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if item["id"] == nil {
		f.nextID++
		item["id"] = fmt.Sprintf("id-%d", f.nextID)
	}
	f.collections[path] = append(f.collections[path], item)
	return item
}

func (f *fakeAPI) seedRecord(zoneID string, item map[string]interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if item["id"] == nil {
		f.nextID++
		item["id"] = fmt.Sprintf("rec-%d", f.nextID)
	}
	f.records[zoneID] = append(f.records[zoneID], item)
}

func (f *fakeAPI) fail(method, path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failOn[method+" "+path] = status
}

func (f *fakeAPI) called(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

func (f *fakeAPI) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "GET ") {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeAPI) reply(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if status, ok := f.failOn[r.Method+" "+r.URL.Path]; ok {
		f.reply(w, status, map[string]string{"message": "forced failure"})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/") // v2, <collection>[, id[, records[, id]]]
	if len(parts) < 2 {
		f.reply(w, 404, map[string]string{"message": "not found"})
		return
	}
	base := "/" + parts[0] + "/" + parts[1]
	if _, ok := f.collections[base]; !ok {
		f.reply(w, 404, map[string]string{"message": "not found"})
		return
	}
	if base == "/v2/dns" && len(parts) >= 4 && parts[3] == "records" {
		f.serveRecords(w, r, parts)
		return
	}
	switch {
	case len(parts) == 2 && r.Method == "GET":
		items := []map[string]interface{}{}
		for _, item := range f.collections[base] {
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
			items = append(items, item)
		}
		f.reply(w, 200, items)
	case len(parts) == 2 && r.Method == "POST":
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		f.nextID++
		body["id"] = fmt.Sprintf("id-%d", f.nextID)
		f.collections[base] = append(f.collections[base], body)
		f.reply(w, 201, body)
	case len(parts) == 3 && r.Method == "GET":
		for _, item := range f.collections[base] {
			if item["id"] == parts[2] {
				f.reply(w, 200, item)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "not found"})
	case len(parts) == 3 && r.Method == "PATCH":
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		for _, item := range f.collections[base] {
			if item["id"] == parts[2] {
				for k, v := range body {
					item[k] = v
				}
				f.reply(w, 200, item)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "not found"})
	case len(parts) == 3 && r.Method == "DELETE":
		items := f.collections[base]
		for i, item := range items {
			if item["id"] == parts[2] {
				f.collections[base] = append(items[:i:i], items[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "not found"})
	default:
		f.reply(w, 405, map[string]string{"message": "method not allowed"})
	}
}

func (f *fakeAPI) serveRecords(w http.ResponseWriter, r *http.Request, parts []string) {
	zoneID := parts[2]
	switch {
	case len(parts) == 4 && r.Method == "GET":
		out := f.records[zoneID]
		if out == nil {
			out = []map[string]interface{}{}
		}
		f.reply(w, 200, out)
	case len(parts) == 4 && r.Method == "POST":
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		f.nextID++
		body["id"] = fmt.Sprintf("rec-%d", f.nextID)
		f.records[zoneID] = append(f.records[zoneID], body)
		f.reply(w, 201, body)
	case len(parts) == 5 && r.Method == "PATCH":
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		for _, item := range f.records[zoneID] {
			if item["id"] == parts[4] {
				for k, v := range body {
					item[k] = v
				}
				f.reply(w, 200, item)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "not found"})
	case len(parts) == 5 && r.Method == "DELETE":
		items := f.records[zoneID]
		for i, item := range items {
			if item["id"] == parts[4] {
				f.records[zoneID] = append(items[:i:i], items[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "not found"})
	default:
		f.reply(w, 405, map[string]string{"message": "method not allowed"})
	}
}

func actionKinds(plan *Plan) []string {
	var out []string
	for _, a := range plan.Actions {
		out = append(out, string(a.Type)+" "+a.Resource+" "+a.Name)
	}
	return out
}

func projectFake(t *testing.T) *fakeAPI {
	f := newFakeAPI(t)
	f.seed("/v2/workspaces", map[string]interface{}{"id": "ws-1", "name": "acme"})
	f.seed("/v2/projects", map[string]interface{}{"id": "proj-1", "name": "demo", "workspaceId": "ws-1"})
	f.seed("/v2/regions", map[string]interface{}{"id": "reg-eu", "name": "europe"})
	return f
}

func fullConfig() *config.Config {
	return &config.Config{
		Workspace: "acme", Project: "demo", Region: "europe",
		Networks:   []config.NetworkConfig{{Name: "backend"}},
		Storages:   []config.StorageConfig{{Name: "data"}},
		Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx:1"}, ServerletPlanId: "plan-1", Env: map[string]string{"A": "1"}}},
		Proxies:    []config.ProxyConfig{{Domain: "app.example.com", Target: "web", Port: 8080}},
		Crons:      []config.CronConfig{{Name: "nightly"}},
		DNS: []config.DNSConfig{{Domain: "example.com", Name: "example.com", TTL: 300, Records: []config.DNSRecordConfig{
			{Name: "www", Type: "A", Value: "192.0.2.1", TTL: 300},
			{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 300, Priority: 10},
		}}},
	}
}

// The DNS zone has to exist before its records can be planned, so a first
// deploy of a config with DNS is two rounds: the zone, then its records.
func applyUntilStable(t *testing.T, f *fakeAPI, cfg *config.Config) int {
	t.Helper()
	rounds := 0
	for ; rounds < 4; rounds++ {
		plan, err := BuildPlan(f.client(), cfg, false)
		if err != nil {
			// The zone is created by the first round; records are planned after.
			if !strings.Contains(err.Error(), "DNS zone") {
				t.Fatal(err)
			}
		}
		if plan == nil || len(plan.Actions) == 0 {
			if err == nil {
				return rounds
			}
		}
		if plan != nil {
			if err := Apply(f.client(), plan); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Fatalf("plan did not converge after %d rounds", rounds)
	return rounds
}

func TestBuildPlanCreatesEveryResourceTypeInOrder(t *testing.T) {
	f := projectFake(t)
	cfg := fullConfig()
	cfg.DNS = nil // zone records need the zone first; covered separately
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create network backend", "create serverlet web", "create proxy app.example.com", "create storage data", "create cron nightly"}
	got := actionKinds(plan)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for _, action := range plan.Actions {
		if action.Payload["regionId"] != "reg-eu" {
			t.Errorf("%s %s did not get the project region: %v", action.Resource, action.Name, action.Payload)
		}
		for _, forbidden := range []string{"id", "createdAt", "updatedAt", "state", "instances"} {
			if _, ok := action.Payload[forbidden]; ok {
				t.Errorf("%s payload must not contain %q", action.Resource, forbidden)
			}
		}
	}
}

func TestPlanApplyAndReplanIsStable(t *testing.T) {
	f := projectFake(t)
	cfg := fullConfig()
	cfg.DNS = nil
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v2/networks", "/v2/serverlets", "/v2/proxies", "/v2/storages", "/v2/crons"} {
		if len(f.collections[path]) != 1 {
			t.Fatalf("%s holds %v after apply", path, f.collections[path])
		}
	}
	again, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Actions) != 0 {
		t.Fatalf("a second plan after apply must be empty, got %v", actionKinds(again))
	}
}

func TestPlanApplyDNSZoneThenRecords(t *testing.T) {
	f := projectFake(t)
	cfg := fullConfig()
	cfg.Networks, cfg.Storages, cfg.Serverlets, cfg.Proxies, cfg.Crons = nil, nil, nil, nil, nil
	// A zone that does not exist yet is created without its records; the
	// records are planned by the next deploy, once the zone has an ID.
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil || len(plan.Actions) != 1 || plan.Actions[0].Resource != "dns zone" || plan.Actions[0].Type != Create {
		t.Fatalf("first plan = %v, %v", actionKinds(plan), err)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	plan, err = BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := actionKinds(plan); len(got) != 2 || !strings.HasPrefix(got[0], "create dns record") {
		t.Fatalf("records plan = %v", got)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	zoneID := f.collections["/v2/dns"][0]["id"].(string)
	if len(f.records[zoneID]) != 2 {
		t.Fatalf("records after apply = %v", f.records[zoneID])
	}
	again, err := BuildPlan(f.client(), cfg, false)
	if err != nil || len(again.Actions) != 0 {
		t.Fatalf("second plan must be empty: %v, %v", actionKinds(again), err)
	}
}

func TestBuildPlanUpdatesChangedResourceAndKeepsUnchangedOnes(t *testing.T) {
	f := projectFake(t)
	cfg := fullConfig()
	cfg.DNS = nil
	plan, _ := BuildPlan(f.client(), cfg, false)
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	cfg.Serverlets[0].Image = "nginx:2"
	cfg.Serverlets[0].Env = map[string]string{"A": "2"}
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("only the serverlet changed: %v", actionKinds(plan))
	}
	action := plan.Actions[0]
	if action.Type != Update || action.Changes["image"] != "nginx:2" || action.Previous["image"] != "nginx:1" {
		t.Fatalf("update = %#v", action)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	if got := f.collections["/v2/serverlets"][0]["image"]; got != "nginx:2" {
		t.Fatalf("remote image = %v", got)
	}
}

func TestBuildPlanDeletesRemoteResourcesRemovedFromTheConfig(t *testing.T) {
	f := projectFake(t)
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-keep", "name": "web", "image": "nginx:1", "regionId": "reg-eu", "serverletPlanId": "plan-1", "env": map[string]interface{}{"A": "1"}})
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-old", "name": "old", "image": "x"})
	f.seed("/v2/networks", map[string]interface{}{"id": "net-old", "name": "legacy"})
	cfg := &config.Config{Workspace: "acme", Project: "demo", Region: "europe",
		Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx:1"}, ServerletPlanId: "plan-1", Env: map[string]string{"A": "1"}}}}
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(actionKinds(plan), "|")
	if got != "delete network legacy|delete serverlet old" {
		t.Fatalf("plan = %s", got)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	if len(f.collections["/v2/serverlets"]) != 1 || len(f.collections["/v2/networks"]) != 0 {
		t.Fatalf("remote state after apply: %v %v", f.collections["/v2/serverlets"], f.collections["/v2/networks"])
	}
}

func TestBuildPlanWithoutAProjectDeletesRemoteResourcesThatAreNotDeclared(t *testing.T) {
	f := newFakeAPI(t)
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-1", "name": "other", "image": "x"})
	cfg := &config.Config{Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "nginx"}}}}
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(actionKinds(plan), "|"); got != "create serverlet web|delete serverlet other" || plan.Actions[1].ID != "sl-1" {
		t.Fatalf("plan = %v", got)
	}
}

func TestBuildPlanDestroyWithProjectDeletesOnlyDeclaredResources(t *testing.T) {
	f := projectFake(t)
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-1", "name": "web", "image": "x"})
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-2", "name": "unrelated", "image": "x"})
	cfg := &config.Config{Workspace: "acme", Project: "demo", Serverlets: []config.ServerletConfig{{Serverlet: api.Serverlet{Name: "web", Image: "x"}}}}
	plan, err := BuildPlan(f.client(), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := actionKinds(plan); len(got) != 1 || got[0] != "delete serverlet web" {
		t.Fatalf("plan = %v (%#v)", got, plan.Actions)
	}
}

func TestBuildPlanErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*fakeAPI)
		cfg   func() *config.Config
		want  string
	}{
		"unknown project": {
			setup: func(f *fakeAPI) {},
			cfg:   func() *config.Config { c := fullConfig(); c.Project = "ghost"; return c },
			want:  "pulling cloud configuration",
		},
		"unknown workspace": {
			setup: func(f *fakeAPI) {},
			cfg:   func() *config.Config { c := fullConfig(); c.Workspace = "nobody"; return c },
			want:  `workspace "nobody" not found`,
		},
		"unknown region": {
			setup: func(f *fakeAPI) {},
			cfg:   func() *config.Config { c := fullConfig(); c.Region = "mars"; return c },
			want:  `region "mars" not found`,
		},
		"regions unavailable": {
			setup: func(f *fakeAPI) { f.fail("GET", "/v2/regions", 500) },
			cfg:   fullConfig,
			want:  "listing regions",
		},
		"serverlet listing fails": {
			setup: func(f *fakeAPI) { f.fail("GET", "/v2/serverlets", 500) },
			cfg:   fullConfig,
			want:  "serverlets",
		},
		"network listing fails": {
			setup: func(f *fakeAPI) { f.fail("GET", "/v2/networks", 503) },
			cfg:   fullConfig,
			want:  "networks",
		},
		"no project and listing fails": {
			setup: func(f *fakeAPI) { f.fail("GET", "/v2/crons", 500) },
			cfg: func() *config.Config {
				return &config.Config{Crons: []config.CronConfig{{Name: "job"}}}
			},
			want: "listing crons",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := projectFake(t)
			tc.setup(f)
			cfg := tc.cfg()
			cfg.DNS = nil
			_, err := BuildPlan(f.client(), cfg, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestBuildPlanRenamesLockedResourceOfEveryType(t *testing.T) {
	f := projectFake(t)
	f.seed("/v2/networks", map[string]interface{}{"id": "n-1", "name": "net-old", "regionId": "reg-eu"})
	f.seed("/v2/storages", map[string]interface{}{"id": "s-1", "name": "disk-old", "regionId": "reg-eu"})
	f.seed("/v2/crons", map[string]interface{}{"id": "c-1", "name": "cron-old", "regionId": "reg-eu"})
	cfg := &config.Config{Workspace: "acme", Project: "demo", Region: "europe",
		Networks: []config.NetworkConfig{{Name: "net-new"}},
		Storages: []config.StorageConfig{{Name: "disk-new"}},
		Crons:    []config.CronConfig{{Name: "cron-new"}},
		Locks: map[string]string{
			"networks.net-new":  config.LockHash("n-1"),
			"storages.disk-new": config.LockHash("s-1"),
			"crons.cron-new":    config.LockHash("c-1"),
		},
	}
	// The cloud locks come from the pull; they are keyed by the old names.
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if action.Type != Update || action.Changes["name"] == nil {
			t.Fatalf("expected renames only, got %v", actionKinds(plan))
		}
	}
	if len(plan.Actions) != 3 {
		t.Fatalf("plan = %v", actionKinds(plan))
	}
}

func TestBuildPlanLockedProxyAndDNSGroupsAreMatched(t *testing.T) {
	f := projectFake(t)
	f.seed("/v2/proxies", map[string]interface{}{"id": "p-1", "domain": "old.example.com", "regionId": "reg-eu"})
	cfg := &config.Config{Workspace: "acme", Project: "demo", Region: "europe",
		Proxies: []config.ProxyConfig{{Domain: "new.example.com"}},
		Locks:   map[string]string{"proxies.new.example.com": config.LockHash("p-1")},
	}
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != Update || plan.Actions[0].RemoteName != "old.example.com" {
		t.Fatalf("a locked proxy is renamed instead of recreated: %v", actionKinds(plan))
	}
}

func TestFindLockedRemote(t *testing.T) {
	remote := []map[string]interface{}{{"id": "a", "name": "one"}, {"id": "b", "name": "two"}}
	hash := config.LockHash("b")
	cloud := map[string]string{"serverlets.two": hash, "networks.two": hash}
	local := map[string]string{"serverlets.renamed": hash}
	if got := findLockedRemote(remote, map[string]bool{}, local, cloud, "serverlets"); got == nil || got["id"] != "b" {
		t.Fatalf("match = %v", got)
	}
	if got := findLockedRemote(remote, map[string]bool{}, local, cloud, ""); got != nil {
		t.Fatalf("no group, no match: %v", got)
	}
	if got := findLockedRemote(remote, map[string]bool{}, local, cloud, "networks"); got != nil {
		t.Fatalf("the network group has no local lock: %v", got)
	}
	if got := findLockedRemote(remote, map[string]bool{"btwo": true}, local, cloud, "serverlets"); got != nil {
		t.Fatalf("an already used remote must not match twice: %v", got)
	}
	if got := findLockedRemote(remote, map[string]bool{}, map[string]string{"serverlets.x": "other"}, cloud, "serverlets"); got != nil {
		t.Fatalf("different fingerprints must not match: %v", got)
	}
}

func TestMatchingResources(t *testing.T) {
	remote := []map[string]interface{}{{"id": "1", "name": "a"}, {"id": "2", "name": "b"}, {"id": "3", "name": "c"}}
	desired := []map[string]interface{}{{"name": "a"}, {"name": "c"}, {"name": "zzz"}}
	got := matchingResources(remote, desired, "name")
	if len(got) != 2 || got[0]["id"] != "1" || got[1]["id"] != "3" {
		t.Fatalf("matching = %v", got)
	}
	if got := matchingResources(nil, desired, "name"); got == nil || len(got) != 0 {
		t.Fatalf("empty remote must give an empty, non-nil list: %#v", got)
	}
}

func TestFindRemote(t *testing.T) {
	remote := []map[string]interface{}{{"id": "1", "name": "a"}, {"id": "2", "name": "b", "domain": "b.example.com"}}
	if got := findRemote(remote, map[string]interface{}{"id": "2", "name": "renamed"}, "name"); got["id"] != "2" {
		t.Fatalf("by id: %v", got)
	}
	if got := findRemote(remote, map[string]interface{}{"name": "a"}, "name"); got["id"] != "1" {
		t.Fatalf("by name: %v", got)
	}
	if got := findRemote(remote, map[string]interface{}{"name": "none"}, "name"); got != nil {
		t.Fatalf("no match: %v", got)
	}
	if got := findRemote(remote, map[string]interface{}{"domain": "b.example.com"}, "domain"); got["id"] != "2" {
		t.Fatalf("by domain: %v", got)
	}
	if got := findRemote(remote, map[string]interface{}{"domain": "a"}, "domain"); got["id"] != "1" {
		t.Fatalf("a zone may carry its domain in the name field: %v", got)
	}
	if got := findRemote(remote, map[string]interface{}{}, "name"); got != nil {
		t.Fatalf("nameless desired item must not match: %v", got)
	}
}

func TestChangedFieldsNormalizesNumbers(t *testing.T) {
	changes := changedFields(
		map[string]interface{}{"port": 8080, "image": "a", "same": float64(3), "list": []interface{}{"x"}},
		map[string]interface{}{"port": float64(8080), "image": "b", "same": 3, "list": []interface{}{"x"}},
	)
	if len(changes) != 1 || changes["image"] != "a" {
		t.Fatalf("changes = %v", changes)
	}
	if got := changedFields(map[string]interface{}{"x": 1.5}, map[string]interface{}{"x": 1.5}); len(got) != 0 {
		t.Fatalf("equal fractions are no change: %v", got)
	}
	if got := changedFields(map[string]interface{}{"x": "new"}, map[string]interface{}{}); got["x"] != "new" {
		t.Fatalf("a field missing remotely is a change: %v", got)
	}
}

func TestWritablePayloadDropsReadOnlyFields(t *testing.T) {
	got := writablePayload(map[string]interface{}{"id": "1", "createdAt": "x", "updatedAt": "y", "instances": 1, "state": "up", "records": []int{}, "name": "keep", "regionId": "r"})
	if len(got) != 2 || got["name"] != "keep" || got["regionId"] != "r" {
		t.Fatalf("payload = %v", got)
	}
}

func TestItemsAsMapsAndArrayMaps(t *testing.T) {
	if got, err := itemsAsMaps([]config.NetworkConfig{{Name: "a"}}); err != nil || len(got) != 1 || got[0]["name"] != "a" {
		t.Fatalf("itemsAsMaps = %v, %v", got, err)
	}
	if got, err := itemsAsMaps(nil); err != nil || got != nil {
		t.Fatalf("nil items = %v, %v", got, err)
	}
	if _, err := itemsAsMaps(make(chan int)); err == nil {
		t.Fatal("unmarshalable items must fail")
	}
	if _, err := itemsAsMaps("not a list"); err == nil {
		t.Fatal("a non-list must fail")
	}
	if got, err := arrayMaps(nil); err != nil || got != nil {
		t.Fatalf("arrayMaps(nil) = %v, %v", got, err)
	}
	if got, err := arrayMaps([]config.DNSRecordConfig{{Name: "a"}}); err != nil || len(got) != 1 {
		t.Fatalf("arrayMaps = %v, %v", got, err)
	}
	if _, err := arrayMaps(make(chan int)); err == nil {
		t.Fatal("unmarshalable records must fail")
	}
	if _, err := arrayMaps("nope"); err == nil {
		t.Fatal("non-array records must fail")
	}
}

func TestResolveRegionIDErrors(t *testing.T) {
	f := newFakeAPI(t)
	f.fail("GET", "/v2/regions", 500)
	if _, err := ResolveRegionID(f.client(), "europe"); err == nil || !strings.Contains(err.Error(), "listing regions") {
		t.Fatalf("error = %v", err)
	}
	g := newFakeAPI(t)
	g.seed("/v2/regions", map[string]interface{}{"id": "r-1", "displayName": "Only Display"})
	if id, err := ResolveRegionID(g.client(), "only display"); err != nil || id != "r-1" {
		t.Fatalf("display names match case-insensitively: %q, %v", id, err)
	}
	if _, err := ResolveRegionID(g.client(), "x"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestBuildPlanServletNeedsOrderAcrossUpdates(t *testing.T) {
	f := newFakeAPI(t)
	cfg := &config.Config{Serverlets: []config.ServerletConfig{
		{Serverlet: api.Serverlet{Name: "web", Image: "w", Needs: []string{"api:healthy"}}},
		{Serverlet: api.Serverlet{Name: "api", Image: "a", Needs: []string{"db"}}},
		{Serverlet: api.Serverlet{Name: "db", Image: "d"}},
	}}
	plan, err := BuildPlan(f.client(), cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(actionKinds(plan), "|"); got != "create serverlet db|create serverlet api|create serverlet web" {
		t.Fatalf("plan = %s", got)
	}
}

func TestPlanClientIsNotCalledWhenNothingIsDeclared(t *testing.T) {
	f := newFakeAPI(t)
	plan, err := BuildPlan(f.client(), &config.Config{}, false)
	if err != nil || len(plan.Actions) != 0 {
		t.Fatalf("plan = %v, %v", plan, err)
	}
	if calls := f.calls; len(calls) != 0 {
		t.Fatalf("an empty config must not touch the API: %v", calls)
	}
	_ = services.NewApiClientWithToken
}

func TestApplyWithFakeAPIResolvesIDsAndFailsClearly(t *testing.T) {
	f := newFakeAPI(t)
	f.seed("/v2/serverlets", map[string]interface{}{"id": "sl-1", "name": "web", "image": "old"})
	plan := &Plan{Actions: []Action{
		{Type: Update, Resource: "serverlet", Name: "web-new", RemoteName: "web", Changes: map[string]interface{}{"name": "web-new", "image": "new"}, Path: "/v2/serverlets"},
	}}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	if got := f.collections["/v2/serverlets"][0]; got["name"] != "web-new" || got["image"] != "new" {
		t.Fatalf("remote = %v", got)
	}
	if got := f.mutations(); len(got) != 1 || got[0] != "PATCH /v2/serverlets/sl-1" {
		t.Fatalf("mutations = %v", got)
	}

	for name, tc := range map[string]struct {
		action Action
		setup  func(*fakeAPI)
		want   string
	}{
		"update of unknown resource":    {Action{Type: Update, Resource: "serverlet", Name: "ghost", Path: "/v2/serverlets"}, func(*fakeAPI) {}, `serverlet "ghost" not found`},
		"delete of unknown resource":    {Action{Type: Delete, Resource: "network", Name: "ghost", Path: "/v2/networks"}, func(*fakeAPI) {}, `network "ghost" not found`},
		"listing fails while resolving": {Action{Type: Delete, Resource: "network", Name: "n", Path: "/v2/networks"}, func(g *fakeAPI) { g.fail("GET", "/v2/networks", 500) }, "resolving network"},
		"create rejected":               {Action{Type: Create, Resource: "proxy", Name: "p", Payload: map[string]interface{}{"domain": "p"}, Path: "/v2/proxies"}, func(g *fakeAPI) { g.fail("POST", "/v2/proxies", 422) }, `create proxy "p"`},
		"patch rejected":                {Action{Type: Update, Resource: "cron", Name: "c", ID: "c-1", Changes: map[string]interface{}{"x": 1}, Path: "/v2/crons"}, func(g *fakeAPI) { g.fail("PATCH", "/v2/crons/c-1", 409) }, `update cron "c"`},
		"delete rejected":               {Action{Type: Delete, Resource: "storage", Name: "s", ID: "s-1", Path: "/v2/storages"}, func(g *fakeAPI) { g.fail("DELETE", "/v2/storages/s-1", 500) }, `delete storage "s"`},
	} {
		t.Run(name, func(t *testing.T) {
			g := newFakeAPI(t)
			tc.setup(g)
			err := Apply(g.client(), &Plan{Actions: []Action{tc.action}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	if err := Apply(f.client(), &Plan{}); err != nil {
		t.Fatalf("an empty plan applies cleanly: %v", err)
	}
}

func TestApplyDNSRecordsThroughTheRecordsEndpoint(t *testing.T) {
	f := newFakeAPI(t)
	f.seed("/v2/dns", map[string]interface{}{"id": "z-1", "domain": "example.com", "name": "example.com"})
	f.seedRecord("z-1", map[string]interface{}{"id": "r-old", "name": "old", "type": "A", "value": "192.0.2.9", "ttl": 300})
	f.seedRecord("z-1", map[string]interface{}{"id": "r-www", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300})
	path := "/v2/dns/z-1/records"
	plan := &Plan{Actions: []Action{
		{Type: Create, Resource: "dns record", Name: "example.com / api A", Payload: map[string]interface{}{"name": "api", "type": "A", "value": "192.0.2.2", "ttl": 300}, Path: path},
		{Type: Update, Resource: "dns record", Name: "example.com / www A", ID: "r-www", Changes: map[string]interface{}{"value": "192.0.2.5"}, Path: path},
		{Type: Delete, Resource: "dns record", Name: "example.com / old A", Path: path},
	}}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range f.records["z-1"] {
		names = append(names, fmt.Sprintf("%v=%v", r["name"], r["value"]))
	}
	if strings.Join(names, ",") != "www=192.0.2.5,api=192.0.2.2" {
		t.Fatalf("records = %v", names)
	}
}

func TestPlanDNSRecordsEdgeCases(t *testing.T) {
	zoneCfg := func(records ...config.DNSRecordConfig) *config.Config {
		return &config.Config{DNS: []config.DNSConfig{{Domain: "example.com", Name: "example.com", Records: records}}}
	}
	zone := map[string]interface{}{"id": "z-1", "domain": "example.com", "name": "example.com", "ttl": 0, "priority": 0}
	t.Run("duplicate name and type match by declaration order", func(t *testing.T) {
		f := newFakeAPI(t)
		f.seed("/v2/dns", zone)
		f.seedRecord("z-1", map[string]interface{}{"id": "a1", "name": "@", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0})
		f.seedRecord("z-1", map[string]interface{}{"id": "a2", "name": "@", "type": "A", "value": "192.0.2.2", "ttl": 300, "priority": 0})
		plan, err := BuildPlan(f.client(), zoneCfg(
			config.DNSRecordConfig{Name: "@", Type: "A", Value: "192.0.2.1", TTL: 300},
			config.DNSRecordConfig{Name: "@", Type: "A", Value: "192.0.2.7", TTL: 300},
		), false)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Actions) != 1 || plan.Actions[0].ID != "a2" || plan.Actions[0].Changes["value"] != "192.0.2.7" {
			t.Fatalf("plan = %#v", plan.Actions)
		}
	})
	t.Run("SOA value is managed by the provider", func(t *testing.T) {
		f := newFakeAPI(t)
		f.seed("/v2/dns", zone)
		f.seedRecord("z-1", map[string]interface{}{"id": "soa", "name": "@", "type": "SOA", "value": "ns1 admin 7", "ttl": 3600, "priority": 0})
		plan, err := BuildPlan(f.client(), zoneCfg(config.DNSRecordConfig{Name: "@", Type: "SOA", Value: "ns1 admin 1", TTL: 3600}), false)
		if err != nil || len(plan.Actions) != 0 {
			t.Fatalf("an SOA serial change is not a plan: %v, %v", actionKinds(plan), err)
		}
		plan, _ = BuildPlan(f.client(), zoneCfg(config.DNSRecordConfig{Name: "@", Type: "SOA", Value: "ns1 admin 1", TTL: 60}), false)
		if len(plan.Actions) != 1 || plan.Actions[0].Changes["ttl"] == nil || plan.Actions[0].Changes["value"] != nil {
			t.Fatalf("other SOA fields still update: %#v", plan.Actions)
		}
	})
	t.Run("locks pair records with their remote ids", func(t *testing.T) {
		f := newFakeAPI(t)
		f.seed("/v2/dns", zone)
		f.seedRecord("z-1", map[string]interface{}{"id": "rid", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0})
		cfg := zoneCfg(config.DNSRecordConfig{Name: "www", Type: "A", Value: "192.0.2.2", TTL: 300})
		cfg.Locks = map[string]string{"dns.example\\.com.records.www.A": config.LockHash("rid")}
		plan, err := BuildPlan(f.client(), cfg, false)
		if err != nil || len(plan.Actions) != 1 || plan.Actions[0].ID != "rid" || plan.Actions[0].Type != Update {
			t.Fatalf("plan = %#v, %v", plan.Actions, err)
		}
	})
	t.Run("an unmatched lock never adopts another record", func(t *testing.T) {
		f := newFakeAPI(t)
		f.seed("/v2/dns", zone)
		f.seedRecord("z-1", map[string]interface{}{"id": "rid", "name": "www", "type": "A", "value": "192.0.2.1", "ttl": 300, "priority": 0})
		cfg := zoneCfg(config.DNSRecordConfig{Name: "www", Type: "A", Value: "192.0.2.1", TTL: 300})
		cfg.Locks = map[string]string{"dns.example\\.com.records.www.A": config.LockHash("someone-else")}
		plan, _ := BuildPlan(f.client(), cfg, false)
		if got := strings.Join(actionKinds(plan), "|"); got != "create dns record example.com / www A|delete dns record example.com / www A" {
			t.Fatalf("plan = %s", got)
		}
	})
	t.Run("errors", func(t *testing.T) {
		f := newFakeAPI(t)
		f.seed("/v2/dns", zone)
		f.fail("GET", "/v2/dns/z-1/records", 500)
		if _, err := BuildPlan(f.client(), zoneCfg(config.DNSRecordConfig{Name: "a", Type: "A", Value: "1.1.1.1"}), false); err == nil || !strings.Contains(err.Error(), "listing DNS records") {
			t.Fatalf("error = %v", err)
		}
		g := newFakeAPI(t)
		g.seed("/v2/dns", zone)
		g.fail("GET", "/v2/dns", 500)
		if _, err := BuildPlan(g.client(), zoneCfg(), false); err == nil {
			t.Fatal("a failing zone listing must fail the plan")
		}
		if _, err := resolveDNSZoneID(f.client(), map[string]interface{}{"domain": "missing.example"}); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error = %v", err)
		}
		if _, err := resolveDNSZoneID(g.client(), map[string]interface{}{"domain": "example.com"}); err == nil || !strings.Contains(err.Error(), "listing DNS zones") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDNSRecordLockKeyEscapesAndNumbersDuplicates(t *testing.T) {
	seen := map[string]int{}
	rec := map[string]interface{}{"name": "a.b", "type": "TXT"}
	if got := dnsRecordLockKey("example.com", rec, seen); got != `dns.example\.com.records.a\.b.TXT` {
		t.Fatalf("key = %s", got)
	}
	if got := dnsRecordLockKey("example.com", rec, seen); got != `dns.example\.com.records.a\.b.TXT[1]` {
		t.Fatalf("second key = %s", got)
	}
	if got := dnsRecordLockKey("example.com", rec, seen); !strings.HasSuffix(got, "[2]") {
		t.Fatalf("third key = %s", got)
	}
}

func TestFindDNSRecord(t *testing.T) {
	remote := []map[string]interface{}{{"id": "1", "name": "www", "type": "A"}, {"id": "2", "name": "www", "type": "A"}, {"id": "3", "name": "mx", "type": "MX"}}
	desired := []map[string]interface{}{{"name": "www", "type": "A"}, {"name": "www", "type": "A"}, {"name": "mx", "type": "MX"}}
	if got := findDNSRecord(remote, desired, 0, map[string]interface{}{"id": "3", "name": "x", "type": "A"}); got["id"] != "3" {
		t.Fatalf("by id = %v", got)
	}
	if got := findDNSRecord(remote, desired, 1, desired[1]); got["id"] != "2" {
		t.Fatalf("duplicates match by index = %v", got)
	}
	if got := findDNSRecord(remote, desired, 2, desired[2]); got["id"] != "3" {
		t.Fatalf("unique by name and type = %v", got)
	}
	if got := findDNSRecord(remote, desired, 0, map[string]interface{}{"name": "none", "type": "A"}); got != nil {
		t.Fatalf("no match = %v", got)
	}
}

func TestBuildTeardownPlanWithFakeAPI(t *testing.T) {
	f := projectFake(t)
	f.seed("/v2/serverlets", map[string]interface{}{"id": "s-db", "name": "db", "projectId": "proj-1"})
	f.seed("/v2/serverlets", map[string]interface{}{"id": "s-api", "name": "api", "projectId": "proj-1", "needs": []interface{}{"db"}})
	f.seed("/v2/serverlets", map[string]interface{}{"id": "s-other", "name": "other", "projectId": "proj-2"})
	f.seed("/v2/networks", map[string]interface{}{"id": "n-1", "name": "net", "projectId": "proj-1"})
	cfg := &config.Config{Workspace: "acme", Project: "demo"}
	plan, err := BuildTeardownPlan(f.client(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(actionKinds(plan), "|"); got != "delete serverlet api|delete serverlet db|delete network net|delete project demo" {
		t.Fatalf("plan = %s", got)
	}
	if err := Apply(f.client(), plan); err != nil {
		t.Fatal(err)
	}
	if len(f.collections["/v2/projects"]) != 0 || len(f.collections["/v2/networks"]) != 0 {
		t.Fatal("the project and its resources must be gone")
	}
	if len(f.collections["/v2/serverlets"]) != 1 || f.collections["/v2/serverlets"][0]["id"] != "s-other" {
		t.Fatalf("resources of other projects stay: %v", f.collections["/v2/serverlets"])
	}

	g := projectFake(t)
	g.fail("GET", "/v2/storages", 500)
	if _, err := BuildTeardownPlan(g.client(), cfg); err == nil || !strings.Contains(err.Error(), "storages") {
		t.Fatalf("error = %v", err)
	}
	h := projectFake(t)
	if _, err := BuildTeardownPlan(h.client(), &config.Config{Workspace: "acme", Project: "ghost"}); err == nil {
		t.Fatal("an unknown project must fail")
	}
}
