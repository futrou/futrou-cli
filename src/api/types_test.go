package api

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAPIErrorMessage(t *testing.T) {
	for name, tc := range map[string]struct {
		err  *APIError
		want string
	}{
		"message only":     {&APIError{Message: "not found"}, "not found"},
		"empty":            {&APIError{}, ""},
		"one field error":  {&APIError{Message: "invalid", Errors: []FieldError{{Message: "name is required"}}}, "invalid: name is required"},
		"several errors":   {&APIError{Message: "invalid", Errors: []FieldError{{Message: "a"}, {Message: "b"}, {Message: "c"}}}, "invalid: a; b; c"},
		"empty error list": {&APIError{Message: "oops", Errors: []FieldError{}}, "oops"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPIErrorWorksWithErrorsAs(t *testing.T) {
	var err error = &APIError{Message: "forbidden", RequestId: "req-1"}
	wrapped := errors.Join(errors.New("deploying"), err)
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) || apiErr.RequestId != "req-1" {
		t.Fatalf("a wrapped APIError must stay reachable: %v", wrapped)
	}
}

func TestAPIErrorDecodesAnErrorResponse(t *testing.T) {
	body := `{"message":"Validation failed","requestId":"req-9","clientIp":"203.0.113.7","errors":[{"message":"must be a URL","code":"url","field":"image"},{"message":"too short","code":"min","field":"name"}]}`
	var apiErr APIError
	if err := json.Unmarshal([]byte(body), &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.RequestId != "req-9" || apiErr.ClientIp != "203.0.113.7" || len(apiErr.Errors) != 2 || apiErr.Errors[0].Field != "image" || apiErr.Errors[1].Code != "min" {
		t.Fatalf("decoded = %#v", apiErr)
	}
	if got := apiErr.Error(); got != "Validation failed: must be a URL; too short" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestLoginResponseDecodes(t *testing.T) {
	body := `{"apiToken":{"id":"tok-1","name":"cli","token":"secret","type":"user","createdAt":"2026-01-02T03:04:05Z"},"user":{"id":"u-1","email":"dev@example.com"}}`
	var resp LoginResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ApiToken.Id != "tok-1" || resp.User.Id != "u-1" {
		t.Fatalf("decoded = %#v", resp)
	}
	if !resp.ApiToken.CreatedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("createdAt = %v", resp.ApiToken.CreatedAt)
	}
}

// The CLI reads these models from the API and writes them into futrou.json,
// so the field names and enum values it depends on are pinned here.
func TestResourceModelsDecodeAPIResponses(t *testing.T) {
	var serverlet Serverlet
	err := json.Unmarshal([]byte(`{"id":"sl-1","name":"web","image":"nginx","regionId":"reg-eu","entrypoint":["node","server.js"],"minInstances":1,"maxInstances":3,"needs":["db:healthy"],"networks":["backend"],"status":"running","createdAt":"2026-01-01T00:00:00Z","mounts":{"/data":"st-1"}}`), &serverlet)
	if err != nil {
		t.Fatal(err)
	}
	if serverlet.Name != "web" || serverlet.RegionId != "reg-eu" || len(serverlet.Entrypoint) != 2 || serverlet.MaxInstances != 3 || serverlet.Needs[0] != "db:healthy" || serverlet.Networks[0] != "backend" || serverlet.Mounts["/data"] != "st-1" {
		t.Fatalf("serverlet = %#v", serverlet)
	}

	var proxy Proxy
	if err := json.Unmarshal([]byte(`{"id":"px-1","domain":"app.example.com","type":"http","strategy":"round-robin","port":8080,"verifyTls":true}`), &proxy); err != nil {
		t.Fatal(err)
	}
	if proxy.Type == nil || *proxy.Type != ProxyTypeHttp || proxy.Strategy == nil || *proxy.Strategy != ProxyStrategyRoundRobin || proxy.Port != 8080 || !proxy.VerifyTls {
		t.Fatalf("proxy = %#v", proxy)
	}

	var cron Cron
	if err := json.Unmarshal([]byte(`{"id":"cr-1","name":"nightly","type":"simple","schedule":"0 3 * * *","enabled":true}`), &cron); err != nil {
		t.Fatal(err)
	}
	if cron.Type == nil || *cron.Type != CronTypeSimple || cron.Schedule != "0 3 * * *" || !cron.Enabled {
		t.Fatalf("cron = %#v", cron)
	}

	var storage Storage
	if err := json.Unmarshal([]byte(`{"id":"st-1","name":"data","storagePlanId":"plan-1","public":true}`), &storage); err != nil {
		t.Fatal(err)
	}
	if storage.Name != "data" || storage.StoragePlanId != "plan-1" || !storage.Public {
		t.Fatalf("storage = %#v", storage)
	}

	var project Project
	if err := json.Unmarshal([]byte(`{"id":"p-1","name":"demo","workspaceId":"ws-1","workspace":{"id":"ws-1","name":"acme"}}`), &project); err != nil {
		t.Fatal(err)
	}
	if project.Workspace == nil || project.Workspace.Name != "acme" || project.WorkspaceId != "ws-1" {
		t.Fatalf("project = %#v", project)
	}
}

func TestModelsOmitEmptyFieldsWhenEncoded(t *testing.T) {
	data, err := json.Marshal(Serverlet{Name: "web", Image: "nginx"})
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	for _, unset := range []string{"id", "regionId", "entrypoint", "env", "mounts", "arch", "needs"} {
		if _, present := object[unset]; present {
			t.Errorf("an unset %s must not be sent to the API: %s", unset, data)
		}
	}
	if object["name"] != "web" || object["image"] != "nginx" {
		t.Fatalf("encoded = %s", data)
	}
}

func TestEnumValuesTheCLIDependsOn(t *testing.T) {
	for got, want := range map[string]string{
		string(ProxyTypeHttp):           "http",
		string(ProxyStrategyRoundRobin): "round-robin",
		string(CronTypeSimple):          "simple",
		string(CronTypeCode):            "code",
	} {
		if got != want {
			t.Errorf("enum value = %q, want %q", got, want)
		}
	}
}
