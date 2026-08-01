package validators

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FieldValidator represents a single field valIdator
type FieldValidator struct {
	name        string
	description string
	rules       []ValidationRule
	required    bool
	errorMsg    string
	transforms  []func(interface{}) interface{}
}

// ValidationRule defines a single valIdation check
type ValidationRule interface {
	Validate(value interface{}) error
	String() string
}

// FuncRule adapts a function to a validation rule. It is useful for fields
// whose validation depends on the shape of nested data, such as arrays of
// polymorphic objects.
type FuncRule struct {
	fn func(interface{}) error
}

func (r FuncRule) Validate(value interface{}) error { return r.fn(value) }
func (r FuncRule) String() string                   { return "custom" }

// BaseRule is a base for all valIdation rules
type BaseRule struct {
	name     string
	errorMsg string
}

func (r BaseRule) String() string {
	return r.name
}

// RequiredRule valIdates that a value is not empty
type RequiredRule struct {
	BaseRule
}

func (r RequiredRule) Validate(value interface{}) error {
	if value == nil {
		return fmt.Errorf("field is required")
	}
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("field is required")
		}
	}
	return nil
}

// StringRule valIdates that a value is a string
type StringRule struct {
	BaseRule
}

func (r StringRule) Validate(value interface{}) error {
	_, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	return nil
}

// NotStringRule valIdates that a value is NOT a string
type NotStringRule struct {
	BaseRule
}

func (r NotStringRule) Validate(value interface{}) error {
	_, ok := value.(string)
	if ok {
		return fmt.Errorf("must not be a string")
	}
	return nil
}

// NumberRule valIdates that a value is a number
type NumberRule struct {
	BaseRule
}

func (r NumberRule) Validate(value interface{}) error {
	switch v := value.(type) {
	case float64, int, int64:
		return nil
	case string:
		_, err := strconv.ParseFloat(v, 64)
		return err
	default:
		return fmt.Errorf("must be a number")
	}
}

// IntegerRule validates JSON-compatible integer values.
type IntegerRule struct {
	BaseRule
}

func (r IntegerRule) Validate(value interface{}) error {
	if _, ok := integer(value); !ok {
		return fmt.Errorf("must be an integer")
	}
	return nil
}

func integer(value interface{}) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		if number > int64(math.MaxInt) || number < int64(math.MinInt) {
			return 0, false
		}
		return int(number), true
	case float64:
		if math.Trunc(number) != number || number > math.MaxInt || number < math.MinInt {
			return 0, false
		}
		return int(number), true
	default:
		return 0, false
	}
}

// EmailRule valIdates email format
type EmailRule struct {
	BaseRule
}

func (r EmailRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	const emailRegex = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	re := regexp.MustCompile(emailRegex)
	if !re.MatchString(s) {
		return fmt.Errorf("must be a valId email")
	}
	return nil
}

// DomainRule valIdates domain name format
type DomainRule struct {
	BaseRule
}

