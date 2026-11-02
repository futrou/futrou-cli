package constants

import (
	"strings"
	"testing"
)

func TestLicenseIsEmbeddedWithTheRepoUrl(t *testing.T) {
	if !strings.HasPrefix(License, "MIT License") {
		t.Fatalf("unexpected license start: %.30q", License)
	}
	if !strings.Contains(License, RepoUrl) {
		t.Fatalf("license does not mention %s", RepoUrl)
	}
	if License != strings.TrimSpace(License) {
		t.Fatal("license has surrounding whitespace")
	}
}
