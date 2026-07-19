package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var schemaCommand = &cli.Command{
	Name:  "schema",
	Usage: "Display the Futrou API v2 OpenAPI schema",
	Action: func(c *cli.Context) error {
		apiUrl := services.NormalizeApiUrl(globalApiUrl(c))
		url := apiUrl + "/v2/openapi.json"

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return fmt.Errorf("fetching schema: %w", err)
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading schema: %w", err)
		}

		if isJSON(c) {
			// Already JSON — print raw
			fmt.Println(string(data))
			return nil
		}

		// Pretty-print
		var pretty interface{}
		if err := json.Unmarshal(data, &pretty); err != nil {
			return fmt.Errorf("parsing schema: %w", err)
		}
		return printJSON(pretty)
	},
}