func (r DomainRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	// Domain regex pattern: valid domain names with TLD
	const domainRegex = `^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	re := regexp.MustCompile(domainRegex)
	if !re.MatchString(s) {
		return fmt.Errorf("must be a valid domain")
	}
	return nil
}

// MinLengthRule valIdates minimum string length
type MinLengthRule struct {
	BaseRule
	length int
}

func (r MinLengthRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if len(s) < r.length {
		return fmt.Errorf("must be at least %d characters", r.length)
	}
	return nil
}

// MaxLengthRule valIdates maximum string length
type MaxLengthRule struct {
	BaseRule
	length int
}

func (r MaxLengthRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if len(s) > r.length {
		return fmt.Errorf("must be at most %d characters", r.length)
	}
	return nil
}

// MinRule valIdates minimum numeric value
type MinRule struct {
	BaseRule
	value float64
}

func (r MinRule) Validate(value interface{}) error {
	var num float64
	switch v := value.(type) {
	case float64:
		num = v
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		num = n
	default:
		return fmt.Errorf("must be a number")
	}
	if num < r.value {
		return fmt.Errorf("must be at least %v", r.value)
	}
	return nil
}

// MaxRule valIdates maximum numeric value
type MaxRule struct {
	BaseRule
	value float64
}

func (r MaxRule) Validate(value interface{}) error {
	var num float64
	switch v := value.(type) {
	case float64:
		num = v
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		num = n
	default:
		return fmt.Errorf("must be a number")
	}
	if num > r.value {
		return fmt.Errorf("must be at most %v", r.value)
	}
	return nil
}

// RegexRule valIdates against a regex pattern
type RegexRule struct {
	BaseRule
	pattern *regexp.Regexp
}

// NoRegexRule valIdates that a value does NOT match a regex pattern
type NoRegexRule struct {
	BaseRule
	pattern *regexp.Regexp
}

func (r RegexRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if !r.pattern.MatchString(s) {
		return fmt.Errorf("must match the required format")
	}
	return nil
}

func (r NoRegexRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	if r.pattern.MatchString(s) {
		return fmt.Errorf("must not match the pattern")
	}
	return nil
}

// DecimalRule valIdates decimal places in numeric values
type DecimalRule struct {
	BaseRule
	decimalPlaces int
}

// EnumRule valIdates that a value is one of allowed values
type EnumRule struct {
	BaseRule
	allowedValues []interface{}
}

// NotEnumRule valIdates that a value is NOT one of disallowed values
type NotEnumRule struct {
	BaseRule
	disallowedValues []interface{}
}

// DateRule valIdates that a value is a valid date (YYYY-MM-DD format)
type DateRule struct {
	BaseRule
}

// DateTimeRule valIdates that a value is a valid datetime (RFC3339 or common formats)
type DateTimeRule struct {
	BaseRule
}

// BoolRule valIdates that a value is a boolean
type BoolRule struct {
	BaseRule
}

// IPv4Rule valIdates that a value is a valid IPv4 address
type IPv4Rule struct {
	BaseRule
}

// IPv6Rule valIdates that a value is a valid IPv6 address
type IPv6Rule struct {
	BaseRule
}

// IPRule valIdates that a value is a valid IPv4 or IPv6 address
type IPRule struct {
	BaseRule
}

// NotNumberRule valIdates that a value is NOT a number
type NotNumberRule struct {
	BaseRule
}

// URLRule valIdates that a value is a valid URL
type URLRule struct {
	BaseRule
}

// UUIDRule valIdates that a value is a valid UUID (v4)
type UUIDRule struct {
	BaseRule
}

// ArrayRule valIdates that a value is an array
type ArrayRule struct {
	BaseRule
	validator *Validator
}

// ObjectRule valIdates that a value is an object with a nested validator
type ObjectRule struct {
	BaseRule
	validator *Validator
}

func (r DecimalRule) Validate(value interface{}) error {
	var numStr string

	// Convert value to string for decimal place checking
	switch v := value.(type) {
	case float64:
		numStr = strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		numStr = strconv.Itoa(v)
	case int64:
		numStr = strconv.FormatInt(v, 10)
	case string:
		numStr = v
	default:
		return fmt.Errorf("must be a number")
	}

	// Check if it's a valid number
	_, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return fmt.Errorf("must be a number")
	}

	// Count decimal places
	parts := strings.Split(numStr, ".")
	if len(parts) > 2 {
		return fmt.Errorf("invalid number format")
	}

	if len(parts) == 2 {
		decimalPart := parts[1]
		if len(decimalPart) > r.decimalPlaces {
			return fmt.Errorf("must have at most %d decimal places", r.decimalPlaces)
		}
	}

	return nil
}

func (r EnumRule) Validate(value interface{}) error {
	for _, allowed := range r.allowedValues {
		if allowed == value {
			return nil
		}
	}

	// Build error message with allowed values
	allowedStrs := make([]string, len(r.allowedValues))
	for i, val := range r.allowedValues {
		allowedStrs[i] = fmt.Sprintf("%v", val)
	}

	return fmt.Errorf("must be one of: %s", strings.Join(allowedStrs, ", "))
}

func (r NotEnumRule) Validate(value interface{}) error {
	for _, disallowed := range r.disallowedValues {
		if disallowed == value {
			// Build error message with disallowed values
			disallowedStrs := make([]string, len(r.disallowedValues))
			for i, val := range r.disallowedValues {
				disallowedStrs[i] = fmt.Sprintf("%v", val)
			}
			return fmt.Errorf("must not be one of: %s", strings.Join(disallowedStrs, ", "))
		}
	}
	return nil
}

func (r DateRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	// Try to parse as YYYY-MM-DD format
	_, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fmt.Errorf("must be a valid date (YYYY-MM-DD)")
	}
	return nil
}

func (r DateTimeRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	// Try common datetime formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if _, err := time.Parse(format, s); err == nil {
			return nil
		}
	}

	return fmt.Errorf("must be a valid datetime (RFC3339 format)")
}

func (r BoolRule) Validate(value interface{}) error {
	switch v := value.(type) {
	case bool:
		return nil
	case float64:
		// JSON unmarshals numbers as float64, so 0 and 1 are valid
		if v == 0 || v == 1 {
			return nil
		}
		return fmt.Errorf("must be a boolean")
	case string:
		// Accept common boolean string representations
		lower := strings.ToLower(v)
		if lower == "true" || lower == "false" || lower == "1" || lower == "0" || lower == "yes" || lower == "no" {
			return nil
		}
		return fmt.Errorf("must be a boolean (true, false, 1, 0, yes, or no)")
	default:
		return fmt.Errorf("must be a boolean")
	}
}

func (r IPv4Rule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("must be a valid IPv4 address")
	}

	// Check if it's IPv4
	if ip.To4() == nil {
		return fmt.Errorf("must be a valid IPv4 address")
	}

	return nil
}

func (r IPv6Rule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("must be a valid IPv6 address")
	}

	// Check if it's IPv6
	if ip.To4() != nil {
		return fmt.Errorf("must be a valid IPv6 address")
	}

	return nil
}

func (r IPRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("must be a valid IPv4 or IPv6 address")
	}

	return nil
}

func (r NotNumberRule) Validate(value interface{}) error {
	switch v := value.(type) {
	case float64, int, int64:
		return fmt.Errorf("must not be a number")
	case string:
		_, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return fmt.Errorf("must not be a number")
		}
	}
	return nil
}

func (r URLRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	parsedURL, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("must be a valid URL")
	}

	// Check that it has a scheme and host
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("must be a valid URL with scheme and host")
	}

	return nil
}

func (r UUIDRule) Validate(value interface{}) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// Validate format: 8-4-4-4-12 hexadecimal characters
	uuidRegex := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	re := regexp.MustCompile(uuidRegex)
	if !re.MatchString(s) {
		return fmt.Errorf("must be a valid UUID")
	}
	return nil
}

func (r ArrayRule) Validate(value interface{}) error {
	items, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("must be an array")
	}
	if r.validator == nil {
		return nil
	}
	for i, item := range items {
		object, ok := item.(map[string]interface{})
		if !ok {
			return fmt.Errorf("item %d must be an object", i)
		}
		if _, errors := r.validator.Validate(object); len(errors) > 0 {
			for field, message := range errors {
				return fmt.Errorf("item %d.%s: %s", i, field, message)
			}
		}
	}
	return nil
}

func (r ObjectRule) Validate(value interface{}) error {
	objMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("must be an object")
	}

	// Validate the object using the nested validator
	_, errors := r.validator.Validate(objMap)
	if len(errors) > 0 {
		// Return the first error from nested validation
		for field, errMsg := range errors {
			return fmt.Errorf("%s: %s", field, errMsg)
		}
	}

	return nil
}

// FieldValidator methods
func (f *FieldValidator) Required() *FieldValidator {
	f.required = true
	f.rules = append(f.rules, RequiredRule{BaseRule{"required", ""}})
	return f
}

func (f *FieldValidator) Optional() *FieldValidator {
	f.required = false
	return f
}

func (f *FieldValidator) String() *FieldValidator {
	f.rules = append(f.rules, StringRule{BaseRule{"string", ""}})
	return f
}

func (f *FieldValidator) NotString() *FieldValidator {
	f.rules = append(f.rules, NotStringRule{BaseRule{"notString", ""}})
	return f
}

func (f *FieldValidator) Number() *FieldValidator {
	f.rules = append(f.rules, NumberRule{BaseRule{"number", ""}})
	return f
}

// Integer requires a whole number. It accepts the float64 values produced by
// encoding/json only when they have no fractional component.
func (f *FieldValidator) Integer() *FieldValidator {
	f.rules = append(f.rules, IntegerRule{BaseRule{"integer", ""}})
	return f
}

func (f *FieldValidator) NotNumber() *FieldValidator {
	f.rules = append(f.rules, NotNumberRule{BaseRule{"notNumber", ""}})
	return f
}

func (f *FieldValidator) Email() *FieldValidator {
	f.rules = append(f.rules, EmailRule{BaseRule{"email", ""}})
	return f
}

func (f *FieldValidator) Domain() *FieldValidator {
	f.rules = append(f.rules, DomainRule{BaseRule{"domain", ""}})
	return f
}

func (f *FieldValidator) MinLength(length int) *FieldValidator {
	f.rules = append(f.rules, MinLengthRule{BaseRule{"minLength", ""}, length})
	return f
}

func (f *FieldValidator) MaxLength(length int) *FieldValidator {
	f.rules = append(f.rules, MaxLengthRule{BaseRule{"maxLength", ""}, length})
	return f
}

func (f *FieldValidator) Min(value float64) *FieldValidator {
	f.rules = append(f.rules, MinRule{BaseRule{"min", ""}, value})
	return f
}

func (f *FieldValidator) Max(value float64) *FieldValidator {
	f.rules = append(f.rules, MaxRule{BaseRule{"max", ""}, value})
	return f
}

// Range applies inclusive numeric minimum and maximum constraints.
func (f *FieldValidator) Range(min, max float64) *FieldValidator {
	return f.Min(min).Max(max)
}

func (f *FieldValidator) Regex(pattern string) *FieldValidator {
	re := regexp.MustCompile(pattern)
	f.rules = append(f.rules, RegexRule{BaseRule{"regex", ""}, re})
	return f
}

func (f *FieldValidator) NoRegex(pattern string) *FieldValidator {
	re := regexp.MustCompile(pattern)
	f.rules = append(f.rules, NoRegexRule{BaseRule{"noRegex", ""}, re})
	return f
}

func (f *FieldValidator) Decimal(decimalPlaces int) *FieldValidator {
	f.rules = append(f.rules, DecimalRule{BaseRule{"decimal", ""}, decimalPlaces})
	return f
}

func (f *FieldValidator) Enum(values ...interface{}) *FieldValidator {
	f.rules = append(f.rules, EnumRule{BaseRule{"enum", ""}, values})
	return f
}

func (f *FieldValidator) NotEnum(values ...interface{}) *FieldValidator {
	f.rules = append(f.rules, NotEnumRule{BaseRule{"notEnum", ""}, values})
	return f
}

func (f *FieldValidator) Date() *FieldValidator {
	f.rules = append(f.rules, DateRule{BaseRule{"date", ""}})
	return f
}

func (f *FieldValidator) DateTime() *FieldValidator {
	f.rules = append(f.rules, DateTimeRule{BaseRule{"datetime", ""}})
	return f
}

func (f *FieldValidator) Bool() *FieldValidator {
	f.rules = append(f.rules, BoolRule{BaseRule{"bool", ""}})
	return f
}

func (f *FieldValidator) IPv4() *FieldValidator {
	f.rules = append(f.rules, IPv4Rule{BaseRule{"ipv4", ""}})
	return f
}

func (f *FieldValidator) IPv6() *FieldValidator {
	f.rules = append(f.rules, IPv6Rule{BaseRule{"ipv6", ""}})
	return f
}

func (f *FieldValidator) IP() *FieldValidator {
	f.rules = append(f.rules, IPRule{BaseRule{"ip", ""}})
	return f
}

func (f *FieldValidator) URL() *FieldValidator {
	f.rules = append(f.rules, URLRule{BaseRule{"url", ""}})
	return f
}

func (f *FieldValidator) UUID() *FieldValidator {
	f.rules = append(f.rules, UUIDRule{BaseRule{"uuid", ""}})
	return f
}

// Array validates an array and, when given an item validator, applies that
// validator to every object in the array. Array() remains valid for arrays of
// primitive values or callers that only need an array type check.
func (f *FieldValidator) Array(validator ...*Validator) *FieldValidator {
	var itemValidator *Validator
	if len(validator) > 0 {
		itemValidator = validator[0]
	}
	f.rules = append(f.rules, ArrayRule{BaseRule: BaseRule{"array", ""}, validator: itemValidator})
	return f
}

func (f *FieldValidator) Object(validator *Validator) *FieldValidator {
	f.rules = append(f.rules, ObjectRule{BaseRule{"object", ""}, validator})
	return f
}

// Custom adds an application-specific validation function to the field.
func (f *FieldValidator) Custom(fn func(interface{}) error) *FieldValidator {
	f.rules = append(f.rules, FuncRule{fn: fn})
	return f
}

func (f *FieldValidator) Trim() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) ToLower() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return strings.ToLower(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) ToUpper() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return strings.ToUpper(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) ToCamelCase() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return toCamelCase(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) ToSnakeCase() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return toSnakeCase(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) ToPascalCase() *FieldValidator {
	f.transforms = append(f.transforms, func(v interface{}) interface{} {
		if s, ok := v.(string); ok {
			return toPascalCase(s)
		}
		return v
	})
	return f
}

func (f *FieldValidator) Transform(fn func(interface{}) interface{}) *FieldValidator {
	f.transforms = append(f.transforms, fn)
	return f
}

func (f *FieldValidator) ErrorMessage(msg string) *FieldValidator {
	f.errorMsg = msg
	return f
}

// Description adds human-readable documentation to this field's JSON Schema.
func (f *FieldValidator) Description(description string) *FieldValidator {
	f.description = description
	return f
}

func (f *FieldValidator) validate(value interface{}) (interface{}, error) {
	// Check if required
	if f.required && (value == nil || (value == "")) {
		if f.errorMsg != "" {
			return nil, fmt.Errorf("%s", f.errorMsg)
		}
		return nil, fmt.Errorf("%s is required", f.name)
	}

	// If optional and empty, skip valIdation
	if !f.required && (value == nil || value == "") {
		return value, nil
	}

	// Apply transformations
	for _, transform := range f.transforms {
		value = transform(value)
	}

	// Run valIdation rules
	for _, rule := range f.rules {
		if err := rule.Validate(value); err != nil {
			if f.errorMsg != "" {
				return nil, fmt.Errorf("%s", f.errorMsg)
			}
			return nil, fmt.Errorf("%s: %s", f.name, err.Error())
		}
	}

	return value, nil
}

// Helper functions for case conversion
func toCamelCase(s string) string {
	if s == "" {
		return s
	}
	// Replace spaces, underscores, and hyphens with spaces first
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	// Split by spaces
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return s
	}
	// First part lowercase, rest capitalized
	result := strings.ToLower(parts[0])
	for _, part := range parts[1:] {
		if len(part) > 0 {
			result += strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return result
}

func toSnakeCase(s string) string {
	if s == "" {
		return s
	}
	// Replace hyphens with underscores
	s = strings.ReplaceAll(s, "-", "_")
	// Insert underscore before uppercase letters
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' && s[i-1] != '_' && s[i-1] != ' ' {
			result.WriteRune('_')
		}
		if r == ' ' {
			result.WriteRune('_')
		} else {
			result.WriteRune(r)
		}
	}
	return strings.ToLower(result.String())
}

func toPascalCase(s string) string {
	if s == "" {
		return s
	}
	// Replace spaces, underscores, and hyphens with spaces first
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	// Split by spaces
	parts := strings.Fields(s)
	// Capitalize each part
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(string(part[0])) + strings.ToLower(part[1:]))
		}
	}
	return result.String()
}

// ConditionalValidator applies validation rules only when a condition is met
type ConditionalValidator struct {
	ConditionField string
	ConditionValue interface{}
	TargetField    string
	Validator      *FieldValidator
}

// ConditionalValidatorBuilder helps build conditional validators with fluent API
type ConditionalValidatorBuilder struct {
	validator      *Validator
	conditionField string
	conditionValue interface{}
}

// Validator is a collection of field validators
type Validator struct {
	description           string
	fields                map[string]*FieldValidator
	conditionalValidators []*ConditionalValidator
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		fields:                make(map[string]*FieldValidator),
		conditionalValidators: make([]*ConditionalValidator, 0),
	}
}

// Description adds human-readable documentation to generated JSON Schema.
func (v *Validator) Description(description string) *Validator {
	v.description = description
	return v
}

// Field adds a new field validator
func (v *Validator) Field(name string) *FieldValidator {
	fv := &FieldValidator{
		name:       name,
		rules:      make([]ValidationRule, 0),
		transforms: make([]func(interface{}) interface{}, 0),
	}
	v.fields[name] = fv
	return fv
}

// When starts a conditional validator - validates a target field only if a condition field has a specific value
// Usage: v.When("type", "A").Then("value").IPv4()
func (v *Validator) When(fieldName string, fieldValue interface{}) *ConditionalValidatorBuilder {
	return &ConditionalValidatorBuilder{
		validator:      v,
		conditionField: fieldName,
		conditionValue: fieldValue,
	}
}

// Then specifies which field to validate when the condition is met
func (b *ConditionalValidatorBuilder) Then(targetField string) *FieldValidator {
	fv := &FieldValidator{
		name:       targetField,
		rules:      make([]ValidationRule, 0),
		transforms: make([]func(interface{}) interface{}, 0),
	}
	b.validator.conditionalValidators = append(b.validator.conditionalValidators, &ConditionalValidator{
		ConditionField: b.conditionField,
		ConditionValue: b.conditionValue,
		TargetField:    targetField,
		Validator:      fv,
	})
	return fv
}

// Validate validates the provided data map
func (v *Validator) Validate(data map[string]interface{}) (map[string]interface{}, map[string]string) {
	result := make(map[string]interface{})
	errors := make(map[string]string)

	// Validate regular fields
	for fieldName, fieldValidator := range v.fields {
		value, exists := data[fieldName]
		if !exists {
			value = nil
		}

		if validatedValue, err := fieldValidator.validate(value); err == nil {
			result[fieldName] = validatedValue
		} else {
			errors[fieldName] = err.Error()
		}
	}

	// Validate conditional fields
	for _, condValidator := range v.conditionalValidators {
		// Check if the condition is met
		conditionValue, exists := data[condValidator.ConditionField]
		if !exists {
			continue
		}

		// If condition matches, validate the target field
		if conditionValue == condValidator.ConditionValue {
			targetValue, targetExists := data[condValidator.TargetField]
			if !targetExists {
				targetValue = nil
			}

			if validatedValue, err := condValidator.Validator.validate(targetValue); err == nil {
				result[condValidator.TargetField] = validatedValue
			} else {
				errors[condValidator.TargetField] = err.Error()
			}
		}
	}

	if len(errors) > 0 {
		return nil, errors
	}

	return result, nil
}

// ValidateWithWarnings validates known fields and returns non-fatal warnings
// for keys that have no validator. Validate keeps its original two-return
// signature for existing callers that do not need warnings.
func (v *Validator) ValidateWithWarnings(data map[string]interface{}) (map[string]interface{}, map[string]string, []string) {
	result, errors := v.Validate(data)
	return result, errors, v.unexpectedKeyWarnings(data, "")
}

func (v *Validator) unexpectedKeyWarnings(data map[string]interface{}, path string) []string {
	known := make(map[string]bool, len(v.fields)+len(v.conditionalValidators)*2)
	for name := range v.fields {
		known[name] = true
	}
	for _, conditional := range v.conditionalValidators {
		known[conditional.ConditionField] = true
		known[conditional.TargetField] = true
	}
	warnings := make([]string, 0)
	for key, value := range data {
		keyPath := key
		if path != "" {
			keyPath = path + "." + key
		}
		if !known[key] {
			warnings = append(warnings, fmt.Sprintf("found unexpected key %q", keyPath))
			continue
		}
		field := v.fields[key]
		if field == nil {
			continue
		}
		for _, rule := range field.rules {
			switch typedRule := rule.(type) {
			case ObjectRule:
				if object, ok := value.(map[string]interface{}); ok {
					warnings = append(warnings, typedRule.validator.unexpectedKeyWarnings(object, keyPath)...)
				}
			case ArrayRule:
				if typedRule.validator == nil {
					continue
				}
				if items, ok := value.([]interface{}); ok {
					for index, item := range items {
						if object, ok := item.(map[string]interface{}); ok {
							warnings = append(warnings, typedRule.validator.unexpectedKeyWarnings(object, fmt.Sprintf("%s[%d]", keyPath, index))...)
						}
					}
				}
			}
		}
	}
	sort.Strings(warnings)
	return warnings
}

// ValidateJSON validates a JSON byte array
func (v *Validator) ValidateJSON(jsonData []byte) (map[string]interface{}, map[string]string) {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, map[string]string{"_error": "invalId JSON: " + err.Error()}
	}
	return v.Validate(data)
}

// ValidateRequest validates request body
// Returns (data, error)
// If validation errors exist, error contains formatted validation errors as a map
// If no errors, error is nil
func (v *Validator) ValidateRequest(request interface{ Body() ([]byte, error) }) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	body, err := request.Body()
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	result, errors := v.Validate(data)
	if len(errors) > 0 {
		return nil, fmt.Errorf("validation errors: %v", errors)
	}
	return result, nil
}

// ToJSONSchema converts this validator to a JSON Schema draft 2020-12 object.
// Runtime-only Custom rules are omitted because they cannot be represented by
// portable JSON Schema keywords.
func (v *Validator) ToJSONSchema() map[string]interface{} {
	return v.toJSONSchema(true)
}

func (v *Validator) toJSONSchema(includeDialect bool) map[string]interface{} {
	properties := make(map[string]interface{}, len(v.fields))
	required := make([]string, 0)
	for name, field := range v.fields {
		properties[name] = field.toJSONSchema()
		if field.required {
			required = append(required, name)
		}
	}
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}
	if includeDialect {
		schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	}
	if v.description != "" {
		schema["description"] = v.description
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	if len(v.conditionalValidators) > 0 {
		allOf := make([]interface{}, 0, len(v.conditionalValidators))
		for _, conditional := range v.conditionalValidators {
			allOf = append(allOf, map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{conditional.ConditionField: map[string]interface{}{"const": conditional.ConditionValue}},
					"required":   []string{conditional.ConditionField},
				},
				"then": map[string]interface{}{
					"properties": map[string]interface{}{conditional.TargetField: conditional.Validator.toJSONSchema()},
				},
			})
		}
		schema["allOf"] = allOf
	}
	return schema
}

// ToJsonSchema is a compatibility spelling for callers that use Json rather
// than JSON in method names.
func (v *Validator) ToJsonSchema() map[string]interface{} { return v.ToJSONSchema() }

// ToJSONSchemaJSON returns an indented .json document for writing to disk or
// emitting from a CLI command.
func (v *Validator) ToJSONSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(v.ToJSONSchema(), "", "  ")
}

func (f *FieldValidator) toJSONSchema() map[string]interface{} {
	schema := make(map[string]interface{})
	if f.description != "" {
		schema["description"] = f.description
	}
	for _, rule := range f.rules {
		switch r := rule.(type) {
		case StringRule:
			schema["type"] = "string"
		case NumberRule:
			schema["type"] = "number"
		case IntegerRule:
			schema["type"] = "integer"
		case BoolRule:
			schema["type"] = "boolean"
		case MinLengthRule:
			schema["minLength"] = r.length
		case MaxLengthRule:
			schema["maxLength"] = r.length
		case MinRule:
			schema["minimum"] = r.value
		case MaxRule:
			schema["maximum"] = r.value
		case RegexRule:
			schema["pattern"] = r.pattern.String()
		case DecimalRule:
			schema["multipleOf"] = decimalMultiple(r.decimalPlaces)
		case EnumRule:
			schema["enum"] = r.allowedValues
		case NotEnumRule:
			schema["not"] = map[string]interface{}{"enum": r.disallowedValues}
		case EmailRule:
			schema["type"], schema["format"] = "string", "email"
		case DateRule:
			schema["type"], schema["format"] = "string", "date"
		case DateTimeRule:
			schema["type"], schema["format"] = "string", "date-time"
		case URLRule:
			schema["type"], schema["format"] = "string", "uri"
		case UUIDRule:
			schema["type"], schema["format"] = "string", "uuid"
		case IPv4Rule:
			schema["type"], schema["format"] = "string", "ipv4"
		case IPv6Rule:
			schema["type"], schema["format"] = "string", "ipv6"
		case IPRule:
			schema["type"], schema["format"] = "string", "ip"
		case DomainRule:
			schema["type"], schema["format"] = "string", "hostname"
		case ArrayRule:
			schema["type"] = "array"
			if r.validator != nil {
				schema["items"] = r.validator.toJSONSchema(false)
			}
		case ObjectRule:
			for key, value := range r.validator.toJSONSchema(false) {
				schema[key] = value
			}
		case NotStringRule:
			schema["not"] = map[string]interface{}{"type": "string"}
		case NotNumberRule:
			schema["not"] = map[string]interface{}{"type": "number"}
		}
	}
	return schema
}

func decimalMultiple(places int) float64 {
	multiple := 1.0
	for i := 0; i < places; i++ {
		multiple /= 10
	}
	return multiple
}
