//go:build !windows

package commands

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

// Ctrl+C during an upload stops it and marks the version failed.
func TestStaticoDeployCancelledByInterruptMarksTheVersionFailed(t *testing.T) {
	ts, fake := staticoServer(t)
	fake.AddStatico("docs", map[string]string{"old.html": "o"})
	fake.HangUploads = make(chan struct{}, 1)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"site/index.html": "<h1>", "site/app.js": "js"})
	withWorkingDirectory(t, dir)

	go func() {
		<-fake.HangUploads // an upload is running
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()
	done := make(chan error, 1)
	go func() {
		_, err := runArgs(t, ts, "statico", "deploy", "--html-to-folders=false", "--include", "./site:./", "docs")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("error = %v, want a cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the deploy did not stop on Ctrl+C")
	}
	var failed, active bool
	for _, v := range fake.Statico("docs").Versions {
		if v.Status == "failed" {
			failed = true
		}
		if v.Name == fake.Statico("docs").Active {
			active = true
		}
	}
	if !failed {
		t.Errorf("no version was marked failed: %+v", fake.Statico("docs").Versions)
	}
	if got := fake.ActiveFiles("docs"); !active || got["old.html"] != "o" || len(got) != 1 {
		t.Errorf("the served version must stay as it was: %v", got)
	}
}
