package commands

import (
	"net/http"
	"testing"
)

func TestProxiesList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies", respond(200, []interface{}{fixtureProxy()}))

	out, err := runArgs(t, ts, "proxies", "list")
	assertNoError(t, err)
	assertContains(t, out, "px-456")
	assertContains(t, out, "example.com")
}

func TestProxiesList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "proxies", "list")
	assertError(t, err)
}

func TestProxiesGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies/px-456", respond(200, fixtureProxy()))

	out, err := runArgs(t, ts, "proxies", "get", "px-456")
	assertNoError(t, err)
	assertContains(t, out, "px-456")
	assertContains(t, out, "example.com")
}

func TestProxiesGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "proxies", "get")
	assertError(t, err)
}

func TestProxiesGet_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/proxies/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "proxies", "get", "bad-id")
	assertError(t, err)
}

func TestProxiesCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/proxies", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProxy())(w, r)
	})

	out, err := runArgs(t, ts, "proxies", "create",
		"--domain", "example.com",
		"--target", "localhost",
		"--port", "8080",
	)
	assertNoError(t, err)
	assertContains(t, out, "created")

	if received["domain"] != "example.com" {
		t.Errorf("expected domain=example.com, got %v", received["domain"])
	}
	if received["target"] != "localhost" {
		t.Errorf("expected target=localhost, got %v", received["target"])
	}
	if received["port"].(float64) != 8080 {
		t.Errorf("expected port=8080, got %v", received["port"])
	}
}

func TestProxiesCreate_withHTTPS(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/proxies", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProxy())(w, r)
	})

	_, err := runArgs(t, ts, "proxies", "create",
		"--domain", "secure.com",
		"--target", "backend",
		"--https",
	)
	assertNoError(t, err)
	if received["enforceHttps"] != true {
		t.Errorf("expected enforceHttps=true, got %v", received["enforceHttps"])
	}
}

func TestProxiesCreate_withStrategy(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/proxies", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureProxy())(w, r)
	})

	_, err := runArgs(t, ts, "proxies", "create",
		"--domain", "lb.com",
		"--target", "backend",
		"--strategy", "round-robin",
	)
	assertNoError(t, err)
	if received["strategy"] != "round-robin" {
		t.Errorf("expected strategy=round-robin, got %v", received["strategy"])
	}
}

func TestProxiesCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "proxies", "create", "--domain", "x.com", "--target", "y")
	assertError(t, err)
}

func TestProxiesUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/proxies/px-456", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureProxy())(w, r)
	})

	out, err := runArgs(t, ts, "proxies", "update", "--domain", "new.com", "px-456")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["domain"] != "new.com" {
		t.Errorf("expected domain=new.com, got %v", received["domain"])
	}
}

func TestProxiesUpdate_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "proxies", "update")
	assertError(t, err)
}

func TestProxiesUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "proxies", "update", "px-456")
	assertError(t, err)
}

func TestProxiesUpdate_port(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/proxies/px-456", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureProxy())(w, r)
	})

	_, err := runArgs(t, ts, "proxies", "update", "--port", "9090", "px-456")
	assertNoError(t, err)
	if received["port"].(float64) != 9090 {
		t.Errorf("expected port=9090, got %v", received["port"])
	}
}

func TestProxiesDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/proxies/px-456", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "proxies", "delete", "px-456")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestProxiesDelete_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "proxies", "delete")
	assertError(t, err)
}

func TestProxiesDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/proxies/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "proxies", "delete", "bad-id")
	assertError(t, err)
}

func TestProxiesVerify(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("POST", "/v2/proxies/px-456/verify", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respond(200, map[string]string{"status": "pending"})(w, r)
	})

	out, err := runArgs(t, ts, "proxies", "verify", "px-456")
	assertNoError(t, err)
	assertContains(t, out, "Verification triggered")
	if !called {
		t.Error("POST /verify was not called")
	}
}

func TestProxiesVerify_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "proxies", "verify")
	assertError(t, err)
}
