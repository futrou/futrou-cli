// build-types fetches the Futrou OpenAPI schema and prints generated Go API
// types. It depends only on utils, so it can run before the CLI is compiled.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"futrou-cli/src/utils"
)

const defaultAPIURL = "https://api.futrou.com"

func main() {
	apiURL := strings.TrimRight(os.Getenv("FUTROU_API_URL"), "/")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	apiURL = strings.TrimSuffix(apiURL, "/v2")
	schemaURL := apiURL + "/v2/openapi.json"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(schemaURL)
	if err != nil {
		fatalf("fetching schema: %v", err)
	}
	defer resp.Body.Close()

	schema, err := io.ReadAll(resp.Body)
	if err != nil {
		fatalf("reading schema: %v", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		fatalf("fetching schema: API returned %s", resp.Status)
	}

	types, err := utils.JSONSchemaToGo(schema, schemaURL)
	if err != nil {
		fatalf("converting schema to Go types: %v", err)
	}
	if _, err := os.Stdout.Write(types); err != nil {
		fatalf("writing generated types: %v", err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "build-types: "+format+"\n", args...)
	os.Exit(1)
}
