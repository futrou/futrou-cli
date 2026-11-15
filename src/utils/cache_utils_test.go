package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDirIsFutrouCacheInTheConfigFolder(t *testing.T) {
	if got := CacheDir("/project"); got != filepath.Join("/project", ".futrou", "cache") {
		t.Fatalf("CacheDir = %q", got)
	}
}

func TestMakeCacheDirCreatesFutrouWithAGitignoreOnce(t *testing.T) {
	base := t.TempDir()
	if _, err := os.Stat(filepath.Join(base, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("nothing exists before it is needed")
	}
	dir, err := MakeCacheDir(base)
	if err != nil || dir != CacheDir(base) {
		t.Fatalf("dir = %q err = %v", dir, err)
	}
	gitignore := filepath.Join(base, ".futrou", ".gitignore")
	if b, err := os.ReadFile(gitignore); err != nil || string(b) != "*\n" {
		t.Fatalf(".gitignore = %q %v", b, err)
	}
	// the user's own .gitignore is not overwritten
	os.WriteFile(gitignore, []byte("custom\n"), 0o644)
	if _, err := MakeCacheDir(base); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gitignore); string(b) != "custom\n" {
		t.Fatalf(".gitignore = %q", b)
	}
	blocked := t.TempDir()
	os.WriteFile(filepath.Join(blocked, ".futrou"), []byte("x"), 0o644)
	if _, err := MakeCacheDir(blocked); err == nil {
		t.Fatal("a .futrou that is a file cannot hold a cache")
	}
}

func TestCommandDoneIsRememberedPerKeyAndFolder(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if CommandDone(a, "npm ci") {
		t.Fatal("nothing is done at first")
	}
	if _, err := os.Stat(filepath.Join(a, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("asking creates nothing")
	}
	MarkCommandDone(a, "npm ci")
	MarkCommandDone(a, "yarn")
	if !CommandDone(a, "npm ci") || !CommandDone(a, "yarn") || CommandDone(a, "pnpm i") {
		t.Fatal("each key is remembered on its own")
	}
	if CommandDone(b, "npm ci") {
		t.Fatal("another folder has its own record")
	}
	if _, err := os.Stat(filepath.Join(a, ".futrou", "cache", "done-commands.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCommandDoneSurvivesADamagedFileAndAnUnwritableFolder(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(CacheDir(base), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(CacheDir(base), "done-commands.json"), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if CommandDone(base, "a") {
		t.Fatal("a damaged file means nothing was done")
	}
	MarkCommandDone(base, "a")
	if !CommandDone(base, "a") {
		t.Fatal("a damaged file is replaced")
	}
	blocked := t.TempDir()
	os.WriteFile(filepath.Join(blocked, ".futrou"), []byte("x"), 0o644)
	MarkCommandDone(blocked, "b") // must not panic or fail
	if CommandDone(blocked, "b") {
		t.Fatal("it could not be written, so it is not remembered")
	}
}

func TestWorkFolders(t *testing.T) {
	base := filepath.Join("some", "project")
	for name, tc := range map[string]struct{ got, want string }{
		"futrou":  {BuildDir(base), filepath.Join(base, ".futrou")},
		"cache":   {CacheDir(base), filepath.Join(base, ".futrou", "cache")},
		"statico": {StaticoDir(base), filepath.Join(base, ".futrou", "statico")},
		"tmp":     {TmpDir(base), filepath.Join(base, ".futrou", "tmp")},
		"debug":   {DebugDir(base), filepath.Join(base, ".futrou", "debug")},
	} {
		if tc.got != tc.want {
			t.Errorf("%s folder = %q, want %q", name, tc.got, tc.want)
		}
	}
}
