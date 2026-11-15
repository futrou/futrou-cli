package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func call(t *testing.T, s *Server, method, target, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	if !s.Handle(rec, req) {
		t.Fatalf("%s %s is not a route of the fake", method, target)
	}
	return rec.Code, rec.Body.String()
}

func TestHandleOnlyServesStaticoAndItsStorageRoutes(t *testing.T) {
	s := New()
	for _, path := range []string{"/v2/serverlets", "/v2/storages/other-1", "/v2/auth/context"} {
		if s.Handle(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil)) {
			t.Errorf("%s must be left to the caller", path)
		}
	}
}

func TestHandleRejectsARequestWithoutTheCLIsToken(t *testing.T) {
	s := New()
	if code, _ := call(t, s, "GET", "/v2/staticos?workspaceId="+WorkspaceID, ""); code != 401 {
		t.Errorf("no token: status %d", code)
	}
	if code, _ := call(t, s, "GET", "/v2/staticos?workspaceId="+WorkspaceID, "wrong"); code != 401 {
		t.Errorf("wrong token: status %d", code)
	}
	if s.Unauthorized != 2 {
		t.Errorf("Unauthorized = %d, want 2", s.Unauthorized)
	}
}

// Like the real API, listing needs a workspace or a project.
func TestListingNeedsAWorkspaceOrAProject(t *testing.T) {
	s := New()
	s.AddStatico("docs", nil)
	if code, body := call(t, s, "GET", "/v2/staticos", Token); code != 400 || !strings.Contains(body, "workspaceId") {
		t.Errorf("no scope: %d %s", code, body)
	}
	code, body := call(t, s, "GET", "/v2/staticos?workspaceId="+WorkspaceID, Token)
	var list []map[string]interface{}
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list) != 1 || list[0]["name"] != "docs" {
		t.Errorf("workspace scope: %d %s", code, body)
	}
}

func TestAddStaticoServesItsFilesAndForcedFailures(t *testing.T) {
	s := New()
	s.AddStatico("docs", map[string]string{"index.html": "hello"})
	if got := s.ActiveFiles("docs")["index.html"]; got != "hello" {
		t.Errorf("served file = %q", got)
	}
	if s.Statico("docs") == nil || s.Statico("nope") != nil {
		t.Error("Statico finds a website by name")
	}
	s.Fail["GET /v2/staticos"] = http.StatusBadGateway
	if code, _ := call(t, s, "GET", "/v2/staticos?workspaceId="+WorkspaceID, Token); code != http.StatusBadGateway {
		t.Errorf("forced failure: status %d", code)
	}
	if s.Count("GET", "/v2/staticos") != 1 {
		t.Errorf("Count = %d", s.Count("GET", "/v2/staticos"))
	}
}
