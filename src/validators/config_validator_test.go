package validators

import (
	"strings"
	"testing"
)

func validateConfig(t *testing.T, value map[string]interface{}) {
	t.Helper()
	if _, errors := ConfigValidator.Validate(value); len(errors) != 0 {
		t.Fatalf("expected valid config, got %v", errors)
	}
}

func assertConfigFieldError(t *testing.T, value map[string]interface{}, field string) {
	t.Helper()
	if _, errors := ConfigValidator.Validate(value); errors[field] == "" {
		t.Fatalf("expected error for %s, got %v", field, errors)
	}
}

func TestConfigValidatorAcceptsEmptyAndMinimalConfigs(t *testing.T) {
	for name, value := range map[string]map[string]interface{}{
		"empty":             {},
		"schema only":       {"$schema": "https://futrou.com/futrou.schema.json"},
		"selectors only":    {"workspace": "workspace-1", "project": "project-1"},
		"empty collections": {"serverlets": []interface{}{}, "dns": []interface{}{}, "proxies": []interface{}{}, "storages": []interface{}{}, "crons": []interface{}{}},
	} {
		t.Run(name, func(t *testing.T) { validateConfig(t, value) })
	}
}

func TestConfigValidatorRejectsInvalidTopLevelValues(t *testing.T) {
	longName := string(make([]byte, 256))
	for name, test := range map[string]struct {
		value map[string]interface{}
		field string
	}{
		"invalid schema URL":   {map[string]interface{}{"$schema": "not a URL"}, "$schema"},
		"workspace is object":  {map[string]interface{}{"workspace": map[string]interface{}{}}, "workspace"},
		"project is number":    {map[string]interface{}{"project": float64(1)}, "project"},
		"workspace too long":   {map[string]interface{}{"workspace": longName}, "workspace"},
		"serverlets is object": {map[string]interface{}{"serverlets": map[string]interface{}{}}, "serverlets"},
		"proxies is string":    {map[string]interface{}{"proxies": "proxy"}, "proxies"},
		"storages is number":   {map[string]interface{}{"storages": float64(1)}, "storages"},
		"crons is boolean":     {map[string]interface{}{"crons": true}, "crons"},
		"dns is null":          {map[string]interface{}{"dns": []interface{}{nil}}, "dns"},
	} {
		t.Run(name, func(t *testing.T) { assertConfigFieldError(t, test.value, test.field) })
	}
}

func TestConfigValidatorServerletValidation(t *testing.T) {
	valid := map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{
		"id": "sl-1", "name": "api", "displayName": "API", "image": "example/api:v1", "status": "ready", "serverletPlanId": "plan-1", "workspaceId": "ws-1", "projectId": "project-1", "regionId": "region-1",
	}}}
	validateConfig(t, valid)
	assertConfigFieldError(t, map[string]interface{}{"serverlets": []interface{}{true}}, "serverlets")
}

func TestConfigValidatorProxyValidation(t *testing.T) {
	validateConfig(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{
		"id": "px-1", "domain": "example.com", "type": "http", "target": "api:8080", "port": float64(443), "enforceHttps": true, "followRedirects": false, "preserveHeaders": true, "preserveHost": true, "preservePath": false, "preserveQuery": true, "verifyTls": true, "strategy": "round-robin", "status": "active",
	}}})

	for name, proxy := range map[string]map[string]interface{}{
		"invalid type":     {"type": "smtp"},
		"invalid strategy": {"strategy": "round_robin"},
		"zero port":        {"port": float64(0)},
		"port too high":    {"port": float64(65536)},
		"decimal port":     {"port": float64(443.5)},
		"bad boolean":      {"enforceHttps": float64(2)},
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"proxies": []interface{}{proxy}}, "proxies")
		})
	}
}

func TestConfigValidatorDNSValidation(t *testing.T) {
	validateConfig(t, map[string]interface{}{"dns": []interface{}{map[string]interface{}{
		"id": "dns-1", "name": "example.com", "domain": "example.com", "ttl": float64(300), "priority": float64(10), "workspaceId": "ws-1", "projectId": "project-1", "records": []interface{}{map[string]interface{}{"name": "www", "type": "A", "value": "192.0.2.1", "ttl": float64(60), "priority": float64(0)}},
	}}})

	for name, dns := range map[string]map[string]interface{}{
		"negative TTL":         {"ttl": float64(-1)},
		"decimal TTL":          {"ttl": float64(1.5)},
		"priority too high":    {"priority": float64(65536)},
		"record is not object": {"records": []interface{}{true}},
		"record decimal TTL":   {"records": []interface{}{map[string]interface{}{"ttl": float64(1.5)}}},
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"dns": []interface{}{dns}}, "dns")
		})
	}
}

