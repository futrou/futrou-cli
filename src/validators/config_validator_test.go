package validators

import "testing"

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
		"empty collections": {"serverlets": []interface{}{}, "dns": []interface{}{}, "proxies": []interface{}{}, "volumes": []interface{}{}, "crons": []interface{}{}},
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
		"volumes is number":    {map[string]interface{}{"volumes": float64(1)}, "volumes"},
		"crons is boolean":     {map[string]interface{}{"crons": true}, "crons"},
		"dns is null":          {map[string]interface{}{"dns": []interface{}{nil}}, "dns"},
	} {
		t.Run(name, func(t *testing.T) { assertConfigFieldError(t, test.value, test.field) })
	}
}

func TestConfigValidatorServerletBoundsAndTypes(t *testing.T) {
	valid := map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{
		"id": "sl-1", "name": "api", "displayName": "API", "image": "example/api:v1", "ram": float64(32), "cpu": float64(10), "instances": float64(0), "minInstances": float64(0), "maxInstances": float64(1000), "runtime": "container", "state": "active", "networkId": "net-1", "serverletPlanId": "plan-1", "workspaceId": "ws-1", "projectId": "project-1",
	}}}
	validateConfig(t, valid)
	assertConfigFieldError(t, map[string]interface{}{"serverlets": []interface{}{true}}, "serverlets")

	for name, serverlet := range map[string]map[string]interface{}{
		"ram below minimum":     {"ram": float64(31)},
		"ram above maximum":     {"ram": float64(262145)},
		"ram decimal":           {"ram": float64(32.5)},
		"cpu below minimum":     {"cpu": float64(9)},
		"cpu above maximum":     {"cpu": float64(32001)},
		"instances decimal":     {"instances": float64(1.5)},
		"minimum above maximum": {"minInstances": float64(2), "maxInstances": float64(1)},
		"maximum too high":      {"maxInstances": float64(1001)},
	} {
		t.Run(name, func(t *testing.T) {
			var item interface{} = serverlet
			assertConfigFieldError(t, map[string]interface{}{"serverlets": []interface{}{item}}, "serverlets")
		})
	}
}

func TestConfigValidatorProxyValidation(t *testing.T) {
	validateConfig(t, map[string]interface{}{"proxies": []interface{}{map[string]interface{}{
		"id": "px-1", "domain": "example.com", "type": "http", "target": "api:8080", "port": float64(443), "compress": true, "enforceHttps": true, "followRedirects": false, "preserveHeaders": true, "preserveHost": true, "preservePath": false, "preserveQuery": true, "verifyTls": true, "strategy": "round_robin", "status": "active",
	}}})

	for name, proxy := range map[string]map[string]interface{}{
		"invalid type":  {"type": "smtp"},
		"zero port":     {"port": float64(0)},
		"port too high": {"port": float64(65536)},
		"decimal port":  {"port": float64(443.5)},
		"bad boolean":   {"compress": float64(2)},
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

func TestConfigValidatorVolumeAndCronValidation(t *testing.T) {
	validateConfig(t, map[string]interface{}{
		"volumes": []interface{}{map[string]interface{}{"id": "vol-1", "name": "data", "sizeGb": float64(10), "type": "ssd"}},
		"crons":   []interface{}{map[string]interface{}{"id": "cron-1", "name": "cleanup", "enabled": true, "method": "POST", "url": "https://example.com/cleanup", "schedule": "0 * * * *", "headers": map[string]interface{}{"Authorization": "Bearer token"}, "createdAt": "2026-01-01T00:00:00Z", "startedAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"}},
	})

	assertConfigFieldError(t, map[string]interface{}{"volumes": []interface{}{map[string]interface{}{"sizeGb": float64(-1)}}}, "volumes")
	assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{"url": "not a URL"}}}, "crons")
	assertConfigFieldError(t, map[string]interface{}{"crons": []interface{}{map[string]interface{}{"createdAt": "not a date"}}}, "crons")
}

