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

func TestJSONSchemaToGoRejectsInvalidJSON(t *testing.T) {
	if _, err := JSONSchemaToGo([]byte("not JSON"), "https://example.test/schema"); err == nil {
		t.Fatal("JSONSchemaToGo() succeeded for invalid JSON")
	}
}
