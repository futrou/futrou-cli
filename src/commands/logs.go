package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// logKind describes a resource whose logs can be browsed with "futrou logs".
type logKind struct {
	path     string   // collection path, e.g. /v2/serverlets
	nameKey  []string // fields matched against a name argument, besides the ID
	hasTail  bool     // the API has a /logs/tail endpoint
	tailOmit []string // query parameters /logs/tail does not accept
}

var logKinds = map[string]logKind{
	"serverlet": {"/v2/serverlets", []string{"name"}, false, nil},
	"proxy":     {"/v2/proxies", []string{"domain", "name"}, true, nil},
	"dns":       {"/v2/dns", []string{"domain", "name"}, true, nil},
	"storage":   {"/v2/storages", []string{"name"}, true, []string{"offset", "limit"}},
	"server":    {"/v2/servers", []string{"name", "hostname"}, false, nil},
	"monitor":   {"/v2/monitors", []string{"name"}, true, nil},
}

var logsPollInterval = 2 * time.Second

var logsCommand = &cli.Command{
	Name:      "logs",
	Usage:     "Browse logs of all services",
	ArgsUsage: "<name-or-id>",
	Description: "Looks the resource up by name or ID (serverlets by default) and prints its log entries.\n" +
		"   Use --type to pick another resource, --instance for a single serverlet instance,\n" +
		"   and --live=false to print once and exit.",
	Flags: append([]cli.Flag{
		&cli.StringFlag{Name: "type", Aliases: []string{"t"}, Value: "serverlet", Usage: "Resource type: serverlet, proxy, dns, storage, server, monitor"},
		&cli.StringFlag{Name: "instance", Usage: "Serverlet instance ID"},
		&cli.BoolFlag{Name: "ls", Usage: "List the resources of --type whose logs can be browsed"},
	}, logFlags...),
	Action: func(c *cli.Context) error {
		kind, ok := logKinds[strings.ToLower(c.String("type"))]
		if !ok {
			return fmt.Errorf("unknown --type %q (serverlet, proxy, dns, storage, server, monitor)", c.String("type"))
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		var items []map[string]interface{}
		status, err := client.RequestInto("GET", kind.path, nil, &items)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if c.Bool("ls") {
			if isJSON(c) {
				return printJSON(items)
			}
			printTable(items, append([]string{"id"}, kind.nameKey...))
			return nil
		}

		ref := c.Args().First()
		if ref == "" {
			return fmt.Errorf("resource name or ID required (see --ls)")
		}
		id, err := matchLogResource(items, kind, ref)
		if err != nil {
			return err
		}
		path := kind.path + "/" + id
		if instance := c.String("instance"); instance != "" {
			if c.String("type") != "serverlet" {
				return fmt.Errorf("--instance only applies to --type serverlet")
			}
			path += "/instances/" + instance
		}
		return streamLogs(c, client, path, logOptions{hasTail: kind.hasTail && c.String("instance") == "", tailOmit: kind.tailOmit})
	},
}

// recentLogEntries is how many entries --tail keeps when the API has no
// dedicated /logs/tail endpoint for the resource.
const recentLogEntries = 100

// logOptions describes how the logs of one resource type can be read.
type logOptions struct {
	hasTail   bool       // the API has a /logs/tail endpoint
	forceTail bool       // behave as if --tail was passed and never stream
	tailOmit  []string   // query parameters the /logs/tail endpoint does not accept
	extra     url.Values // additional query parameters for this resource type
}

// streamLogs prints the logs of the resource at base (e.g. /v2/proxies/<id>).
// --tail limits output to recent entries, using the API's /logs/tail endpoint
// when opts.hasTail is set; --live (the default) keeps polling and prints
// only new entries.
func streamLogs(c *cli.Context, client *services.ApiClient, base string, opts logOptions) error {
	live := c.Bool("live") && !opts.forceTail
	recent := opts.forceTail || c.Bool("tail") || live
	path := base + "/logs"
	query := logQuery(c)
	for k, v := range opts.extra {
		query[k] = v
	}
	if recent && opts.hasTail {
		path += "/tail"
		for _, k := range opts.tailOmit {
			query.Del(k)
		}
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	seen := map[string]bool{}
	for {
		var entries interface{}
		status, err := client.RequestInto("GET", path, nil, &entries)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if list, ok := entries.([]interface{}); ok && recent && !opts.hasTail && len(list) > recentLogEntries {
			entries = list[len(list)-recentLogEntries:]
		}
		if err := printLogEntries(c, entries, seen); err != nil {
			return err
		}
		if !live {
			return nil
		}
		time.Sleep(logsPollInterval)
	}
}

// resourceLogsAction returns the Action for "<resource> logs <id>".
func resourceLogsAction(label, collection string, opts logOptions) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("%s ID required", label)
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		return streamLogs(c, client, collection+"/"+id, opts)
	}
}

// matchLogResource returns the ID of the single item whose ID or one of its
// name fields equals ref.
func matchLogResource(items []map[string]interface{}, kind logKind, ref string) (string, error) {
	var ids []string
	for _, item := range items {
		id, _ := item["id"].(string)
		if id == ref {
			return id, nil
		}
		for _, key := range kind.nameKey {
			if v, _ := item[key].(string); v == ref {
				ids = append(ids, id)
				break
			}
		}
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no resource named %q found", ref)
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%q matches %d resources, use an ID: %s", ref, len(ids), strings.Join(ids, ", "))
}

// printLogEntries prints entries not already in seen. Entries that look like
// {timestamp, message} objects are rendered as text lines; anything else is
// printed as JSON. With --log-format json every entry is a JSON line.
func printLogEntries(c *cli.Context, entries interface{}, seen map[string]bool) error {
	list, ok := entries.([]interface{})
	if !ok {
		if seen["_raw"] {
			return nil
		}
		seen["_raw"] = true
		return printJSON(entries)
	}
	for _, entry := range list {
		raw, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if seen[string(raw)] {
			continue
		}
		seen[string(raw)] = true
		if isJSON(c) {
			fmt.Println(string(raw))
			continue
		}
		fmt.Println(formatLogEntry(entry))
	}
	return nil
}

func formatLogEntry(entry interface{}) string {
	obj, ok := entry.(map[string]interface{})
	if !ok {
		return fmt.Sprint(entry)
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := obj[k]; ok && v != nil && fmt.Sprint(v) != "" {
				return fmt.Sprint(v)
			}
		}
		return ""
	}
	message := pick("message", "line", "text", "log", "msg")
	if message == "" {
		raw, _ := json.Marshal(obj)
		return string(raw)
	}
	if ts := pick("timestamp", "time", "createdAt", "at"); ts != "" {
		return ts + " " + message
	}
	return message
}