func TestConfigValidatorServerletMetadataFieldTypes(t *testing.T) {
	validFields := map[string]interface{}{
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z", "networkId": "network-1", "runtime": "container", "state": "active", "serverletPlanId": "plan-1", "workspaceId": "workspace-1", "projectId": "project-1", "env": map[string]interface{}{"LOG_LEVEL": "info"}, "volumes": []interface{}{}, "ports": []interface{}{}, "scaling": map[string]interface{}{},
	}
	validateConfig(t, map[string]interface{}{"serverlets": []interface{}{validFields}})

	for field, value := range map[string]interface{}{
		"createdAt":       float64(1),
		"updatedAt":       true,
		"networkId":       float64(1),
		"runtime":         false,
		"state":           float64(1),
		"serverletPlanId": true,
		"workspaceId":     float64(1),
		"projectId":       false,
	} {
		t.Run(field+" rejects wrong type", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"serverlets": []interface{}{map[string]interface{}{field: value}}}, "serverlets")
		})
	}
}

func TestConfigValidatorProxyAPIFields(t *testing.T) {
	valid := map[string]interface{}{
		"id": "proxy-1", "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z", "domain": "example.com", "type": "tcp", "target": "127.0.0.1:8080", "port": float64(8080), "status": "active", "strategy": "round_robin", "compress": false, "enforceHttps": false, "followRedirects": true, "preserveHeaders": false, "preserveHost": true, "preservePath": false, "preserveQuery": true, "verifyTls": false,
	}
	validateConfig(t, map[string]interface{}{"proxies": []interface{}{valid}})

	for _, field := range []string{"compress", "enforceHttps", "followRedirects", "preserveHeaders", "preserveHost", "preservePath", "preserveQuery", "verifyTls"} {
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
	validateConfig(t, map[string]interface{}{"dns": []interface{}{map[string]interface{}{"records": []interface{}{validRecord}}}})

	for field, value := range map[string]interface{}{
		"id": float64(1), "name": false, "displayName": float64(1), "domain": true, "type": float64(1), "value": false, "ttl": float64(-1), "priority": float64(-1),
	} {
		t.Run(field+" rejects invalid record field", func(t *testing.T) {
			record := map[string]interface{}{field: value}
			assertConfigFieldError(t, map[string]interface{}{"dns": []interface{}{map[string]interface{}{"records": []interface{}{record}}}}, "dns")
		})
	}
}

func TestConfigValidatorVolumeAndCronFieldTypes(t *testing.T) {
	for field, value := range map[string]interface{}{
		"id": float64(1), "name": false, "displayName": float64(1), "domain": true, "sizeGb": float64(-0.1), "type": false, "createdAt": float64(1), "updatedAt": true,
	} {
		t.Run("volume "+field+" rejects wrong value", func(t *testing.T) {
			assertConfigFieldError(t, map[string]interface{}{"volumes": []interface{}{map[string]interface{}{field: value}}}, "volumes")
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
			"name": "api", "image": "example/api", "ram": float64(128), "cpu": float64(100), "minInstances": float64(0), "maxInstances": float64(1),
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

func TestConfigValidatorRejectsInvalidServerletRange(t *testing.T) {
	_, errors := ConfigValidator.Validate(map[string]interface{}{
		"serverlets": []interface{}{map[string]interface{}{
			"minInstances": float64(2), "maxInstances": float64(1),
		}},
	})
	if len(errors) == 0 {
		t.Fatal("expected validation error")
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
		{"volume array items must be objects", "volumes", []interface{}{"data"}},
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
			"name": "api", "image": "example/api", "ram": float64(128), "cpu": float64(100), "instances": float64(1), "runtime": "container", "state": "active", "networkId": "net-1",
		}},
		"proxies": []interface{}{map[string]interface{}{
			"domain": "example.com", "type": "http", "target": "api:8080", "port": float64(443), "compress": true, "enforceHttps": true, "verifyTls": true,
		}},
		"volumes": []interface{}{map[string]interface{}{"name": "data", "sizeGb": float64(10), "type": "ssd"}},
		"crons":   []interface{}{map[string]interface{}{"name": "cleanup", "enabled": true, "url": "https://example.com/cleanup", "createdAt": "2026-01-01T00:00:00Z"}},
	})
	if len(errors) != 0 {
		t.Fatalf("unexpected validation errors: %v", errors)
	}
	if len(warnings) != 0 {
		t.Fatalf("generated API fields should not warn: %v", warnings)
	}
}
