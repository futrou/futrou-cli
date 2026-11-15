package statico_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func TestArchiveFormatFollowsTheExtension(t *testing.T) {
	for name, want := range map[string]string{"a.zip": "zip", "A.ZIP": "zip", "a.tar.gz": "tar.gz", "a.tgz": "tar.gz", "a": "tar.gz"} {
		if got := ArchiveFormat(name); got != want {
			t.Errorf("ArchiveFormat(%q) = %q, want %q", name, got, want)
		}
	}
	if got := ArchiveName(Statico{Name: "my site/x"}, "v3"); got != "my_site_x-v3.tar.gz" {
		t.Fatalf("ArchiveName = %q", got)
	}
}

func TestDownloadArchiveWritesTheVersionAsAZipOrIntoAFolder(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"a.txt": "a", "d/b.txt": "b"})
	st := staticoOf(t, client, "docs")
	target := filepath.Join(t.TempDir(), "sub", "site.zip")
	version, err := DownloadArchive(client, st, "", target)
	if err != nil || version != "v1" {
		t.Fatalf("version = %q err = %v", version, err)
	}
	zr, err := zip.OpenReader(target)
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	zr.Close()
	if len(zr.File) != 2 {
		t.Fatalf("zip = %v", zr.File)
	}
	folder := t.TempDir()
	if _, err := DownloadArchive(client, st, "active", folder); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "docs-v1.tar.gz")); err != nil {
		t.Fatalf("a folder gets the default name: %v", err)
	}
}

func TestDownloadArchiveErrorsLeaveNothingBehind(t *testing.T) {
	fake, client := newFake(t)
	st0 := fake.AddStatico("docs", map[string]string{"a": "a"})
	st := staticoOf(t, client, "docs")
	dir := t.TempDir()
	if _, err := DownloadArchive(client, st, "nope", filepath.Join(dir, "x.tar.gz")); err == nil {
		t.Fatal("an unknown version fails")
	}
	fake.Fail["GET /v2/storages/statico-"+st0.ID+"/archive"] = 500
	_, err := DownloadArchive(client, st, "", filepath.Join(dir, "x.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "downloading version v1") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a failed download leaves nothing: %v", entries)
	}
}

func TestActivateVersionAndVersionExists(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"a": "a"})
	st := staticoOf(t, client, "docs")
	if ok, err := VersionExists(client, st, "v1"); err != nil || !ok {
		t.Fatalf("v1 exists: %v %v", ok, err)
	}
	if ok, err := VersionExists(client, st, "nope"); err != nil || ok {
		t.Fatalf("nope does not exist: %v %v", ok, err)
	}
	if err := ActivateVersion(client, st, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := ActivateVersion(client, st, "nope"); err == nil {
		t.Fatal("an unknown version cannot be served")
	}
}
