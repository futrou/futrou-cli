package statico_test

import (
	"fmt"
	. "futrou-cli/src/statico"
	"futrou-cli/src/utils"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"futrou-cli/src/services"
	"futrou-cli/src/statico/mock"
)

func newFake(t *testing.T) (*mock.Server, *services.ApiClient) {
	t.Helper()
	fake := mock.New()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fake.Handle(w, r) {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return fake, services.NewApiClientWithToken(ts.URL, mock.Token)
}

func staticoOf(t *testing.T, c Client, name string) Statico {
	t.Helper()
	statico, err := FindStatico(c, name, Scope{WorkspaceID: mock.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	return *statico
}

func hashed(t *testing.T, root string, rules utils.GlobRules) []LocalFile {
	t.Helper()
	resolved, err := Resolve(root, Selection{Rules: rules, NoHTMLToFolders: true})
	if err != nil {
		t.Fatal(err)
	}
	return resolved.Files
}

// tree is a small built site.
func tree(t *testing.T) string {
	root := t.TempDir()
	for _, f := range []string{"dist/index.html", "dist/app.js", "dist/app.js.map", "dist/img/logo.png", "dist/drafts/wip.html"} {
		write(t, root, f, map[string]string{"dist/index.html": "i", "dist/app.js": "a", "dist/app.js.map": "m", "dist/img/logo.png": "l", "dist/drafts/wip.html": "w"}[f])
	}
	return root
}

func TestUploadFirstVersionSendsEverythingAndActivates(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := tree(t)
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, utils.GlobRules{{Source: "dist", Dest: "./"}}), UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Activated || result.Uploaded != 4 || result.Deleted != 0 {
		t.Fatalf("result = %+v", result)
	}
	if got := fake.ActiveFiles("docs"); len(got) != 4 || got["index.html"] != "i" || got["img/logo.png"] != "l" {
		t.Fatalf("served files = %v", got)
	}
	if fake.Unauthorized != 0 {
		t.Fatalf("%d requests did not carry the CLI's token", fake.Unauthorized)
	}
}

func TestUploadSendsOnlyChangedFilesAndDeletesGoneOnes(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i", "app.js": "old", "gone.txt": "x"})
	root := t.TempDir()
	write(t, root, "index.html", "i") // unchanged
	write(t, root, "app.js", "new")   // changed
	write(t, root, "new.css", "css")  // new
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 2 || result.Deleted != 1 || result.Unchanged != 1 {
		t.Fatalf("result = %+v", result)
	}
	want := map[string]string{"index.html": "i", "app.js": "new", "new.css": "css"}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
	if fake.ArchiveFiles != 2 || fake.Archives != 1 || fake.Count("PUT", "/v2/storages/") != 0 {
		t.Fatalf("the 2 changed files travel in one archive: archives = %d (%d files), PUTs = %d", fake.Archives, fake.ArchiveFiles, fake.Count("PUT", "/v2/storages/"))
	}
}

func TestUploadEmptyStartsWithoutTheServedFiles(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"old.txt": "o"})
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	opts := UploadOptions{}
	opts.Empty = true
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), opts); err != nil {
		t.Fatal(err)
	}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, map[string]string{"a.txt": "a"}) {
		t.Fatalf("served = %v", got)
	}
}

func TestUploadOfIdenticalFilesCreatesNoVersion(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i"})
	root := t.TempDir()
	write(t, root, "index.html", "i")
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || !result.Skipped {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if n := fake.Count("POST", "/v2/staticos/"); n != 0 {
		t.Fatalf("a version was created for identical files (%d POSTs)", n)
	}
}

func TestUploadDeploysChangedAnswersEvenWithTheSameFiles(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i"})
	root := t.TempDir()
	write(t, root, "index.html", "i")
	opts := UploadOptions{}
	opts.FallbackFile, opts.FallbackStatus = "/index.html", 200
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), opts)
	if err != nil || result.Skipped || !result.Activated {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	v := fake.Statico("docs").Versions[len(fake.Statico("docs").Versions)-1]
	if v.Extra["fallbackFile"] != "/index.html" || v.Extra["fallbackStatus"] != float64(200) {
		t.Fatalf("answers = %v", v.Extra)
	}
	// the same again is up to date
	if again, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), opts); err != nil || !again.Skipped {
		t.Fatalf("again = %+v, err = %v", again, err)
	}
}

