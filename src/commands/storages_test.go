package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testWorkspaceID = "11111111-1111-1111-1111-111111111111"
const testProjectID = "22222222-2222-2222-2222-222222222222"

func TestStoragesList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages", respond(200, []interface{}{fixtureStorage()}))

	out, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", testProjectID)
	assertNoError(t, err)
	assertContains(t, out, "storage-def")
	assertContains(t, out, "my-storage")
}

func TestStoragesList_empty(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages", respond(200, []interface{}{}))

	out, err := runArgs(t, ts, "storages", "list", "--workspace", testWorkspaceID, "--project", testProjectID)
	assertNoError(t, err)
	assertContains(t, out, "No results.")
}

func TestStoragesList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "storages", "list")
	assertError(t, err)
}

func TestStoragesGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def", respond(200, fixtureStorage()))

	out, err := runArgs(t, ts, "storages", "get", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "storage-def")
}

func TestStoragesGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "storages", "get")
	assertError(t, err)
}

func TestStoragesCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/storages", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureStorage())(w, r)
	})

	out, err := runArgs(t, ts, "storages", "create", "--name", "my-storage", "--plan", "plan-1",
		"--workspace", testWorkspaceID, "--project", testProjectID)
	assertNoError(t, err)
	assertContains(t, out, "created")

	if received["name"] != "my-storage" {
		t.Errorf("expected name=my-storage, got %v", received["name"])
	}
	if received["storagePlanId"] != "plan-1" {
		t.Errorf("expected storagePlanId=plan-1, got %v", received["storagePlanId"])
	}
	if received["workspaceId"] != testWorkspaceID {
		t.Errorf("expected workspaceId to be resolved, got %v", received["workspaceId"])
	}
	if received["projectId"] != testProjectID {
		t.Errorf("expected projectId to be resolved, got %v", received["projectId"])
	}
}

func TestStoragesCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "storages", "create", "--name", "x", "--plan", "y")
	assertError(t, err)
}

func TestStoragesUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/storages/storage-def", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureStorage())(w, r)
	})

	out, err := runArgs(t, ts, "storages", "update", "--name", "new-name", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["name"] != "new-name" {
		t.Errorf("expected name=new-name, got %v", received["name"])
	}
}

func TestStoragesUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "storages", "update", "storage-def")
	assertError(t, err)
}

func TestStoragesDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/storages/storage-def", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "storages", "delete", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestStoragesInstances(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/instances", respond(200, []interface{}{
		map[string]interface{}{"id": "inst-1", "status": "ready"},
	}))

	out, err := runArgs(t, ts, "storages", "instances", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "inst-1")
}

func TestStoragesLogs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/logs", respond(200, []interface{}{
		map[string]interface{}{"ts": "2026-01-01T00:00:00Z", "msg": "ready"},
	}))

	out, err := runArgs(t, ts, "storages", "logs", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "ready")
}

func TestStoragesLogs_tail(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/logs/tail", respond(200, []interface{}{
		map[string]interface{}{"ts": "2026-01-01T00:00:00Z", "msg": "tailed"},
	}))

	out, err := runArgs(t, ts, "storages", "logs", "--tail", "storage-def")
	assertNoError(t, err)
	assertContains(t, out, "tailed")
}

func TestStoragePlansList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/plans", respond(200, []interface{}{
		map[string]interface{}{"id": "plan-1", "name": "small", "displayName": "Small", "isPublic": true},
	}))

	out, err := runArgs(t, ts, "storages", "plans", "list")
	assertNoError(t, err)
	assertContains(t, out, "plan-1")
}

func TestStoragePlansGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/plans/plan-1", respond(200, map[string]interface{}{"id": "plan-1", "name": "small"}))

	out, err := runArgs(t, ts, "storages", "plans", "get", "plan-1")
	assertNoError(t, err)
	assertContains(t, out, "plan-1")
}

func TestStorageFilesList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/files/uploads", respond(200, []interface{}{
		map[string]interface{}{"name": "a.txt", "type": "file", "size": 12},
	}))

	out, err := runArgs(t, ts, "storages", "files", "list", "storage-def", "uploads")
	assertNoError(t, err)
	assertContains(t, out, "a.txt")
}

func TestStorageFilesInfo(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/files/uploads/a.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("info") != "true" {
			t.Fatalf("expected info=true query param, got %s", r.URL.RawQuery)
		}
		respond(200, map[string]interface{}{"name": "a.txt", "size": 12})(w, r)
	})

	out, err := runArgs(t, ts, "storages", "files", "info", "storage-def", "uploads/a.txt")
	assertNoError(t, err)
	assertContains(t, out, "a.txt")
}

