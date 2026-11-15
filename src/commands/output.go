package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

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
	&cli.BoolFlag{Name: "tail", Usage: "Show only the most recent log entries"},
	&cli.BoolFlag{Name: "live", Usage: "Keep streaming new log entries until interrupted"},
}

// logQuery builds the query parameters for logFlags (tail and live are
// handled by streamLogs).
func logQuery(c *cli.Context) url.Values {
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
	return q
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

// metricsAction returns the Action for "<resource> metrics <id>", which
// prints GET <collection>/<id>/metrics.
func metricsAction(label, collection string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("%s ID required", label)
		}
		return getJSON(c, collection+"/"+id+"/metrics")
	}
}

// showAction returns the Action for "<resource> show <id>", which prints
// GET <collection>/<id>.
func showAction(label, collection string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("%s ID required", label)
		}
		return getJSON(c, collection+"/"+id)
	}
}

// rmAction returns the Action for "<resource> rm <id>", which sends
// DELETE <collection>/<id>.
func rmAction(label, collection string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("%s ID required", label)
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		if _, err := client.RequestInto("DELETE", collection+"/"+id, nil, nil); err != nil {
			return err
		}
		if isJSON(c) {
			return printJSON(map[string]string{"status": "deleted"})
		}
		fmt.Printf("✓ %s deleted\n", upperFirst(label))
		return nil
	}
}

// upperFirst upper-cases the first letter, so "network" reads "Network" while
// "DNS zone" stays as it is.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// printTable prints a slice of maps as an aligned table with the given columns.
func printTable(data interface{}, cols []string) {
	raw, err := json.Marshal(data)
	if err != nil {
		fmt.Println(data)
		return
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		// maybe it's wrapped in a data field
		var wrapped struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil || len(wrapped.Data) == 0 {
			fmt.Println(string(raw))
			return
		}
		rows = wrapped.Data
	}

	if len(rows) == 0 {
		fmt.Println("No results.")
		return
	}

	// calculate column widths
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = len(col)
	}
	for _, row := range rows {
		for i, col := range cols {
			v := fmt.Sprintf("%v", row[col])
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}

	// header
	for i, col := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.ToUpper(col))
		fmt.Print(strings.Repeat(" ", widths[i]-len(col)))
	}
	fmt.Println()

	// separator
	for i := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.Repeat("-", widths[i]))
	}
	fmt.Println()

	// rows
	for _, row := range rows {
		for i, col := range cols {
			if i > 0 {
				fmt.Print("  ")
			}
			v := fmt.Sprintf("%v", row[col])
			fmt.Print(v)
			fmt.Print(strings.Repeat(" ", widths[i]-len(v)))
		}
		fmt.Println()
	}
}

func getJSON(c *cli.Context, path string) error {
	client, err := requireAuth(c)
	if err != nil {
		return err
	}
	var result interface{}
	_, err = client.RequestInto("GET", path, nil, &result)
	if err != nil {
		return err
	}
	return printJSON(result)
}