func TestUploadNoActivateLeavesTheServedVersion(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "old"})
	root := t.TempDir()
	write(t, root, "index.html", "new")
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{NoActivate: true})
	if err != nil || result.Activated {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if fake.ActiveFiles("docs")["index.html"] != "old" {
		t.Fatal("the served version changed")
	}
}

func TestUploadFailureMarksTheVersionFailedAndServesTheOldFiles(t *testing.T) {
	fake, client := newFake(t)
	statico := fake.AddStatico("docs", map[string]string{"index.html": "old"})
	root := t.TempDir()
	write(t, root, "index.html", "new")
	fake.FailWrites = 500
	_, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "uploading index.html") {
		t.Fatalf("err = %v", err)
	}
	if fake.ActiveFiles("docs")["index.html"] != "old" {
		t.Fatal("a failed upload changed what is served")
	}
	last := statico.Versions[len(statico.Versions)-1]
	if last.Status != "failed" || last.Error == "" {
		t.Fatalf("version = %+v, want failed with a reason", last)
	}
}

func TestUploadRefusedActivationIsReported(t *testing.T) {
	fake, client := newFake(t)
	statico := fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	fake.Fail["POST /v2/staticos/"+statico.ID+"/versions/v1/activate"] = 413
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err == nil || !strings.Contains(err.Error(), "activating version v1") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadErrors(t *testing.T) {
	fake, client := newFake(t)
	statico := fake.AddStatico("docs", nil)
	if _, err := Upload(client, staticoOf(t, client, "docs"), nil, UploadOptions{}); err == nil || !strings.Contains(err.Error(), "nothing to upload") {
		t.Fatalf("err = %v", err)
	}
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	fake.Fail["POST /v2/staticos/"+statico.ID+"/versions"] = 403
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err == nil || !strings.Contains(err.Error(), "creating a version") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadFilesWithSpecialCharactersInTheirNames(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "my docs/a b#1%.html", "x")
	write(t, root, "ünï/ç.txt", "y")
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	got := fake.ActiveFiles("docs")
	if got["my docs/a b#1%.html"] != "x" || got["ünï/ç.txt"] != "y" {
		t.Fatalf("served = %v", got)
	}
}

func TestUploadManyFilesInParallel(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	for i := 0; i < 120; i++ {
		write(t, root, filepath.Join("d", string(rune('a'+i%26)), "f"+string(rune('a'+i/26))+".txt"), "c")
	}
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || result.Uploaded != 120 || len(fake.ActiveFiles("docs")) != 120 {
		t.Fatalf("result = %+v, err = %v, files = %d", result, err, len(fake.ActiveFiles("docs")))
	}
}

func TestCompareChangesNothing(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"a.txt": "a", "b.txt": "b"})
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	write(t, root, "c.txt", "c")
	plan, err := Compare(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Upload) != 1 || plan.Upload[0].Path != "c.txt" || !reflect.DeepEqual(plan.Delete, []string{"b.txt"}) || plan.Unchanged != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if got := fake.Count("POST", ""); got != 0 {
		t.Fatalf("a comparison made %d changes", got)
	}
	if plan.Bytes() != 1 || plan.Empty() {
		t.Fatalf("bytes = %d empty = %v", plan.Bytes(), plan.Empty())
	}
}

