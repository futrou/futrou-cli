package commands

import (
	"encoding/json"
	"fmt"
	"io"

	"futrou-cli/src/logger"

	"github.com/urfave/cli/v2"
)

// isJSON returns true when --log-format json is set at any level of the context chain.
func isJSON(c *cli.Context) bool {
	if c == nil {
		return false
	}
	for _, ctx := range append([]*cli.Context{c}, c.Lineage()...) {
		if ctx.String("log-format") == "json" {
			return true
		}
	}
	return false
}

// printJSON marshals v and prints it to stdout via the logger.
func printJSON(v interface{}) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	logger.Info("%s", string(out))
	return nil
}

type jsonError struct {
	Status int
	Body   interface{}
}

func (e *jsonError) Error() string {
	return fmt.Sprintf("request failed with status %d", e.Status)
}

func printJSONError(status int, body interface{}) error {
	return &jsonError{Status: status, Body: body}
}

func writeJSONError(w io.Writer, status int, body interface{}) {
	out, err := json.MarshalIndent(map[string]interface{}{
		"error":  body,
		"status": status,
	}, "", "  ")
	if err != nil {
		fmt.Fprintln(w, body)
		return
	}
	fmt.Fprintln(w, string(out))
}