func TestConfigValidatorStorageAndCronValidation(t *testing.T) {
	validateConfig(t, map[string]interface{}{
		"storages": []interface{}{map[string]interface{}{"id": "storage-1", "name": "data", "storagePlanId": "plan-1", "workspaceId": "ws-1", "projectId": "project-1", "regionId": "region-1", "public": false}},
		"crons":    []interface{}{map[string]interface{}{"id": "cron-1", "name": "cleanup", "enabled": true, "method": "POST", "url": "https://example.com/cleanup", "schedule": "0 * * * *", "headers": map[string]interface{}{"Authorization": "Bearer token"}, "createdAt": "2026-01-01T00:00:00Z", "startedAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"}},
	})

	assertConfigFieldError(t, map[string]interface{}{"storages": []interface{}{map[string]interface{}{"public": "not-boolean"}}}, "storages")
	assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{"url": "not a URL"}}}, "crons")
	assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{"createdAt": "not a date"}}}, "crons")
}

func TestConfigValidatorServerletMetadataFieldTypes(t *testing.T) {
	validFields := map[string]interface{}{
		"name": "web", "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z", "status": "ready", "serverletPlanId": "plan-1", "workspaceId": "workspace-1", "projectId": "project-1", "regionId": "region-1", "env": map[string]interface{}{"LOG_LEVEL": "info"}, "mounts": map[string]interface{}{"/data": "storage-1"}, "ports": []interface{}{}, "scaling": map[string]interface{}{},
	}
	validateConfig(t, map[string]interface{}{"serverlets": []interface{}{validFields}})

	for field, value := range map[string]interface{}{
		"createdAt":       float64(1),
		"updatedAt":       true,
		"status":          float64(1),
		"serverletPlanId": true,
		"workspaceId":     float64(1),
		"projectId":       false,
		"regionId":        float64(1),
	} {
		t.Run(field+" rejects wrong type", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{field: value}}}, "serverlets")
		})
	}
}

func TestConfigValidatorProxyAPIFields(t *testing.T) {
	valid := map[string]interface{}{
		"id": "proxy-1", "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z", "domain": "example.com", "type": "tcp", "target": "127.0.0.1:8080", "port": float64(8080), "status": "active", "strategy": "round-robin", "enforceHttps": false, "followRedirects": true, "preserveHeaders": false, "preserveHost": true, "preservePath": false, "preserveQuery": true, "verifyTls": false, "isVerified": true, "proxyPlanId": "plan-1", "regionId": "region-1",
	}
	validateConfig(t, map[string]interface{}{"proxies": []interface{}{valid}})

	for _, field := range []string{"enforceHttps", "followRedirects", "preserveHeaders", "preserveHost", "preservePath", "preserveQuery", "verifyTls", "isVerified"} {
		t.Run(field+" rejects invalid boolean", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{field: float64(2)}}}, "proxies")
		})
	}
	for field, value := range map[string]interface{}{"status": float64(1), "strategy": true, "createdAt": false, "updatedAt": float64(1), "target": true} {
		t.Run(field+" rejects wrong type", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{field: value}}}, "proxies")
		})
	}
}

func TestConfigValidatorDNSRecordFieldTypes(t *testing.T) {
	validRecord := map[string]interface{}{"id": "record-1", "name": "www", "displayName": "WWW", "domain": "example.com", "type": "A", "value": "192.0.2.1", "ttl": float64(60), "priority": float64(0)}
	validateConfig(t, map[string]interface{}{"dns": []interface{}{map[string]interface{}{"domain": "example.com", "records": []interface{}{validRecord}}}})

	for field, value := range map[string]interface{}{
		"id": float64(1), "name": false, "displayName": float64(1), "domain": true, "type": float64(1), "value": false, "ttl": float64(-1), "priority": float64(-1),
	} {
		t.Run(field+" rejects invalid record field", func(t *testing.T) {
			record := map[string]interface{}{field: value}
			assertConfigFieldError(t, map[string]interface{}{"dns": []interface{}{map[string]interface{}{"records": []interface{}{record}}}}, "dns")
		})
	}
}