func TestMakePlan(t *testing.T) {
	local := []HashedFile{{LocalFile: LocalFile{Path: "a"}, Hash: "1"}, {LocalFile: LocalFile{Path: "b"}, Hash: "2"}}
	remote := []RemoteFile{{Path: "a", Hash: "1"}, {Path: "b", Hash: "x"}, {Path: "c", Hash: "3"}}
	plan := MakePlan(local, remote)
	if len(plan.Upload) != 1 || plan.Upload[0].Path != "b" || !reflect.DeepEqual(plan.Delete, []string{"c"}) || plan.Unchanged != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if !MakePlan(local[:1], remote[:1]).Empty() {
		t.Fatal("identical files make an empty plan")
	}
}

type counter struct {
	mu      sync.Mutex
	started map[string]int64
	bytes   map[string]int64
	done    []string
	begun   int
	total   int64
	ended   bool
}

func newCounter() *counter { return &counter{started: map[string]int64{}, bytes: map[string]int64{}} }

func (c *counter) Message(string) {}
func (c *counter) Begin(files int, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.begun, c.total = files, bytes
}
func (c *counter) FileStart(p string, size int64) { c.mu.Lock(); c.started[p] = size; c.mu.Unlock() }
func (c *counter) FileBytes(p string, n int64)    { c.mu.Lock(); c.bytes[p] += n; c.mu.Unlock() }
func (c *counter) FileDone(p string)              { c.mu.Lock(); c.done = append(c.done, p); c.mu.Unlock() }
func (c *counter) End()                           { c.mu.Lock(); c.ended = true; c.mu.Unlock() }

func TestUploadReportsEveryFileAndItsBytes(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"same.txt": "same"})
	root := t.TempDir()
	write(t, root, "same.txt", "same")
	write(t, root, "a.txt", "12345")
	write(t, root, "dir/b.txt", "1234567890")
	progress := newCounter()
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress}); err != nil {
		t.Fatal(err)
	}
	if progress.begun != 2 || progress.total != 15 || !progress.ended || len(progress.done) != 2 {
		t.Fatalf("begin = %d/%d done = %v ended = %v", progress.begun, progress.total, progress.done, progress.ended)
	}
	for path, size := range progress.started {
		if progress.bytes[path] != size {
			t.Errorf("%s: %d of %d bytes reported", path, progress.bytes[path], size)
		}
	}
	if _, unchangedReported := progress.started["same.txt"]; unchangedReported {
		t.Error("an unchanged file is not sent, so it is not reported")
	}
}

func TestUploadEndsTheProgressWhenAFileFails(t *testing.T) {
	fake, client := newFake(t)
	_ = fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	fake.FailWrites = 500
	progress := newCounter()
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress}); err == nil {
		t.Fatal("expected an error")
	}
	if !progress.ended || len(progress.done) != 0 {
		t.Fatalf("ended = %v done = %v", progress.ended, progress.done)
	}
}

