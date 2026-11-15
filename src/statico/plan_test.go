package statico_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	. "futrou-cli/src/statico"

	"futrou-cli/src/utils"
)

func local(path string, size int64, hash string) HashedFile {
	return HashedFile{LocalFile: LocalFile{Path: path, Size: size}, Hash: hash}
}

func remote(path string, size int64, hash string) RemoteFile {
	return RemoteFile{Path: path, Size: size, Hash: hash}
}

func uploadPaths(p Plan) []string {
	out := []string{}
	for _, f := range p.Upload {
		out = append(out, f.Path)
	}
	return out
}

func TestMakePlanComparesFilesBySizeAndHash(t *testing.T) {
	plan := MakePlan(
		[]HashedFile{local("same", 1, "h1"), local("changed", 1, "h2"), local("resized", 5, "h3"), local("new", 2, "")},
		[]RemoteFile{remote("same", 1, "h1"), remote("changed", 1, "x"), remote("resized", 1, "h3"), remote("gone", 1, "g")},
	)
	if want := []string{"changed", "new", "resized"}; !reflect.DeepEqual(uploadPaths(plan), want) {
		t.Fatalf("upload = %v, want %v", uploadPaths(plan), want)
	}
	if !reflect.DeepEqual(plan.Delete, []string{"gone"}) || plan.Unchanged != 1 || plan.Deleted != 1 || len(plan.DeleteDirs) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestMakePlanAFileWithoutAHashIsNeverTheSame(t *testing.T) {
	plan := MakePlan([]HashedFile{local("a", 1, "")}, []RemoteFile{remote("a", 1, "h")})
	if len(plan.Upload) != 1 || plan.Unchanged != 0 {
		t.Fatalf("a file known to differ is uploaded: %+v", plan)
	}
}

func TestMakePlanSkipsAFolderThatIsTheSameOnBothSides(t *testing.T) {
	l := []HashedFile{local("assets/a.css", 1, "1"), local("assets/img/b.png", 2, "2"), local("index.html", 3, "3")}
	r := []RemoteFile{remote("assets/a.css", 1, "1"), remote("assets/img/b.png", 2, "2"), remote("index.html", 3, "x")}
	plan := MakePlan(l, r)
	if !reflect.DeepEqual(uploadPaths(plan), []string{"index.html"}) || plan.Unchanged != 2 || plan.UnchangedDirs != 1 {
		t.Fatalf("the unchanged assets folder is skipped whole: %+v", plan)
	}
}

func TestMakePlanRecursesIntoTheFolderThatChangedOnly(t *testing.T) {
	l := []HashedFile{local("a/x/1", 1, "1"), local("a/x/2", 1, "2"), local("a/y/3", 1, "3"), local("b/4", 1, "4")}
	r := []RemoteFile{remote("a/x/1", 1, "1"), remote("a/x/2", 1, "2"), remote("a/y/3", 1, "other"), remote("b/4", 1, "4")}
	plan := MakePlan(l, r)
	if !reflect.DeepEqual(uploadPaths(plan), []string{"a/y/3"}) || plan.Unchanged != 3 || plan.UnchangedDirs != 2 {
		t.Fatalf("a/x and b are the same: %+v", plan)
	}
}

func TestMakePlanEveryFileInANewFolderIsUploadedWithoutLookingAtTheRest(t *testing.T) {
	l := []HashedFile{local("new/a", 1, ""), local("new/deep/b", 1, ""), local("old", 1, "h")}
	plan := MakePlan(l, []RemoteFile{remote("old", 1, "h")})
	if !reflect.DeepEqual(uploadPaths(plan), []string{"new/a", "new/deep/b"}) || plan.Unchanged != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestMakePlanDeletesAGoneFolderAsOneEntry(t *testing.T) {
	r := []RemoteFile{remote("keep", 1, "k"), remote("old/a", 1, "1"), remote("old/deep/b", 1, "2"), remote("old/deep/c", 1, "3")}
	plan := MakePlan([]HashedFile{local("keep", 1, "k")}, r)
	if !reflect.DeepEqual(plan.DeleteDirs, []string{"old"}) || len(plan.Delete) != 0 || plan.Deleted != 3 || !plan.Empty() == true && false {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Empty() {
		t.Fatal("a folder to delete is a change")
	}
}

func TestMakePlanDeletesASubfolderThatWentAndKeepsTheSiblings(t *testing.T) {
	l := []HashedFile{local("a/keep", 1, "k"), local("a/new", 1, "")}
	r := []RemoteFile{remote("a/keep", 1, "k"), remote("a/gone/x", 1, "1"), remote("a/gone/y", 1, "2")}
	plan := MakePlan(l, r)
	if !reflect.DeepEqual(plan.DeleteDirs, []string{"a/gone"}) || !reflect.DeepEqual(uploadPaths(plan), []string{"a/new"}) || plan.Deleted != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestMakePlanAFileThatBecomesAFolderAndBack(t *testing.T) {
	plan := MakePlan([]HashedFile{local("p/inner", 1, "")}, []RemoteFile{remote("p", 1, "h")})
	if !reflect.DeepEqual(plan.Delete, []string{"p"}) || !reflect.DeepEqual(uploadPaths(plan), []string{"p/inner"}) {
		t.Fatalf("a file that becomes a folder: %+v", plan)
	}
	plan = MakePlan([]HashedFile{local("p", 1, "")}, []RemoteFile{remote("p/inner", 1, "h"), remote("p/other", 1, "h")})
	if !reflect.DeepEqual(plan.DeleteDirs, []string{"p"}) || !reflect.DeepEqual(uploadPaths(plan), []string{"p"}) || plan.Deleted != 2 {
		t.Fatalf("a folder that becomes a file: %+v", plan)
	}
}

func TestMakePlanEmptyAndIdentical(t *testing.T) {
	if plan := MakePlan(nil, nil); !plan.Empty() || plan.Unchanged != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	l := []HashedFile{local("a", 1, "1"), local("d/b", 2, "2")}
	r := []RemoteFile{remote("a", 1, "1"), remote("d/b", 2, "2")}
	plan := MakePlan(l, r)
	if !plan.Empty() || plan.Unchanged != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if everything := MakePlan(nil, r); !reflect.DeepEqual(everything.DeleteDirs, []string{"d"}) || !reflect.DeepEqual(everything.Delete, []string{"a"}) || everything.Deleted != 2 {
		t.Fatalf("nothing local deletes everything: %+v", everything)
	}
}

func TestMakePlanIsSortedAndTheSameForTheSameInputs(t *testing.T) {
	var l []HashedFile
	var r []RemoteFile
	for i := 30; i > 0; i-- {
		l = append(l, local(fmt.Sprintf("d%02d/f%02d", i%7, i), 1, ""))
		r = append(r, remote(fmt.Sprintf("z%02d/g%02d", i%5, i), 1, "h"))
	}
	first, second := MakePlan(l, r), MakePlan(l, r)
	if !reflect.DeepEqual(uploadPaths(first), uploadPaths(second)) || !reflect.DeepEqual(first.DeleteDirs, second.DeleteDirs) {
		t.Fatal("the plan must not depend on map order")
	}
	paths := uploadPaths(first)
	for i := 1; i < len(paths); i++ {
		if paths[i-1] > paths[i] {
			t.Fatalf("not sorted: %v", paths)
		}
	}
}

func TestMakePlanBytes(t *testing.T) {
	plan := MakePlan([]HashedFile{local("a", 10, ""), local("b", 32, "")}, nil)
	if plan.Bytes() != 42 {
		t.Fatalf("bytes = %d", plan.Bytes())
	}
}

// ---- Prepare: what is read ----

func TestPrepareDoesNotReadNewFilesOrFilesOfAnotherSize(t *testing.T) {
	root := t.TempDir()
	write(t, root, "new.txt", "brand new")
	write(t, root, "resized.txt", "now longer than before")
	write(t, root, "same.txt", "1234")
	plan, err := prepare(t, hashed(t, root, nil), []RemoteFile{remote("resized.txt", 3, "x"), remote("same.txt", 4, "no match")})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range plan.Upload {
		if f.Path != "same.txt" && f.Hash != "" {
			t.Errorf("%s is known to differ without reading it, but has the hash %q", f.Path, f.Hash)
		}
		if f.Path == "same.txt" && f.Hash == "" {
			t.Error("a file of the same size must be hashed to tell")
		}
	}
	if len(plan.Upload) != 3 {
		t.Fatalf("upload = %v", uploadPaths(plan))
	}
}

func TestPrepareRecognisesAnUnchangedFileByItsHash(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "hello")
	files := hashed(t, root, nil)
	full, err := prepare(t, files, nil)
	if err != nil || len(full.Upload) != 1 {
		t.Fatal(err)
	}
	h, _ := hashOf(t, filepath.Join(root, "a.txt"))
	plan, err := prepare(t, files, []RemoteFile{remote("a.txt", 5, h)})
	if err != nil || !plan.Empty() || plan.Unchanged != 1 {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
	if other, _ := prepare(t, files, []RemoteFile{remote("a.txt", 5, "something else")}); len(other.Upload) != 1 {
		t.Fatal("same size, other content: changed")
	}
}

func hashOf(t *testing.T, path string) (string, error) {
	t.Helper()
	root := filepath.Dir(path)
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: filepath.Base(path), Dest: "./"}}, NoHTMLToFolders: true})
	if err != nil {
		return "", err
	}
	staged, err := Stage(t.TempDir(), "x", "v1", []HashedFile{{LocalFile: resolved.Files[0]}})
	if err != nil {
		return "", err
	}
	defer staged.Remove()
	return staged.Files[0].Hash, nil
}