func TestConfigValidatorStorageAndCronFieldTypes(t *testing.T) {
	for field, value := range map[string]interface{}{
		"id": float64(1), "name": false, "displayName": float64(1), "domain": true, "storagePlanId": float64(1), "workspaceId": false, "projectId": float64(1), "regionId": true, "public": "maybe", "createdAt": float64(1), "updatedAt": true,
	} {
		t.Run("storage "+field+" rejects wrong value", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"storages": []interface{}{map[string]interface{}{field: value}}}, "storages")
		})
	}

	for field, value := range map[string]interface{}{
		"id": float64(1), "name": false, "body": float64(1), "code": false, "cronPlanId": float64(1), "enabled": float64(2), "method": true, "projectId": false, "regionId": float64(1), "schedule": true, "url": "invalid", "workspaceId": false, "createdAt": "invalid", "startedAt": "invalid", "updatedAt": "invalid",
	} {
		t.Run("cron "+field+" rejects wrong value", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{field: value}}}, "crons")
		})
	}
}

func TestConfigValidator(t *testing.T) {
	_, errors := ConfigValidator.Validate(map[string]interface{}{
		"serverlets": []interface{}{map[string]interface{}{
			"name": "api", "image": "example/api", "serverletPlanId": "plan-1",
		}},
		"proxies": []interface{}{map[string]interface{}{"domain": "example.com", "port": float64(443)}},
	})
	if len(errors) != 0 {
		t.Fatalf("expected config to validate, got %v", errors)
	}
}

func TestConfigJSONSchemaIncludesDescriptions(t *testing.T) {
	schema := ConfigValidator.ToJSONSchema()
	if schema["description"] == "" {
		t.Fatal("expected root schema description")
	}
	properties := schema["properties"].(map[string]interface{})
	if properties["serverlets"].(map[string]interface{})["description"] == "" {
		t.Fatal("expected serverlets description")
	}
	serverletItems := properties["serverlets"].(map[string]interface{})["items"].(map[string]interface{})
	serverletProperties := serverletItems["properties"].(map[string]interface{})
	if serverletProperties["image"].(map[string]interface{})["description"] == "" {
		t.Fatal("expected serverlet image description")
	}
}

func TestConfigValidatorReportsEveryTopLevelError(t *testing.T) {
	_, errors := ConfigValidator.Validate(map[string]interface{}{
		"workspace":  float64(1),
		"project":    float64(2),
		"serverlets": "not-an-array",
		"proxies":    []interface{}{map[string]interface{}{"port": float64(70000)}},
		"dns":        []interface{}{map[string]interface{}{"ttl": float64(1.5)}},
	})
	for _, field := range []string{"workspace", "project", "serverlets", "proxies", "dns"} {
		if _, ok := errors[field]; !ok {
			t.Errorf("expected error for %s, got %v", field, errors)
		}
	}
}

func TestConfigValidatorRejectsInvalidArrayItems(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value interface{}
	}{
		{"serverlet must be an object", "serverlets", []interface{}{true}},
		{"proxy port must be positive", "proxies", []interface{}{map[string]interface{}{"port": float64(0)}}},
		{"dns ttl must be integer", "dns", []interface{}{map[string]interface{}{"ttl": float64(1.5)}}},
		{"storage array items must be objects", "storages", []interface{}{"data"}},
		{"cron array items must be objects", "crons", []interface{}{float64(1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, errors := ConfigValidator.Validate(map[string]interface{}{test.field: test.value})
			if _, ok := errors[test.field]; !ok {
				t.Fatalf("expected error for %s, got %v", test.field, errors)
			}
		})
	}
}

func TestConfigValidatorUsesWorkspaceAndProjectStringSelectors(t *testing.T) {
	_, errors := ConfigValidator.Validate(map[string]interface{}{
		"workspace": "team",
		"project":   "api",
	})
	if len(errors) != 0 {
		t.Fatalf("expected valid workspace/project selectors, got %v", errors)
	}

	_, errors = ConfigValidator.Validate(map[string]interface{}{
		"workspace": map[string]interface{}{"name": "team"},
	})
	if len(errors) == 0 {
		t.Fatal("expected non-string workspace selector to fail")
	}
}