func TestStorageFilesUploadAndDownload(t *testing.T) {
	ts := newTestServer(t)
	var uploadedBody []byte
	ts.on("PUT", "/v2/storages/storage-def/files/remote.txt", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		uploadedBody = buf[:n]
		w.WriteHeader(http.StatusNoContent)
	})
	ts.on("GET", "/v2/storages/storage-def/files/remote.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("info") == "true" {
			respond(200, map[string]interface{}{"name": "remote.txt", "path": "remote.txt", "isDir": false, "size": 11})(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("hello world"))
	})

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.txt")
	if err := os.WriteFile(localPath, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runArgs(t, ts, "storages", "files", "upload", "storage-def", localPath, "remote.txt")
	assertNoError(t, err)
	assertContains(t, out, "uploaded")
	if string(uploadedBody) != "hello world" {
		t.Errorf("expected uploaded body %q, got %q", "hello world", uploadedBody)
	}

	downloadPath := filepath.Join(dir, "downloaded.txt")
	out, err = runArgs(t, ts, "storages", "files", "download", "storage-def", "remote.txt", downloadPath)
	assertNoError(t, err)
	assertContains(t, out, "Downloaded")
	data, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected downloaded content %q, got %q", "hello world", data)
	}
}

func TestStorageFilesMkdir(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("POST", "/v2/storages/storage-def/files/mkdir/new-folder", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "storages", "files", "mkdir", "storage-def", "new-folder")
	assertNoError(t, err)
	assertContains(t, out, "created")
	if !called {
		t.Error("mkdir endpoint was not called")
	}
}

func TestStorageFilesMove(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/storages/storage-def/files/move/old.txt", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "storages", "files", "move", "storage-def", "old.txt", "new.txt")
	assertNoError(t, err)
	assertContains(t, out, "Moved")
	if received["dest"] != "new.txt" {
		t.Errorf("expected dest=new.txt, got %v", received["dest"])
	}
}

func TestStorageFilesRm(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/storages/storage-def/files/old.txt", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "storages", "files", "rm", "storage-def", "old.txt")
	assertNoError(t, err)
	assertContains(t, out, "Deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestStorageFilesRm_recursive(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/storages/storage-def/files/folder", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("recursive") != "true" {
			t.Fatalf("expected recursive=true query param, got %s", r.URL.RawQuery)
		}
		respondEmpty(w, r)
	})

	_, err := runArgs(t, ts, "storages", "files", "rm", "--recursive", "storage-def", "folder")
	assertNoError(t, err)
}

// mockStorageTree serves a small recursive file tree for glob/sync tests:
//
//	images/cat.png
//	images/dog.png
//	notes.txt
func mockStorageTree(ts *testServer, id string, modified map[string]string) {
	entriesFor := func(dir string) []map[string]interface{} {
		switch dir {
		case "":
			return []map[string]interface{}{
				{"name": "images", "path": "images", "isDir": true, "size": 4096, "modifiedAt": "2026-01-01T00:00:00Z"},
				{"name": "notes.txt", "path": "notes.txt", "isDir": false, "size": 5, "modifiedAt": modifiedOrDefault(modified, "notes.txt")},
			}
		case "images":
			return []map[string]interface{}{
				{"name": "cat.png", "path": "images/cat.png", "isDir": false, "size": 100, "modifiedAt": modifiedOrDefault(modified, "images/cat.png")},
				{"name": "dog.png", "path": "images/dog.png", "isDir": false, "size": 200, "modifiedAt": modifiedOrDefault(modified, "images/dog.png")},
			}
		default:
			return nil
		}
	}
	handler := func(dir string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			entries := entriesFor(dir)
			respond(200, map[string]interface{}{"entries": entries, "limit": 200, "offset": 0, "path": dir, "total": len(entries)})(w, r)
		}
	}
	ts.on("GET", "/v2/storages/"+id+"/files", handler(""))
	ts.on("GET", "/v2/storages/"+id+"/files/images", handler("images"))
}

func modifiedOrDefault(modified map[string]string, path string) string {
	if v, ok := modified[path]; ok {
		return v
	}
	return "2026-01-01T00:00:00Z"
}

func TestStorageFilesList_glob(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)

	out, err := runArgs(t, ts, "storages", "files", "list", "storage-def", "images/*")
	assertNoError(t, err)
	assertContains(t, out, "cat.png")
	assertContains(t, out, "dog.png")
	if strings.Contains(out, "notes.txt") {
		t.Errorf("expected notes.txt to be excluded from images/* glob, got %s", out)
	}
}

