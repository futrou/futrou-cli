package statico_test

import (
	"futrou-cli/src/utils"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func stageFiles(t *testing.T, root string, rules utils.GlobRules) []HashedFile {
	t.Helper()
	var out []HashedFile
	for _, f := range hashed(t, root, rules) {
		out = append(out, HashedFile{LocalFile: f})
	}
	return out
}

func TestStageCopiesIntoTheVersionFolderAndHashesWhatItCopied(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/b.txt", "bee")
	write(t, root, "c.txt", "see")
	staged, err := Stage(root, "my site/x", "v7", stageFiles(t, root, utils.GlobRules{{Source: "./a", Dest: "a"}, {Source: "./c.txt"}}))
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.Join(root, ".futrou", "statico", "my_site_x-")
	if !strings.HasPrefix(staged.Dir, wantPrefix) || filepath.Base(staged.Dir) != "v7" {
		t.Fatalf("dir = %s, want %s<hash>/v7", staged.Dir, wantPrefix)
	}
	if len(staged.Files) != 2 {
		t.Fatalf("files = %+v", staged.Files)
	}
	for _, f := range staged.Files {
		if !strings.HasPrefix(f.Abs, staged.Dir) || f.Hash == "" || f.Size != 3 {
			t.Errorf("staged %+v", f)
		}
		if b, err := os.ReadFile(f.Abs); err != nil || len(b) != 3 {
			t.Errorf("%s: %q %v", f.Abs, b, err)
		}
	}
	if staged.Files[0].Path != "a/b.txt" || staged.Files[1].Path != "c.txt" {
		t.Fatalf("paths = %v %v", staged.Files[0].Path, staged.Files[1].Path)
	}
	if b, err := os.ReadFile(filepath.Join(root, ".futrou", ".gitignore")); err != nil || string(b) != "*\n" {
		t.Fatalf(".gitignore = %q err = %v", b, err)
	}
	staged.Remove()
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal(".futrou must be gone")
	}
	staged.Remove() // idempotent
	var nobody *Staged
	nobody.Remove()
}

func TestStageHashIsOfTheCopyEvenIfTheSourceChangedAfterThePlan(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "one")
	files := stageFiles(t, root, nil)
	files[0].Hash = "the hash computed before the file changed"
	write(t, root, "a.txt", "two!")
	staged, err := Stage(root, "docs", "v1", files)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Remove()
	f := staged.Files[0]
	if f.Size != 4 || f.Hash == files[0].Hash {
		t.Fatalf("staged %+v: size and hash must be those of what was copied", f)
	}
}

func TestStageReplacesAStaleFolderOfTheSameVersionAndRefusesBadNames(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "new")
	write(t, root, ".futrou/statico/docs/v1/stale.txt", "stale")
	staged, err := Stage(root, "docs", "v1", stageFiles(t, root, utils.GlobRules{{Source: "./a.txt"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staged.Dir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("a stale folder of that version must be replaced")
	}
	for _, bad := range []string{"", ".", "..", "a/b", "../x", "a b"} {
		if _, err := Stage(root, "docs", bad, nil); err == nil {
			t.Errorf("Stage with the version %q must fail", bad)
		}
	}
}

func TestStageRefusesAPathThatLeavesTheStagingFolder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	_, err := Stage(root, "docs", "v1", []HashedFile{{LocalFile: LocalFile{Abs: filepath.Join(root, "a.txt"), Path: "../escape.txt"}}})
	if err == nil || !strings.Contains(err.Error(), "leaves the staging folder") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou", "statico", "docs", "escape.txt")); err == nil {
		t.Fatal("a file escaped")
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("a failed stage must clean up")
	}
}

func TestStageFailsOnAnUnreadableSource(t *testing.T) {
	root := t.TempDir()
	_, err := Stage(root, "docs", "v1", []HashedFile{{LocalFile: LocalFile{Abs: filepath.Join(root, "missing"), Path: "missing"}}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("a failed stage must clean up")
	}
}

func TestStageNeverLeavesTheStagingFolderWhateverTheWebsiteIsCalled(t *testing.T) {
	for _, name := range []string{"..", ".", "...", "", "a/../..", "../..", `..\..`} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "in.txt")
			if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			staged, err := Stage(root, name, "v1", []HashedFile{{LocalFile: LocalFile{Abs: src, Path: "in.txt"}}})
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(filepath.Join(root, ".futrou", "statico"), staged.Dir)
			parts := strings.Split(rel, string(filepath.Separator))
			if err != nil || len(parts) != 2 || parts[0] == ".." || parts[0] == "." || parts[1] != "v1" {
				t.Errorf("staged in %s (%q below the staging folder, %v): want <website>/<version>", staged.Dir, rel, err)
			}
			staged.Remove()
		})
	}
}
