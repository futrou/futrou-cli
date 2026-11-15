package statico_test

import (
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func init() { RetryDelay = 0 }

// An archive that fails is sent file by file, with a warning; nothing is lost.
func TestUploadSendsFilesOneByOneWhenAnArchiveFails(t *testing.T) {
	fake, client := newFake(t)
	st := fake.AddStatico("docs", nil)
	fake.FailTimes["POST /v2/storages/statico-"+st.ID+"/archive"] = 1
	root := t.TempDir()
	manyFiles(t, root, "p", 2100) // two archives
	progress := &messages{}
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress}); err != nil {
		t.Fatal(err)
	}
	if got := fake.ActiveFiles("docs"); len(got) != 2100 {
		t.Fatalf("served %d files, want 2100", len(got))
	}
	if fake.Archives != 1 || fake.Count("PUT", "/v2/storages/") != 2000 && fake.Count("PUT", "/v2/storages/") != 100 {
		t.Logf("archives = %d, PUTs = %d", fake.Archives, fake.Count("PUT", "/v2/storages/"))
	}
	if !progress.has("warning: uploading") {
		t.Fatalf("no warning in %v", progress.texts)
	}
}

// A persistent failure is retried twice, then the version is marked failed.
func TestUploadGivesUpAfterTwoRetries(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	fake.FailWrites = 500
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	_, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "uploading a.txt") {
		t.Fatalf("err = %v", err)
	}
	if n := fake.Count("PUT", "/v2/storages/"); n != 3 {
		t.Fatalf("file requests = %d, want 1 try and 2 retries", n)
	}
	failed := false
	for _, v := range fake.Statico("docs").Versions {
		failed = failed || v.Status == "failed"
	}
	if !failed {
		t.Error("the version must be marked failed")
	}
}
