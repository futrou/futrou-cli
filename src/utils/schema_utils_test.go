package utils

import (
	"strings"
	"testing"
)

func TestJSONSchemaToGo(t *testing.T) {
	schema := []byte(`{
  "components": {"schemas": {
    "ApiToken": {"type": "object", "properties": {"id": {"type": "string"}}},
    "User": {"type": "object", "properties": {"name": {"type": "string"}}},
    "Serverlet": {"type": "object", "properties": {"createdAt": {"type": "string", "format": "date-time"}}}
  }}
}`)

	generated, err := JSONSchemaToGo(schema, "https://example.test/v2/openapi.json")
	if err != nil {
		t.Fatalf("JSONSchemaToGo() error = %v", err)
	}

	output := string(generated)
	for _, want := range []string{
		"package api",
		"// Source: https://example.test/v2/openapi.json",
		"type Serverlet struct",
		"CreatedAt time.Time",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("generated source does not contain %q", want)
		}
	}
}

func TestJSONSchemaToGoAPIErrorSurfacesFieldErrorDetails(t *testing.T) {
	schema := []byte(`{"components": {"schemas": {}}}`)

	generated, err := JSONSchemaToGo(schema, "https://example.test/v2/openapi.json")
	if err != nil {
		t.Fatalf("JSONSchemaToGo() error = %v", err)
	}

	output := string(generated)
	for _, want := range []string{
		`import "strings"`,
		"func (e *APIError) Error() string {",
		"strings.Join(details",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("generated source does not contain %q\ngot:\n%s", want, output)
		}
	}
}

func TestJSONSchemaToGoRejectsInvalidJSON(t *testing.T) {
	if _, err := JSONSchemaToGo([]byte("not JSON"), "https://example.test/schema"); err == nil {
		t.Fatal("JSONSchemaToGo() succeeded for invalid JSON")
	}
}

func TestJSONSchemaToGoGeneratesStringEnums(t *testing.T) {
	schema := []byte(`{
  "components": {"schemas": {
    "Serverlet": {"type": "object", "properties": {"strategy": {"$ref": "#/components/schemas/ProxyStrategy"}}},
    "ProxyStrategy": {"type": "string", "enum": ["round-robin", "primary-failover"], "description": "Proxy Strategy enumeration"}
  }}
}`)

	generated, err := JSONSchemaToGo(schema, "https://example.test/v2/openapi.json")
	if err != nil {
		t.Fatalf("JSONSchemaToGo() error = %v", err)
	}

	output := string(generated)
	for _, want := range []string{
		"type ProxyStrategy string",
		`ProxyStrategyRoundRobin ProxyStrategy = "round-robin"`,
		`ProxyStrategyPrimaryFailover ProxyStrategy = "primary-failover"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("generated source does not contain %q\ngot:\n%s", want, output)
		}
	}
	if strings.Contains(output, "type ProxyStrategy struct") {
		t.Errorf("enum schema was generated as an empty struct instead of a string type")
	}
}