func TestSyncUploadsTheSelectionAndLeavesNoStagingBehind(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := project(t)
	os.RemoveAll(filepath.Join(root, ".futrou")) // the sample project has one of its own
	st := staticoOf(t, client, "docs")
	result, err := Sync(client, st, root, Selection{Rules: utils.GlobRules{{Source: "./src/public", Dest: "./"}, {Source: "./src/public/*.{js,css}", Exclude: true}}, NoHTMLToFolders: true}, UploadOptions{})
	if err != nil || !result.Activated {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	got := fake.ActiveFiles("docs")
	for _, p := range []string{"logo.svg", "main.html", "images/a.jpg", "images/deep/d.jpg"} {
		if got[p] != p && got[p] == "" {
			t.Errorf("%s was not uploaded: %v", p, got)
		}
	}
	for _, p := range []string{"app.js", "style.css"} {
		if _, ok := got[p]; ok {
			t.Errorf("%s was excluded", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatalf(".futrou must be removed when nothing else is in it: %v", err)
	}
}

func TestSyncUploadsTheHTMLToFolderCopiesToo(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "home")
	write(t, root, "about.html", "about")
	write(t, root, "blog/post.htm", "post")
	if _, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{}, UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"index.html": "home", "about.html": "about", "about/index.html": "about", "blog/post.htm": "post", "blog/post/index.html": "post"}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}

func TestSyncSetsTheRedirectsOfAnUnderscoreRedirectsFileAfterTheVersionExists(t *testing.T) {
	fake, client := newFake(t)
	st := fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, "_redirects", "/old /new 301\n/blog/* /news/:splat\n/bad 200 200\n")
	var messages []string
	opts := UploadOptions{Progress: recorder{&messages}}
	result, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{}, opts)
	if err != nil || !result.Activated {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if _, served := fake.ActiveFiles("docs")["_redirects"]; served {
		t.Fatal("_redirects must not be uploaded")
	}
	version := st.Versions[0]
	redirects, _ := version.Extra["redirects"].([]interface{})
	if len(redirects) != 2 {
		t.Fatalf("redirects = %v", version.Extra["redirects"])
	}
	// the rules are set with a PATCH after the version was created, not in its creation
	if fake.Count("PATCH", "/v2/staticos/"+st.ID+"/versions/v1") != 2 { // uploading, then the redirects
		t.Fatalf("log = %v", fake.Log)
	}
	if !contains(messages, "warning: _redirects line 3: the status 200 (proxying) is not supported; use a redirect status") {
		t.Fatalf("messages = %v", messages)
	}
}

type recorder struct{ messages *[]string }

func (r recorder) Message(text string)     { *r.messages = append(*r.messages, text) }
func (r recorder) Begin(int, int64)        {}
func (r recorder) FileStart(string, int64) {}
func (r recorder) FileBytes(string, int64) {}
func (r recorder) FileDone(string)         {}
func (r recorder) End()                    {}

func TestSyncDeclaredRedirectsComeBeforeTheFilesRedirects(t *testing.T) {
	fake, client := newFake(t)
	st := fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, "_redirects", "/from-file /b\n")
	declared, err := RedirectRules([]Redirect{{From: "/from-config", To: "/a", StatusCode: 301}})
	if err != nil {
		t.Fatal(err)
	}
	opts := UploadOptions{}
	opts.Redirects = declared
	if _, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{}, opts); err != nil {
		t.Fatal(err)
	}
	redirects := st.Versions[0].Extra["redirects"].([]interface{})
	if len(redirects) != 2 {
		t.Fatalf("redirects = %v", redirects)
	}
	first := redirects[0].(map[string]interface{})["if"].(map[string]interface{})["path"]
	second := redirects[1].(map[string]interface{})["if"].(map[string]interface{})["path"]
	if first != "/from-config" || second != "/from-file" {
		t.Fatalf("order = %v, %v", first, second)
	}
}

func TestSyncDoesNothingWhenFilesAndRedirectsAreTheSame(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, "_redirects", "/old /new 301\n")
	st := staticoOf(t, client, "docs")
	if _, err := Sync(client, st, root, Selection{}, UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err := Sync(client, st, root, Selection{}, UploadOptions{})
	if err != nil || !again.Skipped {
		t.Fatalf("again = %+v err = %v", again, err)
	}
	// a changed _redirects alone makes a new version
	write(t, root, "_redirects", "/old /newer 301\n")
	changed, err := Sync(client, st, root, Selection{}, UploadOptions{})
	if err != nil || changed.Skipped || !changed.Activated {
		t.Fatalf("changed = %+v err = %v", changed, err)
	}
}

func TestSyncKeepsTheStagedFilesWhenTheUploadFailsAndSaysWhere(t *testing.T) {
	fake, client := newFake(t)
	_ = fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	fake.FailWrites = 500
	_, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{NoHTMLToFolders: true}, UploadOptions{})
	if err == nil {
		t.Fatal("expected the upload to fail")
	}
	staged := filepath.Join(root, ".futrou", "statico", "docs", "v1")
	if !strings.Contains(err.Error(), staged) {
		t.Fatalf("the error must say where the staged files are: %v", err)
	}
	if b, readErr := os.ReadFile(filepath.Join(staged, "index.html")); readErr != nil || string(b) != "i" {
		t.Fatalf("staged = %q err = %v", b, readErr)
	}
	// the next run clears it
	if err := ClearWorkDir(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou", "statico")); !os.IsNotExist(err) {
		t.Fatal("ClearWorkDir must remove the staged files")
	}
}