func TestStorageFilesList_recursiveGlob(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)

	out, err := runArgs(t, ts, "storages", "files", "list", "storage-def", "**/*.png")
	assertNoError(t, err)
	assertContains(t, out, "cat.png")
	assertContains(t, out, "dog.png")
}

func TestStorageFilesRm_glob(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)
	var deleted []string
	ts.on("DELETE", "/v2/storages/storage-def/files/images/cat.png", func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, "images/cat.png")
		respondEmpty(w, r)
	})
	ts.on("DELETE", "/v2/storages/storage-def/files/images/dog.png", func(w http.ResponseWriter, r *http.Request) {
		deleted = append(deleted, "images/dog.png")
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "storages", "files", "rm", "storage-def", "images/*.png")
	assertNoError(t, err)
	assertContains(t, out, "Deleted 2 file(s)")
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deletes, got %v", deleted)
	}
}

func TestStorageFilesUpload_globAndFolder(t *testing.T) {
	ts := newTestServer(t)
	var uploaded []string
	ts.on("PUT", "/v2/storages/storage-def/files/dest/a.txt", func(w http.ResponseWriter, r *http.Request) {
		uploaded = append(uploaded, "dest/a.txt")
		w.WriteHeader(http.StatusNoContent)
	})
	ts.on("PUT", "/v2/storages/storage-def/files/dest/b.log", func(w http.ResponseWriter, r *http.Request) {
		uploaded = append(uploaded, "dest/b.log")
		w.WriteHeader(http.StatusNoContent)
	})
	ts.on("PUT", "/v2/storages/storage-def/files/dest/c.skip", func(w http.ResponseWriter, r *http.Request) {
		uploaded = append(uploaded, "dest/c.skip")
		w.WriteHeader(http.StatusNoContent)
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.skip"), []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runArgs(t, ts, "storages", "files", "upload", "storage-def", filepath.Join(dir, "*.txt"), "dest")
	assertNoError(t, err)
	assertContains(t, out, "Uploaded 1 file(s)")
	if len(uploaded) != 1 || uploaded[0] != "dest/a.txt" {
		t.Fatalf("expected only a.txt uploaded, got %v", uploaded)
	}

	uploaded = nil
	out, err = runArgs(t, ts, "storages", "files", "upload", "storage-def", dir, "dest")
	assertNoError(t, err)
	assertContains(t, out, "Uploaded 3 file(s)")
	if len(uploaded) != 3 {
		t.Fatalf("expected all 3 files uploaded, got %v", uploaded)
	}
}

func TestStorageFilesDownload_glob(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)
	ts.on("GET", "/v2/storages/storage-def/files/images/cat.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("cat-bytes"))
	})
	ts.on("GET", "/v2/storages/storage-def/files/images/dog.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("dog-bytes"))
	})

	dir := t.TempDir()
	out, err := runArgs(t, ts, "storages", "files", "download", "storage-def", "images/*.png", dir)
	assertNoError(t, err)
	assertContains(t, out, "Downloaded 2 file(s)")

	cat, err := os.ReadFile(filepath.Join(dir, "cat.png"))
	if err != nil || string(cat) != "cat-bytes" {
		t.Fatalf("expected cat.png downloaded, got %v %q", err, cat)
	}
	dog, err := os.ReadFile(filepath.Join(dir, "dog.png"))
	if err != nil || string(dog) != "dog-bytes" {
		t.Fatalf("expected dog.png downloaded, got %v %q", err, dog)
	}
}

func TestStorageSync_uploadsNewerLocalAndDownloadsNewerRemote(t *testing.T) {
	ts := newTestServer(t)

	dir := t.TempDir()
	// local-only.txt exists only locally -> should upload.
	if err := os.WriteFile(filepath.Join(dir, "local-only.txt"), []byte("local"), 0644); err != nil {
		t.Fatal(err)
	}
	// shared.txt exists on both sides; make the local copy newer.
	if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("newer-local"), 0644); err != nil {
		t.Fatal(err)
	}
	newLocalTime := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "shared.txt"), newLocalTime, newLocalTime); err != nil {
		t.Fatal(err)
	}

	remoteEntries := []map[string]interface{}{
		{"name": "shared.txt", "path": "shared.txt", "isDir": false, "size": 3, "modifiedAt": "2020-01-01T00:00:00Z"},
		{"name": "remote-only.txt", "path": "remote-only.txt", "isDir": false, "size": 6, "modifiedAt": "2026-01-01T00:00:00Z"},
	}
	ts.on("GET", "/v2/storages/storage-def/files", respond(200, map[string]interface{}{
		"entries": remoteEntries, "limit": 200, "offset": 0, "path": "", "total": len(remoteEntries),
	}))

	var uploadedShared bool
	ts.on("PUT", "/v2/storages/storage-def/files/shared.txt", func(w http.ResponseWriter, r *http.Request) {
		uploadedShared = true
		w.WriteHeader(http.StatusNoContent)
	})
	var uploadedLocalOnly bool
	ts.on("PUT", "/v2/storages/storage-def/files/local-only.txt", func(w http.ResponseWriter, r *http.Request) {
		uploadedLocalOnly = true
		w.WriteHeader(http.StatusNoContent)
	})
	ts.on("GET", "/v2/storages/storage-def/files/remote-only.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("remote"))
	})

	out, err := runArgs(t, ts, "storages", "sync", "storage-def", "", dir)
	assertNoError(t, err)
	assertContains(t, out, "2 uploaded")
	if !uploadedShared {
		t.Error("expected shared.txt (newer locally) to be uploaded")
	}
	if !uploadedLocalOnly {
		t.Error("expected local-only.txt to be uploaded")
	}
	data, err := os.ReadFile(filepath.Join(dir, "remote-only.txt"))
	if err != nil || string(data) != "remote" {
		t.Fatalf("expected remote-only.txt downloaded, got %v %q", err, data)
	}
}

