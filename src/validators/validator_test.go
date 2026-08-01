package validators

import (
	"encoding/json"
	"testing"
)

func TestStringValidator(t *testing.T) {
	v := NewValidator()
	v.Field("name").Required().String()

	result, errors := v.Validate(map[string]interface{}{
		"name": "John",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["name"] != "John" {
		t.Errorf("Expected 'John', got %v", result["name"])
	}
}

func TestNumberValidator(t *testing.T) {
	v := NewValidator()
	v.Field("age").Required().Number().Min(0).Max(120)

	result, errors := v.Validate(map[string]interface{}{
		"age": 25,
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["age"] != 25 {
		t.Errorf("Expected 25, got %v", result["age"])
	}
}

func TestNumberValidatorFailsOnMinimum(t *testing.T) {
	v := NewValidator()
	v.Field("age").Required().Number().Min(18)

	_, errors := v.Validate(map[string]interface{}{
		"age": 10,
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for age < 18")
	}
}

func TestIntegerAndRangeValidator(t *testing.T) {
	v := NewValidator()
	v.Field("port").Required().Integer().Range(1, 65535)

	if _, errors := v.Validate(map[string]interface{}{"port": float64(443)}); len(errors) != 0 {
		t.Fatalf("expected integer in range to validate, got %v", errors)
	}
	for _, value := range []interface{}{float64(443.5), "443", float64(70000)} {
		if _, errors := v.Validate(map[string]interface{}{"port": value}); len(errors) == 0 {
			t.Errorf("expected %v to fail integer/range validation", value)
		}
	}
}

func TestEmailValidator(t *testing.T) {
	v := NewValidator()
	v.Field("email").Required().Email()

	result, errors := v.Validate(map[string]interface{}{
		"email": "test@example.com",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["email"] != "test@example.com" {
		t.Errorf("Expected 'test@example.com', got %v", result["email"])
	}
}

func TestEmailValidatorFailsOnInvalid(t *testing.T) {
	v := NewValidator()
	v.Field("email").Required().Email()

	_, errors := v.Validate(map[string]interface{}{
		"email": "invalid-email",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid email")
	}
}

func TestBoolValidator(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		valid bool
	}{
		{"bool true", true, true},
		{"bool false", false, true},
		{"float64 1", float64(1), true},
		{"float64 0", float64(0), true},
		{"string true", "true", true},
		{"string false", "false", true},
		{"string yes", "yes", true},
		{"string no", "no", true},
		{"string 1", "1", true},
		{"string 0", "0", true},
		{"invalid float", float64(2), false},
		{"invalid string", "maybe", false},
	}

	for _, tt := range tests {
		v := NewValidator()
		v.Field("enabled").Required().Bool()

		_, errors := v.Validate(map[string]interface{}{
			"enabled": tt.input,
		})

		hasError := len(errors) > 0
		if hasError != !tt.valid {
			t.Errorf("%s: expected valid=%v, got hasError=%v", tt.name, tt.valid, hasError)
		}
	}
}

func TestIPv4Validator(t *testing.T) {
	v := NewValidator()
	v.Field("ip").Required().IPv4()

	result, errors := v.Validate(map[string]interface{}{
		"ip": "192.168.1.1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["ip"] != "192.168.1.1" {
		t.Errorf("Expected '192.168.1.1', got %v", result["ip"])
	}
}

func TestIPv4ValidatorFailsOnIPv6(t *testing.T) {
	v := NewValidator()
	v.Field("ip").Required().IPv4()

	_, errors := v.Validate(map[string]interface{}{
		"ip": "2001:db8::1",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for IPv6 address with IPv4 validator")
	}
}

func TestIPv6Validator(t *testing.T) {
	v := NewValidator()
	v.Field("ip").Required().IPv6()

	result, errors := v.Validate(map[string]interface{}{
		"ip": "2001:db8::1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["ip"] != "2001:db8::1" {
		t.Errorf("Expected '2001:db8::1', got %v", result["ip"])
	}
}

func TestIPValidator(t *testing.T) {
	v := NewValidator()
	v.Field("ip").Required().IP()

	// Test IPv4
	_, errors := v.Validate(map[string]interface{}{
		"ip": "192.168.1.1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for IPv4, got %v", errors)
	}

	// Test IPv6
	_, errors = v.Validate(map[string]interface{}{
		"ip": "2001:db8::1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for IPv6, got %v", errors)
	}
}

func TestEnumValidator(t *testing.T) {
	v := NewValidator()
	v.Field("status").Required().String().Enum("active", "inactive", "pending")

	result, errors := v.Validate(map[string]interface{}{
		"status": "active",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["status"] != "active" {
		t.Errorf("Expected 'active', got %v", result["status"])
	}
}

func TestEnumValidatorFailsOnInvalid(t *testing.T) {
	v := NewValidator()
	v.Field("status").Required().String().Enum("active", "inactive")

	_, errors := v.Validate(map[string]interface{}{
		"status": "unknown",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid enum value")
	}
}

func TestNotEnumValidator(t *testing.T) {
	v := NewValidator()
	v.Field("status").Required().String().NotEnum("deleted", "archived")

	result, errors := v.Validate(map[string]interface{}{
		"status": "active",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["status"] != "active" {
		t.Errorf("Expected 'active', got %v", result["status"])
	}
}

func TestNotEnumValidatorFailsOnDisallowed(t *testing.T) {
	v := NewValidator()
	v.Field("status").Required().String().NotEnum("deleted", "archived")

	_, errors := v.Validate(map[string]interface{}{
		"status": "deleted",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for disallowed enum value")
	}
}

func TestDateValidator(t *testing.T) {
	v := NewValidator()
	v.Field("birthdate").Required().Date()

	result, errors := v.Validate(map[string]interface{}{
		"birthdate": "1990-05-15",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["birthdate"] != "1990-05-15" {
		t.Errorf("Expected '1990-05-15', got %v", result["birthdate"])
	}
}

func TestDateValidatorFailsOnInvalid(t *testing.T) {
	v := NewValidator()
	v.Field("birthdate").Required().Date()

	_, errors := v.Validate(map[string]interface{}{
		"birthdate": "05/15/1990",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid date format")
	}
}

func TestDateTimeValidator(t *testing.T) {
	v := NewValidator()
	v.Field("timestamp").Required().DateTime()

	result, errors := v.Validate(map[string]interface{}{
		"timestamp": "2023-05-15T10:30:00Z",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["timestamp"] != "2023-05-15T10:30:00Z" {
		t.Errorf("Expected '2023-05-15T10:30:00Z', got %v", result["timestamp"])
	}
}

func TestURLValidator(t *testing.T) {
	v := NewValidator()
	v.Field("website").Required().URL()

	result, errors := v.Validate(map[string]interface{}{
		"website": "https://example.com",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["website"] != "https://example.com" {
		t.Errorf("Expected 'https://example.com', got %v", result["website"])
	}
}

func TestURLValidatorFailsOnInvalid(t *testing.T) {
	v := NewValidator()
	v.Field("website").Required().URL()

	_, errors := v.Validate(map[string]interface{}{
		"website": "not a url",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid URL")
	}
}

func TestUUIDValidator(t *testing.T) {
	v := NewValidator()
	v.Field("id").Required().UUID()

	result, errors := v.Validate(map[string]interface{}{
		"id": "550e8400-e29b-41d4-a716-446655440000",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["id"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("Expected UUID, got %v", result["id"])
	}
}

func TestUUIDValidatorFailsOnInvalid(t *testing.T) {
	v := NewValidator()
	v.Field("id").Required().UUID()

	_, errors := v.Validate(map[string]interface{}{
		"id": "not-a-uuid",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid UUID")
	}
}

func TestNotStringValidator(t *testing.T) {
	v := NewValidator()
	v.Field("value").Required().NotString()

	result, errors := v.Validate(map[string]interface{}{
		"value": 42,
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for non-string, got %v", errors)
	}

	if result["value"] != 42 {
		t.Errorf("Expected 42, got %v", result["value"])
	}
}

func TestNotStringValidatorFailsOnString(t *testing.T) {
	v := NewValidator()
	v.Field("value").Required().NotString()

	_, errors := v.Validate(map[string]interface{}{
		"value": "a string",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for string value")
	}
}

func TestNotNumberValidator(t *testing.T) {
	v := NewValidator()
	v.Field("value").Required().NotNumber()

	result, errors := v.Validate(map[string]interface{}{
		"value": "hello",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for non-number string, got %v", errors)
	}

	if result["value"] != "hello" {
		t.Errorf("Expected 'hello', got %v", result["value"])
	}
}

func TestNotNumberValidatorFailsOnNumber(t *testing.T) {
	v := NewValidator()
	v.Field("value").Required().NotNumber()

	_, errors := v.Validate(map[string]interface{}{
		"value": 42,
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for numeric value")
	}
}

func TestNotNumberValidatorFailsOnNumericString(t *testing.T) {
	v := NewValidator()
	v.Field("value").Required().NotNumber()

	_, errors := v.Validate(map[string]interface{}{
		"value": "42",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for numeric string")
	}
}

func TestOptionalField(t *testing.T) {
	v := NewValidator()
	v.Field("nickname").Optional().String()

	result, errors := v.Validate(map[string]interface{}{})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for missing optional field, got %v", errors)
	}

	// Optional field should not be in result
	if _, exists := result["nickname"]; !exists {
		// This is expected behavior - optional missing fields are not included
	}
}

func TestTrimTransform(t *testing.T) {
	v := NewValidator()
	v.Field("name").Required().String().Trim()

	result, errors := v.Validate(map[string]interface{}{
		"name": "  John  ",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["name"] != "John" {
		t.Errorf("Expected 'John', got '%v'", result["name"])
	}
}

func TestChainedValidators(t *testing.T) {
	v := NewValidator()
	v.Field("username").Required().String().MinLength(3).MaxLength(20).Trim()

	result, errors := v.Validate(map[string]interface{}{
		"username": "  john_doe  ",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["username"] != "john_doe" {
		t.Errorf("Expected 'john_doe', got '%v'", result["username"])
	}
}

func TestMultipleFields(t *testing.T) {
	v := NewValidator()
	v.Field("email").Required().Email()
	v.Field("age").Required().Number().Min(18).Max(100)
	v.Field("status").Optional().String().Enum("active", "inactive")

	result, errors := v.Validate(map[string]interface{}{
		"email":  "test@example.com",
		"age":    25,
		"status": "active",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}

	if result["email"] != "test@example.com" {
		t.Errorf("Expected email 'test@example.com'")
	}

	if result["age"] != 25 {
		t.Errorf("Expected age 25")
	}
}

func TestMinMaxLength(t *testing.T) {
	v := NewValidator()
	v.Field("code").Required().String().MinLength(3).MaxLength(5)

	// Valid
	_, errors := v.Validate(map[string]interface{}{
		"code": "ABC",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid length, got %v", errors)
	}

	// Too short
	_, errors = v.Validate(map[string]interface{}{
		"code": "AB",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for string too short")
	}

	// Too long
	_, errors = v.Validate(map[string]interface{}{
		"code": "ABCDEF",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for string too long")
	}
}

func TestConditionalValidation(t *testing.T) {
	// DNS record validator: if type is A, value must be IPv4; if AAAA, must be IPv6
	v := NewValidator()
	v.Field("type").Required().String().Enum("A", "AAAA", "MX", "TXT")
	v.Field("value").Required().String()

	// Add conditional validators
	v.When("type", "A").Then("value").IPv4()
	v.When("type", "AAAA").Then("value").IPv6()
	v.When("type", "MX").Then("value").Domain()
	v.When("type", "TXT").Then("value").String()

	// Test A record with valid IPv4
	_, errors := v.Validate(map[string]interface{}{
		"type":  "A",
		"value": "192.168.1.1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid A record, got %v", errors)
	}

	// Test A record with invalid IPv4 (should fail)
	_, errors = v.Validate(map[string]interface{}{
		"type":  "A",
		"value": "not-an-ip",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid IPv4 in A record")
	}

	// Test AAAA record with valid IPv6
	_, errors = v.Validate(map[string]interface{}{
		"type":  "AAAA",
		"value": "2001:db8::1",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid AAAA record, got %v", errors)
	}

	// Test AAAA record with IPv4 (should fail)
	_, errors = v.Validate(map[string]interface{}{
		"type":  "AAAA",
		"value": "192.168.1.1",
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error for IPv4 in AAAA record")
	}

	// Test MX record with valid domain
	_, errors = v.Validate(map[string]interface{}{
		"type":  "MX",
		"value": "mail.example.com",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid MX record, got %v", errors)
	}

	// Test TXT record (just needs to be a string)
	_, errors = v.Validate(map[string]interface{}{
		"type":  "TXT",
		"value": "v=spf1 include:example.com ~all",
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid TXT record, got %v", errors)
	}
}

func TestConditionalValidationNoConditionMatch(t *testing.T) {
	v := NewValidator()
	v.Field("status").Required().String().Enum("active", "inactive")
	v.Field("reason").Required().String()

	// If status is inactive, reason is required; but if active, reason shouldn't be validated
	v.When("status", "inactive").Then("reason").MinLength(5)

	// Status is active, so conditional validation should not apply
	_, errors := v.Validate(map[string]interface{}{
		"status": "active",
		"reason": "ok", // Short reason is OK because condition doesn't match
	})

	if len(errors) > 0 {
		t.Errorf("Expected no errors when condition doesn't match, got %v", errors)
	}

	// Status is inactive, so conditional validation should apply
	_, errors = v.Validate(map[string]interface{}{
		"status": "inactive",
		"reason": "ok", // Now short reason fails because MinLength(5)
	})

	if len(errors) == 0 {
		t.Errorf("Expected validation error when condition matches and rule fails")
	}
}

func TestArrayValidator(t *testing.T) {
	v := NewValidator()
	v.Field("items").Required().Array()

	// Valid array
	_, errors := v.Validate(map[string]interface{}{
		"items": []interface{}{1, 2, 3},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid array, got %v", errors)
	}

	// Valid empty array
	_, errors = v.Validate(map[string]interface{}{
		"items": []interface{}{},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for empty array, got %v", errors)
	}

	// Invalid: not an array (should fail)
	_, errors = v.Validate(map[string]interface{}{
		"items": "not-an-array",
	})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for non-array value")
	}

	// Invalid: missing required array
	_, errors = v.Validate(map[string]interface{}{})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for missing required array")
	}
}

func TestObjectValidator(t *testing.T) {
	// Create nested validator for object structure
	nestedValidator := NewValidator()
	nestedValidator.Field("name").Required().String()
	nestedValidator.Field("age").Required().Number().Min(0).Max(150)

	v := NewValidator()
	v.Field("user").Required().Object(nestedValidator)

	// Valid object
	_, errors := v.Validate(map[string]interface{}{
		"user": map[string]interface{}{
			"name": "John",
			"age":  30,
		},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid object, got %v", errors)
	}

	// Invalid: missing required field in nested object
	_, errors = v.Validate(map[string]interface{}{
		"user": map[string]interface{}{
			"age": 30,
		},
	})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for missing required nested field")
	}

	// Invalid: invalid field type in nested object
	_, errors = v.Validate(map[string]interface{}{
		"user": map[string]interface{}{
			"name": "John",
			"age":  "not-a-number",
		},
	})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for invalid nested field type")
	}

	// Invalid: not an object (should fail)
	_, errors = v.Validate(map[string]interface{}{
		"user": "not-an-object",
	})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for non-object value")
	}

	// Invalid: missing required object
	_, errors = v.Validate(map[string]interface{}{})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for missing required object")
	}
}

func TestNestedObjectValidator(t *testing.T) {
	// Create validator for address object
	addressValidator := NewValidator()
	addressValidator.Field("street").Required().String()
	addressValidator.Field("city").Required().String()
	addressValidator.Field("zipcode").Required().String()

	// Create validator for user object with nested address
	userValidator := NewValidator()
	userValidator.Field("name").Required().String()
	userValidator.Field("email").Required().Email()
	userValidator.Field("address").Required().Object(addressValidator)

	// Main validator
	v := NewValidator()
	v.Field("users").Required().Array()
	v.Field("primaryUser").Required().Object(userValidator)

	// Valid nested structure
	_, errors := v.Validate(map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"name": "John",
			},
		},
		"primaryUser": map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
			"address": map[string]interface{}{
				"street":  "123 Main St",
				"city":    "Springfield",
				"zipcode": "12345",
			},
		},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid nested structure, got %v", errors)
	}

	// Invalid: missing required nested address field
	_, errors = v.Validate(map[string]interface{}{
		"users": []interface{}{},
		"primaryUser": map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
			"address": map[string]interface{}{
				"street": "123 Main St",
				"city":   "Springfield",
				// Missing zipcode
			},
		},
	})
	if len(errors) == 0 {
		t.Errorf("Expected validation error for missing nested address field")
	}
}

func TestOptionalArrayValidator(t *testing.T) {
	v := NewValidator()
	v.Field("tags").Optional().Array()

	// Valid: omitted optional array
	_, errors := v.Validate(map[string]interface{}{})
	if len(errors) > 0 {
		t.Errorf("Expected no errors when optional array is omitted, got %v", errors)
	}

	// Valid: array present
	_, errors = v.Validate(map[string]interface{}{
		"tags": []interface{}{"a", "b", "c"},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid array, got %v", errors)
	}
}

func TestOptionalObjectValidator(t *testing.T) {
	nestedValidator := NewValidator()
	nestedValidator.Field("name").Required().String()

	v := NewValidator()
	v.Field("metadata").Optional().Object(nestedValidator)

	// Valid: omitted optional object
	_, errors := v.Validate(map[string]interface{}{})
	if len(errors) > 0 {
		t.Errorf("Expected no errors when optional object is omitted, got %v", errors)
	}

	// Valid: object present
	_, errors = v.Validate(map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test",
		},
	})
	if len(errors) > 0 {
		t.Errorf("Expected no errors for valid object, got %v", errors)
	}
}

func TestToJSONSchema(t *testing.T) {
	v := NewValidator()
	v.Description("Example validator.")
	item := NewValidator()
	item.Field("port").Required().Number().Min(1).Max(65535)
	v.Field("name").Required().String().MinLength(1).MaxLength(255)
	v.Field("ports").Optional().Array(item)

	schema := v.ToJSONSchema()
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected draft: %v", schema["$schema"])
	}
	if schema["description"] != "Example validator." {
		t.Fatalf("unexpected root description: %#v", schema)
	}
	properties := schema["properties"].(map[string]interface{})
	name := properties["name"].(map[string]interface{})
	if name["type"] != "string" || name["minLength"] != 1 {
		t.Fatalf("unexpected name schema: %#v", name)
	}
	ports := properties["ports"].(map[string]interface{})
	items := ports["items"].(map[string]interface{})
	if _, ok := items["$schema"]; ok {
		t.Fatalf("nested schema must not repeat $schema: %#v", items)
	}
	itemProperties := items["properties"].(map[string]interface{})
	if itemProperties["port"].(map[string]interface{})["maximum"] != float64(65535) {
		t.Fatalf("unexpected item schema: %#v", items)
	}

	data, err := v.ToJSONSchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("schema is not JSON: %s", data)
	}
}

func TestArrayValidatorAppliesItemValidator(t *testing.T) {
	itemValidator := NewValidator()
	itemValidator.Field("port").Required().Number().Min(1).Max(65535)
	v := NewValidator()
	v.Field("targets").Required().Array(itemValidator)

	if _, errors := v.Validate(map[string]interface{}{"targets": []interface{}{map[string]interface{}{"port": float64(443)}}}); len(errors) != 0 {
		t.Fatalf("expected valid array items, got %v", errors)
	}
	if _, errors := v.Validate(map[string]interface{}{"targets": []interface{}{map[string]interface{}{"port": float64(0)}}}); len(errors) == 0 {
		t.Fatal("expected invalid nested item to fail")
	}
}

func TestValidateWithWarningsReportsUnexpectedKeysWithoutFailing(t *testing.T) {
	itemValidator := NewValidator()
	itemValidator.Field("name").Required().String()
	v := NewValidator()
	v.Field("items").Required().Array(itemValidator)

	_, errors, warnings := v.ValidateWithWarnings(map[string]interface{}{
		"items":   []interface{}{map[string]interface{}{"name": "api", "extra": true}},
		"unknown": true,
	})
	if len(errors) != 0 {
		t.Fatalf("unexpected validation errors: %v", errors)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected two warnings, got %v", warnings)
	}
	if warnings[0] != `found unexpected key "items[0].extra"` && warnings[1] != `found unexpected key "items[0].extra"` {
		t.Fatalf("missing nested warning: %v", warnings)
	}
}