func TestConfigValidatorWarnsAboutUnexpectedKeys(t *testing.T) {
	_, errors, warnings := ConfigValidator.ValidateWithWarnings(map[string]interface{}{
		"futureField": true,
		"serverlets":  []interface{}{map[string]interface{}{"name": "api", "futureField": true}},
	})
	if len(errors) != 0 {
		t.Fatalf("unexpected validation errors: %v", errors)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected two warnings, got %v", warnings)
	}
}

func TestConfigValidatorAcceptsGeneratedAPIResourceFields(t *testing.T) {
	_, errors, warnings := ConfigValidator.ValidateWithWarnings(map[string]interface{}{
		"serverlets": []interface{}{map[string]interface{}{
			"name": "api", "image": "example/api", "serverletPlanId": "plan-1", "status": "ready", "regionId": "region-1",
		}},
		"proxies": []interface{}{map[string]interface{}{
			"domain": "example.com", "type": "http", "target": "api:8080", "port": float64(443), "enforceHttps": true, "verifyTls": true,
		}},
		"storages": []interface{}{map[string]interface{}{"name": "data", "storagePlanId": "plan-1"}},
		"crons":    []interface{}{map[string]interface{}{"name": "cleanup", "enabled": true, "url": "https://example.com/cleanup", "createdAt": "2026-01-01T00:00:00Z"}},
	})
	if len(errors) != 0 {
		t.Fatalf("unexpected validation errors: %v", errors)
	}
	if len(warnings) != 0 {
		t.Fatalf("generated API fields should not warn: %v", warnings)
	}
}

func TestConfigValidatorServerletConfigShapes(t *testing.T) {
	serverlet := func(fields map[string]interface{}) map[string]interface{} {
		fields["name"] = "web"
		return map[string]interface{}{"serverlets": []interface{}{fields}}
	}
	validateConfig(t, serverlet(map[string]interface{}{
		"ports":      []interface{}{map[string]interface{}{"port": float64(8080), "protocol": "tcp"}},
		"networks":   []interface{}{"backend"},
		"entrypoint": "node server.js",
		"scaling":    map[string]interface{}{"min": float64(1)},
	}))

	for name, fields := range map[string]map[string]interface{}{
		"port out of range":        {"ports": []interface{}{map[string]interface{}{"port": float64(70000)}}},
		"port missing":             {"ports": []interface{}{map[string]interface{}{"protocol": "tcp"}}},
		"unknown protocol":         {"ports": []interface{}{map[string]interface{}{"port": float64(80), "protocol": "icmp"}}},
		"networks not strings":     {"networks": []interface{}{float64(1)}},
		"networks empty string":    {"networks": []interface{}{" "}},
		"networks not array":       {"networks": "backend"},
		"entrypoint not strings":   {"entrypoint": []interface{}{true}},
		"entrypoint empty":         {"entrypoint": " "},
		"entrypoint number":        {"entrypoint": float64(1)},
		"env value not string":     {"env": map[string]interface{}{"PORT": float64(80)}},
		"mounts value not string":  {"mounts": map[string]interface{}{"/data": float64(1)}},
		"scaling value not number": {"scaling": map[string]interface{}{"min": "one"}},
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigFieldError(t, serverlet(fields), "serverlets")
		})
	}
}

func TestConfigValidatorNetworks(t *testing.T) {
	validateConfig(t, map[string]interface{}{"networks": []interface{}{
		map[string]interface{}{"name": "backend", "ipv4": "10.0.0.0/24", "internal": true, "regionId": "region-1"},
	}})
	assertConfigFieldError(t, map[string]interface{}{"networks": []interface{}{map[string]interface{}{"name": "a", "internal": "maybe"}}}, "networks")
	assertConfigFieldError(t, map[string]interface{}{"networks": "backend"}, "networks")
}

func TestConfigValidatorRequiresUniqueResourceNames(t *testing.T) {
	for field, key := range map[string]string{"serverlets": "name", "networks": "name", "storages": "name", "crons": "name", "proxies": "domain", "dns": "domain"} {
		t.Run(field, func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{field: []interface{}{map[string]interface{}{}}}, field)
			assertConfigFieldError(t, map[string]interface{}{field: []interface{}{
				map[string]interface{}{key: "same"}, map[string]interface{}{key: "same"},
			}}, field)
			validateConfig(t, map[string]interface{}{field: []interface{}{
				map[string]interface{}{key: "one"}, map[string]interface{}{key: "two"},
			}})
		})
	}
}