func TestStorageSync_dryRunMakesNoChanges(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "local-only.txt"), []byte("local"), 0644); err != nil {
		t.Fatal(err)
	}
	ts.on("GET", "/v2/storages/storage-def/files", respond(200, map[string]interface{}{
		"entries": []interface{}{}, "limit": 200, "offset": 0, "path": "", "total": 0,
	}))
	ts.on("PUT", "/v2/storages/storage-def/files/local-only.txt", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("dry-run must not upload")
	})

	out, err := runArgs(t, ts, "storages", "sync", "--dry-run", "storage-def", "", dir)
	assertNoError(t, err)
	assertContains(t, out, "Would sync")
}

func TestStorageFilesUpload_missingLocalFile(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "storages", "files", "upload", "storage-def", "/no/such/file.txt", "dest.txt")
	assertError(t, err)
}

func TestStorageFilesUpload_apiErrorStatus(t *testing.T) {
	ts := newTestServer(t)
	ts.on("PUT", "/v2/storages/storage-def/files/dest.txt", respond(500, map[string]interface{}{"message": "disk full"}))

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.txt")
	if err := os.WriteFile(localPath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runArgs(t, ts, "storages", "files", "upload", "storage-def", localPath, "dest.txt")
	assertError(t, err)
}

func TestStorageFilesDownload_apiErrorStatus(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/files/missing.txt", respond(404, map[string]interface{}{"message": "not found"}))

	dir := t.TempDir()
	_, err := runArgs(t, ts, "storages", "files", "download", "storage-def", "missing.txt", filepath.Join(dir, "out.txt"))
	assertError(t, err)
}

func TestStorageFilesList_globNoMatches(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)

	out, err := runArgs(t, ts, "storages", "files", "list", "storage-def", "**/*.gif")
	assertNoError(t, err)
	assertContains(t, out, "No results.")
}

func TestStorageFilesRm_globNoMatches(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)

	_, err := runArgs(t, ts, "storages", "files", "rm", "storage-def", "**/*.gif")
	assertError(t, err)
}

func TestStorageFilesDownload_globNoMatches(t *testing.T) {
	ts := newTestServer(t)
	mockStorageTree(ts, "storage-def", nil)

	dir := t.TempDir()
	_, err := runArgs(t, ts, "storages", "files", "download", "storage-def", "**/*.gif", dir)
	assertError(t, err)
}

func TestStorageFilesUpload_globNoMatches(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runArgs(t, ts, "storages", "files", "upload", "storage-def", filepath.Join(dir, "*.gif"), "dest")
	assertError(t, err)
}

func TestStorageSync_missingLocalDirIsCreated(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/storages/storage-def/files", respond(200, map[string]interface{}{
		"entries": []interface{}{}, "limit": 200, "offset": 0, "path": "", "total": 0,
	}))

	dir := t.TempDir()
	localDir := filepath.Join(dir, "does-not-exist-yet")
	out, err := runArgs(t, ts, "storages", "sync", "storage-def", "", localDir)
	assertNoError(t, err)
	assertContains(t, out, "0 uploaded, 0 downloaded, 0 up to date")
	if info, statErr := os.Stat(localDir); statErr != nil || !info.IsDir() {
		t.Fatalf("expected sync to create %s", localDir)
	}
}

func TestStorageSync_localPathIsFileErrors(t *testing.T) {
	ts := newTestServer(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runArgs(t, ts, "storages", "sync", "storage-def", "", filePath)
	assertError(t, err)
}
