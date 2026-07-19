package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"futrou-cli/src/logger"

	"github.com/urfave/cli/v2"
)

// logFlags are the shared flags for endpoints that return raw log streams
// with offset/limit/search/startAt/endAt query parameters.
var logFlags = []cli.Flag{
	&cli.IntFlag{Name: "offset", Usage: "Offset into the log stream"},
	&cli.IntFlag{Name: "limit", Usage: "Maximum number of log entries to return"},
	&cli.StringFlag{Name: "search", Usage: "Filter logs by search term"},
	&cli.StringFlag{Name: "start-at", Usage: "Only return logs at or after this time"},
	&cli.StringFlag{Name: "end-at", Usage: "Only return logs at or before this time"},
}

// logQueryString builds the query string for logFlags, including the
// leading "?" when at least one flag is set.
func logQueryString(c *cli.Context) string {
	q := url.Values{}
	if c.IsSet("offset") {
		q.Set("offset", fmt.Sprintf("%d", c.Int("offset")))
	}
	if c.IsSet("limit") {
		q.Set("limit", fmt.Sprintf("%d", c.Int("limit")))
	}
	if v := c.String("search"); v != "" {
		q.Set("search", v)
	}
	if v := c.String("start-at"); v != "" {
		q.Set("startAt", v)
	}
	if v := c.String("end-at"); v != "" {
		q.Set("endAt", v)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

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
