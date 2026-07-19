package commands

import (
	"net/http"
	"testing"
)

func TestDNSList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns", respond(200, []interface{}{fixtureDNSZone()}))

	out, err := runArgs(t, ts, "dns", "list")
	assertNoError(t, err)
	assertContains(t, out, "dns-789")
	assertContains(t, out, "example.com")
}

func TestDNSList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "dns", "list")
	assertError(t, err)
}

func TestDNSGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dns-789", respond(200, fixtureDNSZone()))

	out, err := runArgs(t, ts, "dns", "get", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "dns-789")
}

func TestDNSGet_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "get")
	assertError(t, err)
}

func TestDNSGet_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "dns", "get", "bad-id")
	assertError(t, err)
}

func TestDNSCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/dns", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureDNSZone())(w, r)
	})

	out, err := runArgs(t, ts, "dns", "create", "--name", "example.com")
	assertNoError(t, err)
	assertContains(t, out, "created")
	if received["name"] != "example.com" {
		t.Errorf("expected name=example.com, got %v", received["name"])
	}
}

func TestDNSCreate_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "dns", "create", "--name", "x.com")
	assertError(t, err)
}

func TestDNSUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/dns/dns-789", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureDNSZone())(w, r)
	})

	out, err := runArgs(t, ts, "dns", "update", "--name", "new.com", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["name"] != "new.com" {
		t.Errorf("expected name=new.com, got %v", received["name"])
	}
}

func TestDNSUpdate_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "update")
	assertError(t, err)
}

func TestDNSUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "update", "dns-789")
	assertError(t, err)
}

func TestDNSDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/dns/dns-789", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "dns", "delete", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestDNSDelete_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "delete")
	assertError(t, err)
}

func TestDNSDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/dns/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "dns", "delete", "bad-id")
	assertError(t, err)
}

func TestDNSLogs(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dns-789/logs", respond(200, []interface{}{map[string]string{"message": "query received"}}))

	out, err := runArgs(t, ts, "dns", "logs", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "query received")
}

func TestDNSLogs_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "logs")
	assertError(t, err)
}

func TestDNSLogs_withFlags(t *testing.T) {
	ts := newTestServer(t)
	var gotQuery string
	ts.on("GET", "/v2/dns/dns-789/logs", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		respond(200, []interface{}{})(w, r)
	})

	_, err := runArgs(t, ts, "dns", "logs", "--offset", "10", "--limit", "50", "--search", "error", "dns-789")
	assertNoError(t, err)
	assertContains(t, gotQuery, "offset=10")
	assertContains(t, gotQuery, "limit=50")
	assertContains(t, gotQuery, "search=error")
}

func TestDNSLogsTail(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dns-789/logs/tail", respond(200, []interface{}{map[string]string{"message": "recent entry"}}))

	out, err := runArgs(t, ts, "dns", "logs", "tail", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "recent entry")
}

func TestDNSLogsTail_missingID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "logs", "tail")
	assertError(t, err)
}

// --- DNS Records ---

func TestDNSRecordsList(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dns-789/records", respond(200, []interface{}{fixtureDNSRecord()}))

	out, err := runArgs(t, ts, "dns", "records", "list", "dns-789")
	assertNoError(t, err)
	assertContains(t, out, "rec-001")
	assertContains(t, out, "1.2.3.4")
}

func TestDNSRecordsList_missingZoneID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "list")
	assertError(t, err)
}

func TestDNSRecordsList_requiresAuth(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgsNoAuth(t, ts, "dns", "records", "list", "dns-789")
	assertError(t, err)
}

func TestDNSRecordsGet(t *testing.T) {
	ts := newTestServer(t)
	ts.on("GET", "/v2/dns/dns-789/records/rec-001", respond(200, fixtureDNSRecord()))

	out, err := runArgs(t, ts, "dns", "records", "get", "dns-789", "rec-001")
	assertNoError(t, err)
	assertContains(t, out, "rec-001")
	assertContains(t, out, "1.2.3.4")
}