func TestSyncStagesIntoAFolderNamedAfterTheVersion(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	var seen []string
	opts := UploadOptions{Progress: hook{onStart: func(string) {
		filepath.WalkDir(filepath.Join(root, ".futrou"), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				seen = append(seen, filepath.ToSlash(strings.TrimPrefix(p, root)))
			}
			return nil
		})
	}}}
	if _, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{NoHTMLToFolders: true}, opts); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range seen {
		if p == "/.futrou/statico/docs/v1/index.html" {
			found = true
		}
		if strings.Contains(p, ".staging-") {
			t.Errorf("while files are sent they are in the version's folder, not %s", p)
		}
	}
	if !found {
		t.Fatalf("files seen during the upload: %v", seen)
	}
}

func TestSyncKeepsAFutrouFolderThatHoldsOtherThings(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, ".futrou/keep.txt", "mine")
	if _, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{NoHTMLToFolders: true}, UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(root, ".futrou", "keep.txt")); err != nil || string(b) != "mine" {
		t.Fatalf("keep.txt = %q err = %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou", "statico")); !os.IsNotExist(err) {
		t.Fatal("the staged files must be removed")
	}
	if _, ok := fake.ActiveFiles("docs")[".futrou/keep.txt"]; ok {
		t.Fatal(".futrou is never uploaded")
	}
}

func TestSyncUploadsASnapshotNotTheFilesAsTheyChangeLater(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "one")
	var edited bool
	opts := UploadOptions{Progress: hook{onStart: func(string) {
		if !edited {
			edited = true
			write(t, root, "a.txt", "two") // edited while the upload runs
		}
	}}}
	if _, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{NoHTMLToFolders: true}, opts); err != nil {
		t.Fatal(err)
	}
	if got := fake.ActiveFiles("docs")["a.txt"]; got != "one" {
		t.Fatalf("uploaded %q; the staged copy must be what was hashed", got)
	}
}

type hook struct{ onStart func(string) }

func (h hook) Message(string)              {}
func (h hook) Begin(int, int64)            {}
func (h hook) FileStart(p string, _ int64) { h.onStart(p) }
func (h hook) FileBytes(string, int64)     {}
func (h hook) FileDone(string)             {}
func (h hook) End()                        {}

func TestSyncFailsBeforeStagingWhenTheSelectionIsBad(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	_, err := Sync(client, staticoOf(t, client, "docs"), root, Selection{Rules: utils.GlobRules{{Source: "missing"}}}, UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("nothing may be staged for a bad selection")
	}
	if fake.Count("POST", "/v2/staticos/") != 0 {
		t.Fatal("the API must not be touched")
	}
}

func TestCheckComparesWithoutChangingAnything(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i", "gone.txt": "g"})
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, "new.txt", "n")
	st := staticoOf(t, client, "docs")
	diff, err := Check(client, &st, root, Selection{NoHTMLToFolders: true}, UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Changed() || len(diff.Plan.Upload) != 1 || diff.Plan.Upload[0].Path != "new.txt" || !reflect.DeepEqual(diff.Plan.Delete, []string{"gone.txt"}) {
		t.Fatalf("diff = %+v", diff.Plan)
	}
	if !strings.Contains(diff.Describe(), "2 file(s) selected: 1 to upload") || !strings.Contains(diff.Describe(), "1 to delete") {
		t.Fatalf("describe = %q", diff.Describe())
	}
	if fake.Count("POST", "") != 0 || fake.Count("PUT", "") != 0 || fake.Count("DELETE", "") != 0 {
		t.Fatalf("a check changed something: %v", fake.Log)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("a check stages nothing")
	}
}