func TestConfigValidatorServerletRuntimeOptions(t *testing.T) {
	serverlet := func(fields map[string]interface{}) map[string]interface{} {
		fields["name"] = "web"
		return map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{"name": "db"}, fields}}
	}
	validateConfig(t, serverlet(map[string]interface{}{
		"minInstances": float64(1), "maxInstances": float64(3), "user": "node", "needs": []interface{}{"db"},
		"healthcheck": "curl -f localhost", "healthcheckInterval": float64(30), "healthcheckTimeout": float64(5),
		"healthcheckRetries": float64(3), "startTimeout": float64(60), "stopTimeout": float64(30),
		"strategy": map[string]interface{}{"type": "rolling"},
	}))
	for name, fields := range map[string]map[string]interface{}{
		"negative min":        {"minInstances": float64(-1)},
		"fractional max":      {"maxInstances": 1.5},
		"user not string":     {"user": float64(1)},
		"needs not strings":   {"needs": []interface{}{float64(1)}},
		"healthcheck number":  {"healthcheck": float64(1)},
		"negative interval":   {"healthcheckInterval": float64(-1)},
		"retries not integer": {"healthcheckRetries": 1.5},
		"timeout not number":  {"startTimeout": "soon"},
		"stop timeout bad":    {"stopTimeout": "soon"},
		"strategy not object": {"strategy": "rolling"},
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigFieldError(t, serverlet(fields), "serverlets")
		})
	}
}

func TestConfigValidatorProxyVariantRulesAndCronType(t *testing.T) {
	validateConfig(t, map[string]interface{}{
		"proxies": []interface{}{map[string]interface{}{"domain": "example.com", "variant": "non-www", "rules": []interface{}{}}},
		"crons":   []interface{}{map[string]interface{}{"name": "job", "type": "code"}},
	})
	assertConfigFieldError(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{"domain": "example.com", "variant": "sometimes"}}}, "proxies")
	assertConfigFieldError(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{"domain": "example.com", "rules": "none"}}}, "proxies")
	assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{"name": "job", "type": "shell"}}}, "crons")
}

func TestConfigValidatorEntrypointStringOrArray(t *testing.T) {
	serverlet := func(entrypoint interface{}) map[string]interface{} {
		return map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{"name": "web", "entrypoint": entrypoint}}}
	}
	validateConfig(t, serverlet("node server.js"))
	validateConfig(t, serverlet("/usr/local/bin/start"))
	validateConfig(t, serverlet([]interface{}{"node", "server.js"}))
	assertConfigFieldError(t, serverlet("  "), "serverlets")
	assertConfigFieldError(t, serverlet(float64(1)), "serverlets")
	assertConfigFieldError(t, serverlet([]interface{}{"node", float64(1)}), "serverlets")
}

func TestConfigValidatorRegion(t *testing.T) {
	validateConfig(t, map[string]interface{}{"region": "europe"})
	validateConfig(t, map[string]interface{}{"region": "019b1334-dabc-7000-8000-000000000001"})
	assertConfigFieldError(t, map[string]interface{}{"region": float64(1)}, "region")
	assertConfigFieldError(t, map[string]interface{}{"region": []interface{}{"europe"}}, "region")
	assertConfigFieldError(t, map[string]interface{}{"region": strings.Repeat("x", 256)}, "region")
}

func TestParseNeed(t *testing.T) {
	for need, want := range map[string][2]string{
		"db": {"db", ""}, "db:healthy": {"db", "healthy"}, "db:started": {"db", "started"}, " db : healthy ": {"db", "healthy"},
	} {
		name, status, err := ParseNeed(need)
		if err != nil || name != want[0] || status != want[1] {
			t.Errorf("ParseNeed(%q) = %q, %q, %v; want %v", need, name, status, err, want)
		}
	}
	for _, bad := range []string{"", ":healthy", "db:", "db:ready", "db:healthy:extra", "db:HEALTHY"} {
		if _, _, err := ParseNeed(bad); err == nil {
			t.Errorf("ParseNeed(%q) should fail", bad)
		}
	}
}