func TestDNSRecordsGet_missingArgs(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "get", "dns-789")
	assertError(t, err)
}

func TestDNSRecordsCreate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/dns/dns-789/records", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureDNSRecord())(w, r)
	})

	out, err := runArgs(t, ts, "dns", "records", "create",
		"--name", "www",
		"--type", "A",
		"--value", "1.2.3.4",
		"--ttl", "600",
		"dns-789",
	)
	assertNoError(t, err)
	assertContains(t, out, "created")

	if received["name"] != "www" {
		t.Errorf("expected name=www, got %v", received["name"])
	}
	if received["type"] != "A" {
		t.Errorf("expected type=A, got %v", received["type"])
	}
	if received["value"] != "1.2.3.4" {
		t.Errorf("expected value=1.2.3.4, got %v", received["value"])
	}
	if received["ttl"].(float64) != 600 {
		t.Errorf("expected ttl=600, got %v", received["ttl"])
	}
}

func TestDNSRecordsCreate_withPriority(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("POST", "/v2/dns/dns-789/records", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(201, fixtureDNSRecord())(w, r)
	})

	_, err := runArgs(t, ts, "dns", "records", "create",
		"--name", "mail",
		"--type", "MX",
		"--value", "mail.example.com",
		"--priority", "10",
		"dns-789",
	)
	assertNoError(t, err)
	if received["priority"].(float64) != 10 {
		t.Errorf("expected priority=10, got %v", received["priority"])
	}
}

func TestDNSRecordsCreate_missingZoneID(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "create",
		"--name", "www", "--type", "A", "--value", "1.2.3.4")
	assertError(t, err)
}

func TestDNSRecordsUpdate(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/dns/dns-789/records/rec-001", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureDNSRecord())(w, r)
	})

	out, err := runArgs(t, ts, "dns", "records", "update",
		"--value", "5.6.7.8",
		"dns-789", "rec-001",
	)
	assertNoError(t, err)
	assertContains(t, out, "updated")
	if received["value"] != "5.6.7.8" {
		t.Errorf("expected value=5.6.7.8, got %v", received["value"])
	}
}

func TestDNSRecordsUpdate_missingArgs(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "update", "dns-789")
	assertError(t, err)
}

func TestDNSRecordsUpdate_noFields(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "update", "dns-789", "rec-001")
	assertError(t, err)
}

func TestDNSRecordsUpdate_ttl(t *testing.T) {
	ts := newTestServer(t)
	var received map[string]interface{}
	ts.on("PATCH", "/v2/dns/dns-789/records/rec-001", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(r, &received)
		respond(200, fixtureDNSRecord())(w, r)
	})

	_, err := runArgs(t, ts, "dns", "records", "update", "--ttl", "3600", "dns-789", "rec-001")
	assertNoError(t, err)
	if received["ttl"].(float64) != 3600 {
		t.Errorf("expected ttl=3600, got %v", received["ttl"])
	}
}

func TestDNSRecordsDelete(t *testing.T) {
	ts := newTestServer(t)
	called := false
	ts.on("DELETE", "/v2/dns/dns-789/records/rec-001", func(w http.ResponseWriter, r *http.Request) {
		called = true
		respondEmpty(w, r)
	})

	out, err := runArgs(t, ts, "dns", "records", "delete", "dns-789", "rec-001")
	assertNoError(t, err)
	assertContains(t, out, "deleted")
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestDNSRecordsDelete_missingArgs(t *testing.T) {
	ts := newTestServer(t)
	_, err := runArgs(t, ts, "dns", "records", "delete", "dns-789")
	assertError(t, err)
}

func TestDNSRecordsDelete_apiError(t *testing.T) {
	ts := newTestServer(t)
	ts.on("DELETE", "/v2/dns/dns-789/records/bad-id", respond(404, fixtureAPIError("not found")))

	_, err := runArgs(t, ts, "dns", "records", "delete", "dns-789", "bad-id")
	assertError(t, err)
}
