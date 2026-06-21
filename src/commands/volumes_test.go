package commands

import (
	"net/http"
	"testing"
)

func TestVolumesList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/volumes", respond(200, []interface{}{fixtureVolume()}))

	out, err := runArgs(t, ts, "volumes", "list")
	assertNoError(t, err)
	assertContains(t, out, "vol-def")
	assertContains(t, out, "my-vol")
}

func TestVolumesList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "volumes", "list")
	assertError(t, err)
}

func TestVolumesGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/volumes/vol-def", respond(200, fixtureVolume()))

	out, err := runArgs(t, ts, "volumes", "get", "vol-def")
	assertNoError(t, err)
	assertContains(t, out, "vol-def")
}

func TestVolumesGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "volumes", "get")
	assertError(t, err)
}

func TestVolumesGet_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/volumes/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "volumes", "get", "bad-id")
	assertError(t, err)
}

func TestVolumesCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/volumes", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureVolume())(w, r)
	})

	out, err := runArgs(t, ts, "volumes", "create", "--name", "my-vol")
	assertNoError(t, err)
	assertContains(t, out, "created")
	if received["name"] != "my-vol" {
		t.Errorf("expected name=my-vol, got %v", received["name"])
	}
}

func TestVolumesCreate_withSizeAndType(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/volumes", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureVolume())(w, r)
	})

	_, err := runArgs(t, ts, "volumes", "create",
		"--name", "big-vol",
		"--size", "50",
		"--type", "nvme",
	)
	assertNoError(t, err)
	if received["sizeGb"].(float64) != 50 {
		t.Errorf("expected sizeGb=50, got %v", received["sizeGb"])
	}
	if received["type"] != "nvme" {
		t.Errorf("expected type=nvme, got %v", received["type"])
	}
}

func TestVolumesCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "volumes", "create", "--name", "x")
	assertError(t, err)
}

func TestVolumesUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/volumes/vol-def", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureVolume())(w, r)
	})

	out, err := runArgs(t, ts, "volumes", "update", "--name", "renamed", "vol-def")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["name"] != "renamed" {
		t.Errorf("expected name=renamed, got %v", received["name"])
	}
}

func TestVolumesUpdate_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "volumes", "update")
	assertError(t, err)
}

func TestVolumesUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "volumes", "update", "vol-def")
	assertError(t, err)
}

func TestVolumesUpdate_size(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/volumes/vol-def", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureVolume())(w, r)
	})

	_, err := runArgs(t, ts, "volumes", "update", "--size", "100", "vol-def")
	assertNoError(t, err)
	if received["sizeGb"].(float64) != 100 {
		t.Errorf("expected sizeGb=100, got %v", received["sizeGb"])
	}
}

func TestVolumesDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/volumes/vol-def", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "volumes", "delete", "vol-def")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestVolumesDelete_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "volumes", "delete")
	assertError(t, err)
}

func TestVolumesDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/volumes/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "volumes", "delete", "bad-id")
	assertError(t, err)
}