func TestPrepareReportsAnUnreadableFileOfTheSameSize(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := t.TempDir()
	write(t, root, "a.txt", "four")
	files := hashed(t, root, nil)
	os.Chmod(filepath.Join(root, "a.txt"), 0o000)
	defer os.Chmod(filepath.Join(root, "a.txt"), 0o644)
	_, err := prepare(t, files, []RemoteFile{remote("a.txt", 4, "h")})
	if err == nil || !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("err = %v", err)
	}
}

func prepare(t *testing.T, local []LocalFile, remote []RemoteFile) (Plan, error) {
	t.Helper()
	return Prepare(local, remote, t.TempDir())
}

// ---- the hash cache ----

func cachePath(base string) string { return filepath.Join(utils.CacheDir(base), "statico-hashes.json") }

func oldFile(t *testing.T, root, name, content string) {
	t.Helper()
	write(t, root, name, content)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, name), past, past); err != nil {
		t.Fatal(err)
	}
}

func TestHashCacheLivesInFutrouCacheAndTrustsHashesWhileSizeAndTimeMatch(t *testing.T) {
	base, root := t.TempDir(), t.TempDir()
	oldFile(t, root, "a.txt", "hello")
	files := hashed(t, root, nil)
	real, _ := hashOf(t, filepath.Join(root, "a.txt"))
	if _, err := Prepare(files, []RemoteFile{remote("a.txt", 5, real)}, base); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cachePath(base))
	if err != nil {
		t.Fatalf("the cache must be in .futrou/cache: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(base, ".futrou", ".gitignore")); err != nil || string(b) != "*\n" {
		t.Fatalf(".futrou needs a .gitignore: %q %v", b, err)
	}
	var entries map[string]struct {
		S int64  `json:"s"`
		M int64  `json:"m"`
		H string `json:"h"`
	}
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("cache = %s err = %v", data, err)
	}
	entry := entries[files[0].Abs]
	if entry.H != real || entry.S != 5 {
		t.Fatalf("entry = %+v, want the hash %s", entry, real)
	}

	// proof that the file is not read again: a planted hash is believed while size and time match
	entry.H = "planted"
	entries[files[0].Abs] = entry
	planted, _ := json.Marshal(entries)
	if err := os.WriteFile(cachePath(base), planted, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := Prepare(files, []RemoteFile{remote("a.txt", 5, "planted")}, base)
	if err != nil || !plan.Empty() {
		t.Fatalf("an unchanged file must be taken from the cache: %+v err = %v", plan, err)
	}

	// a changed modification time invalidates the entry
	later := time.Now().Add(-30 * time.Minute)
	os.Chtimes(filepath.Join(root, "a.txt"), later, later)
	plan, err = Prepare(hashed(t, root, nil), []RemoteFile{remote("a.txt", 5, "planted")}, base)
	if err != nil || len(plan.Upload) != 1 {
		t.Fatalf("a file with another time is read again: %+v err = %v", plan, err)
	}
}