func TestConfigValidatorNeeds(t *testing.T) {
	serverlets := func(specs ...map[string]interface{}) map[string]interface{} {
		items := make([]interface{}, len(specs))
		for i, spec := range specs {
			items[i] = spec
		}
		return map[string]interface{}{"serverlets": items}
	}
	sl := func(name string, needs ...interface{}) map[string]interface{} {
		spec := map[string]interface{}{"name": name}
		if len(needs) > 0 {
			spec["needs"] = needs
		}
		return spec
	}
	validateConfig(t, serverlets(sl("db"), sl("cache"), sl("web", "db", "cache:started"), sl("worker", "db:healthy", "web")))

	for name, config := range map[string]map[string]interface{}{
		"undeclared serverlet": serverlets(sl("web", "db")),
		"needs itself":         serverlets(sl("web", "web")),
		"direct cycle":         serverlets(sl("a", "b"), sl("b", "a")),
		"longer cycle":         serverlets(sl("a", "b"), sl("b", "c"), sl("c", "a:healthy")),
		"unknown status":       serverlets(sl("db"), sl("web", "db:ready")),
		"empty name":           serverlets(sl("db"), sl("web", ":healthy")),
		"not strings":          serverlets(sl("db"), sl("web", float64(1))),
		"not an array":         serverlets(sl("db"), map[string]interface{}{"name": "web", "needs": "db"}),
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigFieldError(t, config, "serverlets")
		})
	}
}

func TestConfigValidatorNeedsErrorsAreSpecific(t *testing.T) {
	for want, config := range map[string]map[string]interface{}{
		"which is not declared": {"serverlets": []interface{}{map[string]interface{}{"name": "web", "needs": []interface{}{"db"}}}},
		"cannot need itself":    {"serverlets": []interface{}{map[string]interface{}{"name": "web", "needs": []interface{}{"web"}}}},
		"form a cycle":          {"serverlets": []interface{}{map[string]interface{}{"name": "a", "needs": []interface{}{"b"}}, map[string]interface{}{"name": "b", "needs": []interface{}{"a"}}}},
		"unknown status":        {"serverlets": []interface{}{map[string]interface{}{"name": "db"}, map[string]interface{}{"name": "web", "needs": []interface{}{"db:ready"}}}},
	} {
		_, errs, _ := ConfigValidator.ValidateWithWarnings(config)
		if message := errs["serverlets"]; !strings.Contains(message, want) {
			t.Errorf("error %q should mention %q", message, want)
		}
	}
}

