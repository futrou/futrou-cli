// go run ./cmd/generate generates src/api/types.go from the live Futrou OpenAPI spec.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	openapiURL = "https://api.futrou.com/v2/openapi.json"
	outputFile = "src/api/types.go"
)

// root schemas — the generator will transitively include all $ref dependencies
var rootSchemas = map[string]bool{
	"User":              true,
	"ApiToken":          true,
	"Serverlet":         true,
	"ServerletInstance": true,
	"ServerletPlan":     true,
	"Project":           true,
	"Workspace":         true,
	"Variable":          true,
	"Volume":            true,
	"Proxy":             true,
	"Region":            true,
}

type openapiSpec struct {
	Components struct {
		Schemas map[string]schemaObj `json:"schemas"`
	} `json:"components"`
}

type schemaObj struct {
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Properties  map[string]propertyDef `json:"properties"`
	Required    []string             `json:"required"`
	Items       *propertyDef         `json:"items"`
	Ref         string               `json:"$ref"`
	Nullable    bool                 `json:"nullable"`
	Format      string               `json:"format"`
	AdditionalProperties *propertyDef `json:"additionalProperties"`
}

type propertyDef struct {
	Type                 string               `json:"type"`
	Format               string               `json:"format"`
	Nullable             bool                 `json:"nullable"`
	Ref                  string               `json:"$ref"`
	Items                *propertyDef         `json:"items"`
	AdditionalProperties *propertyDef         `json:"additionalProperties"`
	Description          string               `json:"description"`
}

func toPascalCase(s string) string {
	// Insert underscore before uppercase runs in camelCase, then title-case each part
	re := regexp.MustCompile(`([a-z])([A-Z])`)
	snake := re.ReplaceAllString(s, `${1}_${2}`)
	parts := strings.Split(snake, "_")
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

func propToGoType(p propertyDef, schemas map[string]schemaObj) string {
	if p.Ref != "" {
		parts := strings.Split(p.Ref, "/")
		return parts[len(parts)-1]
	}
	switch p.Type {
	case "string":
		if p.Format == "date-time" || p.Format == "date" {
			if p.Nullable {
				return "*time.Time"
			}
			return "time.Time"
		}
		return "string"
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		if p.Items == nil {
			return "[]interface{}"
		}
		return "[]" + propToGoType(*p.Items, schemas)
	case "object":
		if p.AdditionalProperties != nil {
			return "map[string]" + propToGoType(*p.AdditionalProperties, schemas)
		}
		return "map[string]interface{}"
	default:
		if p.Nullable {
			return "interface{}"
		}
		return "interface{}"
	}
}

func schemaToGoType(p propertyDef, schemas map[string]schemaObj) string {
	t := propToGoType(p, schemas)
	if p.Nullable && !strings.HasPrefix(t, "*") && !strings.HasPrefix(t, "[]") && !strings.HasPrefix(t, "map[") && t != "interface{}" {
		return "*" + t
	}
	return t
}

func generateStruct(name string, s schemaObj, schemas map[string]schemaObj) string {
	var sb strings.Builder

	if s.Description != "" {
		desc := strings.ReplaceAll(s.Description, "\n", "\n// ")
		sb.WriteString(fmt.Sprintf("// %s %s\n", name, desc))
	}

	sb.WriteString(fmt.Sprintf("type %s struct {\n", name))

	// Sort fields for stable output
	fields := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	for _, fieldName := range fields {
		prop := s.Properties[fieldName]
		goName := toPascalCase(fieldName)

		p := propertyDef{
			Type:                 prop.Type,
			Format:               prop.Format,
			Nullable:             prop.Nullable,
			Ref:                  prop.Ref,
			Items:                prop.Items,
			AdditionalProperties: prop.AdditionalProperties,
		}
		goType := schemaToGoType(p, schemas)
		// Use pointer for nested struct types to break potential circular references
		if !strings.HasPrefix(goType, "*") && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") && goType != "string" && goType != "int" && goType != "float64" && goType != "bool" && goType != "interface{}" && goType != "time.Time" {
			goType = "*" + goType
		}

		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s,omitempty\"`\n", goName, goType, fieldName))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// collectSchemas returns the set of all schema names needed (roots + transitive $ref deps).
func collectSchemas(roots map[string]bool, schemas map[string]schemaObj) map[string]bool {
	result := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if result[name] {
			return
		}
		if _, ok := schemas[name]; !ok {
			return
		}
		result[name] = true
		s := schemas[name]
		for _, p := range s.Properties {
			visitProp(p, schemas, result, visit)
		}
	}
	for name := range roots {
		visit(name)
	}
	return result
}

func visitProp(p propertyDef, schemas map[string]schemaObj, result map[string]bool, visit func(string)) {
	if p.Ref != "" {
		parts := strings.Split(p.Ref, "/")
		visit(parts[len(parts)-1])
	}
	if p.Items != nil {
		visitProp(*p.Items, schemas, result, visit)
	}
	if p.AdditionalProperties != nil {
		visitProp(*p.AdditionalProperties, schemas, result, visit)
	}
}

func main() {
	fmt.Println("Fetching OpenAPI spec from", openapiURL)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(openapiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching spec: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing spec: %v\n", err)
		os.Exit(1)
	}

	var sb strings.Builder

	sb.WriteString("// Code generated by cmd/generate from the Futrou OpenAPI spec. DO NOT EDIT.\n")
	sb.WriteString(fmt.Sprintf("// Generated at: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString("// Source: " + openapiURL + "\n")
	sb.WriteString("\n")
	sb.WriteString("package api\n")
	sb.WriteString("\n")

	// Collect all schemas to generate (root + transitive $ref dependencies)
	toGenerate := collectSchemas(rootSchemas, spec.Components.Schemas)

	// Check if any date-time fields exist to decide on time import
	needsTime := false
	for name := range toGenerate {
		s := spec.Components.Schemas[name]
		for _, p := range s.Properties {
			if p.Format == "date-time" || p.Format == "date" {
				needsTime = true
				break
			}
		}
	}
	if needsTime {
		sb.WriteString("import \"time\"\n\n")
	}

	// Fixed types not in OpenAPI schemas
	sb.WriteString(`// APIError represents a structured error response from the Futrou API.
type APIError struct {
	Message   string       ` + "`json:\"message\"`" + `
	RequestId string       ` + "`json:\"requestId\"`" + `
	ClientIp  string       ` + "`json:\"clientIp\"`" + `
	Errors    []FieldError ` + "`json:\"errors\"`" + `
}

func (e *APIError) Error() string {
	return e.Message
}

// FieldError is a single validation error within an APIError.
type FieldError struct {
	Message string ` + "`json:\"message\"`" + `
	Code    string ` + "`json:\"code\"`" + `
	Field   string ` + "`json:\"field\"`" + `
}

// LoginResponse is returned from POST /v2/auth/login.
type LoginResponse struct {
	ApiToken ApiToken ` + "`json:\"apiToken\"`" + `
	User     User     ` + "`json:\"user\"`" + `
}

`)

	// Generate structs in sorted order for stable output
	names := make([]string, 0, len(toGenerate))
	for name := range toGenerate {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		s := spec.Components.Schemas[name]
		sb.WriteString(generateStruct(name, s, spec.Components.Schemas))
		sb.WriteString("\n")
	}

	if err := os.WriteFile(outputFile, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Generated", outputFile)
}