func TestHashCacheCreatesNothingWhenThereIsNothingToKeep(t *testing.T) {
	base, root := t.TempDir(), t.TempDir()
	write(t, root, "fresh.txt", "hello") // modified a moment ago: its time stamp could still change
	if _, err := Prepare(hashed(t, root, nil), []RemoteFile{remote("fresh.txt", 5, "h")}, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, ".futrou")); !os.IsNotExist(err) {
		t.Fatal(".futrou is made only when something needs it")
	}
	// a new file is not read at all, so there is nothing to cache either
	if _, err := Prepare(hashed(t, root, nil), nil, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, ".futrou")); !os.IsNotExist(err) {
		t.Fatal(".futrou must not be made for nothing")
	}
}

func TestHashCacheKeepsOnlyTheFilesOfTheCurrentRun(t *testing.T) {
	base, root := t.TempDir(), t.TempDir()
	oldFile(t, root, "a.txt", "aaaa")
	oldFile(t, root, "b.txt", "bbbb")
	both := hashed(t, root, nil)
	if _, err := Prepare(both, []RemoteFile{remote("a.txt", 4, "x"), remote("b.txt", 4, "x")}, base); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(both[:1], []RemoteFile{remote("a.txt", 4, "x")}, base); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cachePath(base))
	if !strings.Contains(string(data), "a.txt") || strings.Contains(string(data), "b.txt") {
		t.Fatalf("the cache must not grow with files that are no longer uploaded: %s", data)
	}
}

func TestHashCacheSurvivesADamagedFileAndAnUnwritableFolder(t *testing.T) {
	base, root := t.TempDir(), t.TempDir()
	oldFile(t, root, "a.txt", "aaaa")
	files := hashed(t, root, nil)
	if err := os.MkdirAll(utils.CacheDir(base), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath(base), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(files, []RemoteFile{remote("a.txt", 4, "x")}, base); err != nil {
		t.Fatalf("a damaged cache is an empty one: %v", err)
	}
	// .futrou is a file, so the cache folder cannot be made: the cache is skipped, the upload is not
	blocked := t.TempDir()
	os.WriteFile(filepath.Join(blocked, ".futrou"), []byte("x"), 0o644)
	if _, err := Prepare(files, []RemoteFile{remote("a.txt", 4, "x")}, blocked); err != nil {
		t.Fatalf("a cache that cannot be written must not fail the upload: %v", err)
	}
}
