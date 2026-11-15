package statico_test

import (
	"futrou-cli/src/statico/mock"
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func TestFindSiteByNameAndByID(t *testing.T) {
	fake, client := newFake(t)
	statico := fake.AddStatico("docs", nil)
	fake.AddStatico("blog", nil)
	byName, err := FindStatico(client, "docs", Scope{WorkspaceID: mock.WorkspaceID})
	if err != nil || byName.ID != statico.ID || byName.StorageID() != "statico-"+statico.ID {
		t.Fatalf("by name: %+v %v", byName, err)
	}
	byID, err := FindStatico(client, statico.ID, Scope{WorkspaceID: mock.WorkspaceID})
	if err != nil || byID.Name != "docs" {
		t.Fatalf("by id: %+v %v", byID, err)
	}
}

func TestFindSiteErrors(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	fake.AddStatico("dup", nil)
	fake.AddStatico("dup", nil)
	for ref, want := range map[string]string{"": "required", "nope": "not found", "dup": "2 statico websites are named"} {
		if _, err := FindStatico(client, ref, Scope{WorkspaceID: mock.WorkspaceID}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("FindStatico(%q) err = %v, want %q", ref, err, want)
		}
	}
}

func TestFindSiteNarrowsByScope(t *testing.T) {
	fake, client := newFake(t)
	a := fake.AddStatico("dup", nil)
	b := fake.AddStatico("dup", nil)
	b.ProjectID = "proj-2"
	statico, err := FindStatico(client, "dup", Scope{WorkspaceID: mock.WorkspaceID, ProjectID: "proj-2"})
	if err != nil || statico.ID != b.ID {
		t.Fatalf("statico = %+v err = %v (a=%s b=%s)", statico, err, a.ID, b.ID)
	}
}

func TestVersionsAndGetVersion(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"a": "a"})
	statico := staticoOf(t, client, "docs")
	versions, err := ListStaticoVersions(client, statico)
	if err != nil || len(versions) != 1 || versions[0].Status != "completed" || versions[0].Files != 1 {
		t.Fatalf("versions = %+v %v", versions, err)
	}
	if v, err := GetStaticoVersion(client, statico, "active"); err != nil || v.Name != "v1" {
		t.Fatalf("active = %+v %v", v, err)
	}
	if _, err := GetStaticoVersion(client, statico, "nope"); err == nil {
		t.Fatal("unknown version must fail")
	}
}

func TestEveryRequestCarriesTheCLIToken(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"a": "a"})
	statico := staticoOf(t, client, "docs")
	ListStaticoVersions(client, statico)
	Hashes(client, statico, StaticoVersion{Name: "v1"})
	if fake.Unauthorized != 0 {
		t.Fatalf("%d requests were not authorised with the CLI's token", fake.Unauthorized)
	}
}

func TestStaticoURLIsTheHTTPSAddressOfThePublicDomain(t *testing.T) {
	if got := (Statico{PublicDomain: "statico-abc.futrou.cloud"}).URL(); got != "https://statico-abc.futrou.cloud" {
		t.Fatalf("URL = %q", got)
	}
	if got := (Statico{}).URL(); got != "" {
		t.Fatalf("a website without a domain has no URL: %q", got)
	}
	fake, client := newFake(t)
	st := fake.AddStatico("docs", nil)
	found, err := FindStatico(client, "docs", Scope{WorkspaceID: mock.WorkspaceID})
	if err != nil || found.URL() != "https://statico-"+st.ID[len(st.ID)-12:]+".futrou.cloud" {
		t.Fatalf("found = %+v err = %v", found, err)
	}
}

func TestUploadResultCarriesTheURL(t *testing.T) {
	fake, client := newFake(t)
	st := fake.AddStatico("docs", map[string]string{"a.txt": "a"})
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	same, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || !same.Skipped || same.URL != "https://"+mock.PublicDomain(st) {
		t.Fatalf("skipped = %+v err = %v", same, err)
	}
	write(t, root, "a.txt", "b")
	changed, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || changed.URL != "https://"+mock.PublicDomain(st) {
		t.Fatalf("changed = %+v err = %v", changed, err)
	}
}