func TestCheckOfANewWebsiteHasEverythingToUpload(t *testing.T) {
	_, client := newFake(t)
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	diff, err := Check(client, nil, root, Selection{NoHTMLToFolders: true}, UploadOptions{})
	if err != nil || !diff.New || !diff.Changed() || len(diff.Plan.Upload) != 1 {
		t.Fatalf("diff = %+v err = %v", diff, err)
	}
}

func TestCheckSeesNoChangeForTheSameFilesAndAnswers(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"index.html": "i"})
	root := t.TempDir()
	write(t, root, "index.html", "i")
	st := staticoOf(t, client, "docs")
	diff, err := Check(client, &st, root, Selection{NoHTMLToFolders: true}, UploadOptions{})
	if err != nil || diff.Changed() {
		t.Fatalf("diff = %+v err = %v", diff.Plan, err)
	}
	opts := UploadOptions{}
	opts.FallbackFile = "/index.html"
	diff, err = Check(client, &st, root, Selection{NoHTMLToFolders: true}, opts)
	if err != nil || !diff.Changed() || !diff.AnswersChanged || !diff.Plan.Empty() {
		t.Fatalf("a changed answer alone is a change: %+v err = %v", diff, err)
	}
}

func TestCheckFailsForABadSelectionOrUnreadableFiles(t *testing.T) {
	_, client := newFake(t)
	root := t.TempDir()
	if _, err := Check(client, nil, root, Selection{Rules: utils.GlobRules{{Source: "nope"}}}, UploadOptions{}); err == nil {
		t.Fatal("a missing include must fail the check")
	}
}

// ---- sending: archives, big files, folders ----

func manyFiles(t *testing.T, root string, folder string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		write(t, root, fmt.Sprintf("%s/f%04d.txt", folder, i), fmt.Sprintf("content %d", i))
	}
}

func TestUploadSendsManySmallFilesAsOneArchive(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	manyFiles(t, root, "pages", 300)
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || result.Uploaded != 300 {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if fake.Archives != 1 || fake.ArchiveFiles != 300 || fake.Count("PUT", "/v2/storages/") != 0 {
		t.Fatalf("archives = %d (%d files), PUTs = %d: 300 small files are one request", fake.Archives, fake.ArchiveFiles, fake.Count("PUT", "/v2/storages/"))
	}
	if len(fake.ActiveFiles("docs")) != 300 || fake.ActiveFiles("docs")["pages/f0042.txt"] != "content 42" {
		t.Fatal("the archive was not unpacked")
	}
}

func TestUploadSplitsArchivesAtTheFileLimit(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	manyFiles(t, root, "p", 2100)
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if fake.Archives != 2 || fake.ArchiveFiles != 2100 {
		t.Fatalf("archives = %d (%d files), want 2 holding 2100", fake.Archives, fake.ArchiveFiles)
	}
}

func TestUploadSendsEverythingInCompressedArchives(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "video.bin", strings.Repeat("B", 600<<10))
	write(t, root, "big2.bin", strings.Repeat("B", 600<<10)+"2")
	manyFiles(t, root, "small", 10)
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if fake.Archives != 1 || fake.ArchiveFiles != 12 || fake.Count("PUT", "/v2/storages/") != 0 {
		t.Fatalf("archives = %d (%d files), PUTs = %d; want one archive of 12 and no bare file", fake.Archives, fake.ArchiveFiles, fake.Count("PUT", "/v2/storages/"))
	}
	if got := fake.ActiveFiles("docs"); got["video.bin"] != strings.Repeat("B", 600<<10) || len(got) != 12 {
		t.Fatalf("served %d files", len(got))
	}
}

