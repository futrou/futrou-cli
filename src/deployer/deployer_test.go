package deployer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