func TestValidateNeedsIgnoresShapesOtherRulesReport(t *testing.T) {
	for name, value := range map[string]interface{}{
		"not an array":       "nope",
		"items not objects":  []interface{}{"a", 1, nil},
		"unnamed serverlets": []interface{}{map[string]interface{}{"needs": []interface{}{"x"}}},
		"non-string needs":   []interface{}{map[string]interface{}{"name": "a", "needs": []interface{}{5}}},
		"needs not an array": []interface{}{map[string]interface{}{"name": "a", "needs": "b"}},
		"no needs":           []interface{}{map[string]interface{}{"name": "a"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateNeeds(value); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	err := validateNeeds([]interface{}{map[string]interface{}{"name": "a", "needs": []interface{}{"b:later"}}})
	if err == nil || !strings.Contains(err.Error(), `serverlet "a"`) {
		t.Fatalf("a malformed need must name the serverlet: %v", err)
	}
}

func TestListAndMapHelpers(t *testing.T) {
	for name, tc := range map[string]struct {
		fn      func(interface{}) error
		value   interface{}
		wantErr string
	}{
		"list ok":           {stringList, []interface{}{"a", "b"}, ""},
		"list empty ok":     {stringList, []interface{}{}, ""},
		"list wrong type":   {stringList, "a", "must be an array"},
		"list non-string":   {stringList, []interface{}{"a", 2}, "item 1"},
		"list blank item":   {stringList, []interface{}{"  "}, "item 0"},
		"string map ok":     {stringMap, map[string]interface{}{"A": "1"}, ""},
		"string map type":   {stringMap, []interface{}{}, "must be an object"},
		"string map value":  {stringMap, map[string]interface{}{"PORT": 80}, "PORT must be a string"},
		"number map ok":     {numberMap, map[string]interface{}{"cpu": 1.5}, ""},
		"number map type":   {numberMap, "x", "must be an object"},
		"number map value":  {numberMap, map[string]interface{}{"cpu": "1"}, "cpu must be a number"},
		"unique non-array":  {uniqueNames("name"), "x", ""},
		"unique ok":         {uniqueNames("name"), []interface{}{map[string]interface{}{"name": "a"}, map[string]interface{}{"name": "b"}}, ""},
		"unique duplicate":  {uniqueNames("name"), []interface{}{map[string]interface{}{"name": "a"}, map[string]interface{}{"name": "a"}}, "duplicate"},
		"unique missing":    {uniqueNames("name"), []interface{}{map[string]interface{}{}}, "name"},
		"unique skips junk": {uniqueNames("name"), []interface{}{"junk", map[string]interface{}{"name": "a"}}, ""},
		"unique other key":  {uniqueNames("domain", "name"), []interface{}{map[string]interface{}{"name": "a"}, map[string]interface{}{"domain": "a"}}, "duplicate"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.fn(tc.value)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestConfigValidatorRejectsWrongShapesPerResource(t *testing.T) {
	item := func(m map[string]interface{}) []interface{} { return []interface{}{m} }
	for name, tc := range map[string]struct {
		config map[string]interface{}
		field  string
	}{
		"serverlets not an array": {map[string]interface{}{"serverlets": "web"}, "serverlets"},
		"serverlet not an object": {map[string]interface{}{"serverlets": []interface{}{"web"}}, "serverlets"},
		"networks not an array":   {map[string]interface{}{"networks": map[string]interface{}{"a": 1}}, "networks"},
		"network name number":     {map[string]interface{}{"networks": item(map[string]interface{}{"name": 5})}, "networks"},
		"storages not an array":   {map[string]interface{}{"storages": 5}, "storages"},
		"storage name blank":      {map[string]interface{}{"storages": item(map[string]interface{}{"name": ""})}, "storages"},
		"crons not an array":      {map[string]interface{}{"crons": "x"}, "crons"},
		"cron name missing":       {map[string]interface{}{"crons": item(map[string]interface{}{"schedule": "* * * * *"})}, "crons"},
		"proxies not an array":    {map[string]interface{}{"proxies": true}, "proxies"},
		"proxy domain missing":    {map[string]interface{}{"proxies": item(map[string]interface{}{"target": "web"})}, "proxies"},
		"dns not an array":        {map[string]interface{}{"dns": "example.com"}, "dns"},
		"dns domain missing":      {map[string]interface{}{"dns": item(map[string]interface{}{"records": []interface{}{}})}, "dns"},
		"dns records not array":   {map[string]interface{}{"dns": item(map[string]interface{}{"domain": "a.com", "name": "a.com", "records": "x"})}, "dns"},
		"locks not an object":     {map[string]interface{}{"locks": []interface{}{}}, "locks"},
		"locks value not string":  {map[string]interface{}{"locks": map[string]interface{}{"serverlets.web": 1}}, "locks"},
		"template not an object":  {map[string]interface{}{"template": "nice"}, "template"},
		"region not a string":     {map[string]interface{}{"region": []interface{}{}}, "region"},
		"project not a string":    {map[string]interface{}{"project": 5}, "project"},
		"workspace not a string":  {map[string]interface{}{"workspace": 5}, "workspace"},
	} {
		t.Run(name, func(t *testing.T) { assertConfigFieldError(t, tc.config, tc.field) })
	}
}

func TestConfigValidatorAcceptsEveryResourceShape(t *testing.T) {
	validateConfig(t, map[string]interface{}{
		"workspace": "acme", "project": "demo", "region": "europe",
		"networks": []interface{}{map[string]interface{}{"name": "backend", "internal": true, "ipv4": "10.0.0.0/24"}},
		"storages": []interface{}{map[string]interface{}{"name": "data", "public": false}},
		"crons":    []interface{}{map[string]interface{}{"name": "nightly", "schedule": "0 3 * * *", "type": "simple", "url": "https://example.com/hook", "method": "POST", "enabled": true}},
		"proxies":  []interface{}{map[string]interface{}{"domain": "app.example.com", "target": "web", "port": float64(8080)}},
		"dns":      []interface{}{map[string]interface{}{"domain": "example.com", "name": "example.com", "records": []interface{}{map[string]interface{}{"name": "@", "type": "A", "value": "192.0.2.1", "ttl": float64(300)}}}},
		"serverlets": []interface{}{
			map[string]interface{}{"name": "db", "image": "postgres:17"},
			map[string]interface{}{"name": "web", "image": "nginx", "needs": []interface{}{"db:healthy"}, "ports": []interface{}{map[string]interface{}{"port": float64(80)}}},
		},
		"locks": map[string]interface{}{"serverlets.web": "abc"},
	})
}