func TestUploadSplitsArchivesAtTheByteLimit(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	chunk := strings.Repeat("x", 500<<10) // small files that together exceed 32 MiB
	for i := 0; i < 70; i++ {
		write(t, root, fmt.Sprintf("c%03d.bin", i), chunk+fmt.Sprint(i))
	}
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if fake.Archives < 2 || fake.ArchiveFiles != 70 {
		t.Fatalf("archives = %d (%d files): 70 x 500 KiB does not fit one 32 MiB archive", fake.Archives, fake.ArchiveFiles)
	}
}

func TestUploadOfASingleFileIsStillAnArchive(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "only.txt", "x")
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if fake.Archives != 1 || fake.Count("PUT", "/v2/storages/") != 0 {
		t.Fatalf("archives = %d PUTs = %d: even one file is sent compressed", fake.Archives, fake.Count("PUT", "/v2/storages/"))
	}
}

func TestUploadDeletesAGoneFolderInOneRequest(t *testing.T) {
	fake, client := newFake(t)
	served := map[string]string{"index.html": "i"}
	for i := 0; i < 60; i++ {
		served[fmt.Sprintf("old/deep/f%02d.txt", i)] = "x"
	}
	fake.AddStatico("docs", served)
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, "new.txt", "n")
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || result.Deleted != 60 {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if fake.RecursiveDeletes != 1 || fake.Count("DELETE", "/v2/storages/") != 1 {
		t.Fatalf("recursive deletes = %d, DELETE requests = %d: a folder that is gone is one request", fake.RecursiveDeletes, fake.Count("DELETE", "/v2/storages/"))
	}
	if got := fake.ActiveFiles("docs"); len(got) != 2 {
		t.Fatalf("served = %v", got)
	}
}

func TestUploadHandlesAFileBecomingAFolderAndAFolderBecomingAFile(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", map[string]string{"was-file": "f", "was-dir/inner.txt": "i", "keep.txt": "k"})
	root := t.TempDir()
	write(t, root, "was-file/now-inside.txt", "n")
	write(t, root, "was-dir", "now a file")
	write(t, root, "keep.txt", "k")
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"was-file/now-inside.txt": "n", "was-dir": "now a file", "keep.txt": "k"}
	if got := fake.ActiveFiles("docs"); !reflect.DeepEqual(got, want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}

func TestUploadSkipsUnchangedFoldersAndSendsOnlyTheChangedFile(t *testing.T) {
	fake, client := newFake(t)
	served := map[string]string{}
	root := t.TempDir()
	for _, folder := range []string{"assets", "docs", "img"} {
		for i := 0; i < 20; i++ {
			name := fmt.Sprintf("%s/f%02d.txt", folder, i)
			served[name] = "same " + name
			write(t, root, name, "same "+name)
		}
	}
	fake.AddStatico("docs", served)
	write(t, root, "docs/f05.txt", "this one changed")
	result, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || result.Uploaded != 1 || result.Unchanged != 59 {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if fake.Archives != 1 || fake.Count("PUT", "/v2/storages/") != 0 {
		t.Fatalf("archives = %d PUTs = %d", fake.Archives, fake.Count("PUT", "/v2/storages/"))
	}
	plan, err := Compare(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{})
	if err != nil || !plan.Empty() || plan.Unchanged != 60 {
		t.Fatalf("after the upload everything is the same: %+v err = %v", plan, err)
	}
}

func TestUploadStagesOnlyTheFilesThatChanged(t *testing.T) {
	fake, client := newFake(t)
	_ = fake.AddStatico("docs", map[string]string{"same.txt": "same", "changed.txt": "old"})
	root := t.TempDir()
	write(t, root, "same.txt", "same")
	write(t, root, "changed.txt", "new")
	fake.FailWrites = 500
	opts := UploadOptions{Base: root, Stage: true}
	_, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), opts)
	if err == nil {
		t.Fatal("expected a failure")
	}
	staged := filepath.Join(root, ".futrou", "statico", "docs", "v2")
	if !strings.Contains(err.Error(), staged) {
		t.Fatalf("the error must say where the staged files are: %v", err)
	}
	entries, _ := os.ReadDir(staged)
	if len(entries) != 1 || entries[0].Name() != "changed.txt" {
		t.Fatalf("staged = %v: only the changed file is copied", entries)
	}
}

func TestUploadRemovesTheStagedFilesWhenItWorked(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "a")
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Base: root, Stage: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".futrou")); !os.IsNotExist(err) {
		t.Fatal("the staged files must be removed")
	}
}

func TestUploadFailsWhenArchivesAndSingleFilesAreRefused(t *testing.T) {
	fake, client := newFake(t)
	st := fake.AddStatico("docs", nil)
	root := t.TempDir()
	manyFiles(t, root, "p", 20)
	fake.FailWrites = 500
	progress := newCounter()
	_, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress})
	if err == nil || !strings.Contains(err.Error(), "forced failure") {
		t.Fatalf("err = %v", err)
	}
	if !progress.ended {
		t.Fatal("the progress must end when an upload fails")
	}
	if last := st.Versions[len(st.Versions)-1]; last.Status != "failed" {
		t.Fatalf("version = %+v", last)
	}
}

// A file that changed while it was packed fails the archive; the batch is then
// sent file by file, with a warning, and the upload still works.
func TestUploadFallsBackToFilesWhenAFileChangedWhileItWasBeingPacked(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	write(t, root, "a.txt", "short")
	write(t, root, "b.txt", "other")
	files := hashed(t, root, nil)
	write(t, root, "a.txt", "now it is much longer than before") // no staging: sent from where it is
	progress := &messages{}
	if _, err := Upload(client, staticoOf(t, client, "docs"), files, UploadOptions{Progress: progress}); err != nil {
		t.Fatal(err)
	}
	if len(fake.ActiveFiles("docs")) != 2 {
		t.Fatalf("served %v", fake.ActiveFiles("docs"))
	}
	if !progress.has("warning: uploading 2 files as an archive failed") {
		t.Fatalf("no warning in %v", progress.texts)
	}
}

type messages struct {
	silentProgress
	mu    sync.Mutex
	texts []string
}

func (m *messages) Message(text string) { m.mu.Lock(); m.texts = append(m.texts, text); m.mu.Unlock() }
func (m *messages) has(prefix string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.texts {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

type silentProgress struct{}

func (silentProgress) Message(string)          {}
func (silentProgress) Begin(int, int64)        {}
func (silentProgress) FileStart(string, int64) {}
func (silentProgress) FileBytes(string, int64) {}
func (silentProgress) FileDone(string)         {}
func (silentProgress) End()                    {}

func TestUploadArchiveProgressCountsEveryFile(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	root := t.TempDir()
	manyFiles(t, root, "p", 25)
	progress := newCounter()
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress}); err != nil {
		t.Fatal(err)
	}
	if progress.begun != 25 || len(progress.done) != 25 || !progress.ended {
		t.Fatalf("begun = %d done = %d ended = %v", progress.begun, len(progress.done), progress.ended)
	}
	for path, size := range progress.started {
		if progress.bytes[path] != size {
			t.Errorf("%s: %d of %d bytes", path, progress.bytes[path], size)
		}
	}
}

// An API whose Content-Type check refuses the archive route gets the files one
// by one, once, with a notice.
func TestUploadFallsBackToBareFilesWhenTheAPIRefusesArchives(t *testing.T) {
	fake, client := newFake(t)
	fake.AddStatico("docs", nil)
	fake.RejectArchives = true
	root := t.TempDir()
	manyFiles(t, root, "f", 5)
	progress := newCounter()
	if _, err := Upload(client, staticoOf(t, client, "docs"), hashed(t, root, nil), UploadOptions{Progress: progress}); err != nil {
		t.Fatal(err)
	}
	if fake.Archives != 0 || fake.Count("PUT", "/v2/storages/") != 5 || len(fake.ActiveFiles("docs")) != 5 {
		t.Fatalf("archives = %d, PUTs = %d", fake.Archives, fake.Count("PUT", "/v2/storages/"))
	}
}
