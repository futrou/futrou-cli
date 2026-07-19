package commands

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/gorilla/websocket"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var serverletsCommand = &cli.Command{
	Name:  "serverlets",
	Usage: "Manage serverlets",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all serverlets",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "image", "state", "instances", "minInstances", "maxInstances", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a serverlet by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		{
			Name:  "create",
			Usage: "Create a new serverlet",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Serverlet name"},
				&cli.StringFlag{Name: "image", Required: true, Usage: "Container image"},
				&cli.StringFlag{Name: "plan", Usage: "Serverlet plan ID"},
				&cli.IntFlag{Name: "min", Value: 1, Usage: "Minimum instances"},
				&cli.IntFlag{Name: "max", Value: 1, Usage: "Maximum instances"},
			},
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":         c.String("name"),
					"image":        c.String("image"),
					"minInstances": c.Int("min"),
					"maxInstances": c.Int("max"),
				}
				if p := c.String("plan"); p != "" {
					body["serverletPlanId"] = p
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/serverlets", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Serverlet created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a serverlet",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "New name"},
				&cli.StringFlag{Name: "image", Usage: "New image"},
				&cli.IntFlag{Name: "min", Usage: "Minimum instances"},
				&cli.IntFlag{Name: "max", Usage: "Maximum instances"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if v := c.String("image"); v != "" {
					body["image"] = v
				}
				if c.IsSet("min") {
					body["minInstances"] = c.Int("min")
				}
				if c.IsSet("max") {
					body["maxInstances"] = c.Int("max")
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/serverlets/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Serverlet updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a serverlet",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/serverlets/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Serverlet deleted")
				return nil
			},
		},
		{
			Name:      "start",
			Usage:     "Start a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("start"),
		},
		{
			Name:      "stop",
			Usage:     "Stop a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("stop"),
		},
		{
			Name:      "restart",
			Usage:     "Restart a serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("restart"),
		},
		{
			Name:      "logs",
			Usage:     "View serverlet logs",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id+"/logs", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		{
			Name:      "instances",
			Usage:     "List serverlet instances",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("serverlet ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/serverlets/"+id+"/instances", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "state", "cpu", "ram", "createdAt"})
				return nil
			},
		},
		shellCommand,
		execCommand,
	},
}

func serverletAction(action string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("serverlet ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		status, err := client.RequestInto("POST", "/v2/serverlets/"+id+"/"+action, nil, nil)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if isJSON(c) {
			return printJSON(map[string]string{"status": action})
		}
		fmt.Printf("✓ Serverlet %s\n", action)
		return nil
	}
}

const shellMaxReconnects = 3

var shellCommand = &cli.Command{
	Name:      "shell",
	Usage:     "Open an interactive shell in a serverlet",
	ArgsUsage: "<id>",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "instance", Usage: "Connect to a specific instance ID instead of a random one"},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("serverlet ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}

		wsURL, err := shellWebsocketURLFor(client, id, c.String("instance"))
		if err != nil {
			return err
		}

		return runShell(wsURL, client.ApiToken())
	},
}

// shellWebsocketURLFor builds the shell WebSocket URL for a serverlet,
// optionally targeting a specific instance.
func shellWebsocketURLFor(client *services.ApiClient, serverletID, instanceID string) (string, error) {
	path := "/v2/serverlets/" + serverletID + "/shell"
	if instanceID != "" {
		path = "/v2/serverlets/" + serverletID + "/instances/" + instanceID + "/shell"
	}
	return shellWebsocketURL(client.ApiUrl(), path)
}

// shellWebsocketURL converts an http(s) API base URL + path into a ws(s) URL.
func shellWebsocketURL(apiUrl, path string) (string, error) {
	u, err := url.Parse(apiUrl)
	if err != nil {
		return "", fmt.Errorf("parsing API URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported API URL scheme: %s", u.Scheme)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	return u.String(), nil
}

// runShell connects to the shell WebSocket and wires it up to the local
// terminal, retrying up to shellMaxReconnects times only when the
// connection can't be established in the first place. Once a session is
// open, its end — clean or abrupt, server-initiated or user-initiated via
// "~." — is treated as the shell exiting normally, not a failure to retry.
func runShell(wsURL, token string) error {
	if !isInteractiveTerminal() {
		return fmt.Errorf("shell requires an interactive terminal")
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("entering raw terminal mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	conn, err := dialShellWithRetry(wsURL, token)
	if err != nil {
		return err
	}
	defer conn.Close()

	pumpShell(conn)
	return nil
}

// dialShellWithRetry dials the shell WebSocket, retrying up to
// shellMaxReconnects times when the connection can't be established.
func dialShellWithRetry(wsURL, token string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Cookie", "authorization="+token)

	var lastErr error
	for attempt := 0; attempt <= shellMaxReconnects; attempt++ {
		if attempt > 0 {
			logger.StartLoader(fmt.Sprintf("Reconnecting (%d/%d)...", attempt, shellMaxReconnects))
			time.Sleep(reconnectBackoff(attempt))
		}

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			lastErr = fmt.Errorf("connecting to shell: %w", err)
			continue
		}
		if attempt > 0 {
			logger.StopLoader()
		}
		return conn, nil
	}

	logger.StopLoader()
	if lastErr == nil {
		lastErr = fmt.Errorf("shell connection failed")
	}
	return nil, fmt.Errorf("could not connect to shell after %d attempts: %w", shellMaxReconnects+1, lastErr)
}

func reconnectBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * time.Second
}

// escapeState tracks progress through the SSH-style "~." disconnect escape:
// the sequence is only recognized right after a newline, so ordinary input
// (including Ctrl+C, which always reaches the remote shell) never triggers
// it by accident.
type escapeState int

const (
	escNone  escapeState = iota // not at a fresh line
	escLine                     // at start of a line
	escTilde                    // just saw "~" at start of a line
)

// pumpShell wires the WebSocket connection to stdin/stdout until the session
// ends — the server closes it (including abruptly, e.g. the remote shell
// process exiting), or the user disconnects via the "~." escape. Either way
// it simply returns; the caller does not retry an already-open session.
//
// A second "~." sent while a graceful close is already in flight force-exits
// the process immediately, in case the close hangs.
func pumpShell(conn *websocket.Conn) {
	done := make(chan struct{})
	var disconnecting bool
	state := escLine

	go func() {
		defer close(done)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
				os.Stdout.Write(data)
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			for i := 0; i < n; i++ {
				b := buf[i]

				switch state {
				case escLine:
					if b == '~' {
						state = escTilde
						continue // hold back; may be part of the escape
					}
				case escTilde:
					if b == '.' {
						if disconnecting {
							// Second "~.": the graceful close hasn't
							// finished yet, so bail out immediately.
							term.Restore(int(os.Stdin.Fd()), nil)
							os.Exit(130)
						}
						disconnecting = true
						conn.WriteControl(websocket.CloseMessage,
							websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
							time.Now().Add(time.Second))
						state = escLine
						continue
					}
					// Not a disconnect: forward the held-back "~" first.
					if !disconnecting {
						conn.WriteMessage(websocket.BinaryMessage, []byte{'~'})
					}
				}

				state = escNone
				if b == '\r' || b == '\n' {
					state = escLine
				}

				if !disconnecting {
					if werr := conn.WriteMessage(websocket.BinaryMessage, []byte{b}); werr != nil {
						return
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	<-done
}

var execCommand = &cli.Command{
	Name:      "exec",
	Usage:     "Run a command in a serverlet and wait for it to exit",
	ArgsUsage: "<id> -- <command>",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "instance", Usage: "Run in a specific instance ID instead of a random one"},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("serverlet ID required")
		}
		args := c.Args().Tail()
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		command := strings.Join(args, " ")
		if command == "" {
			return fmt.Errorf("command required, e.g. futrou serverlets exec <id> -- <command>")
		}

		client, err := requireAuth(c)
		if err != nil {
			return err
		}

		wsURL, err := shellWebsocketURLFor(client, id, c.String("instance"))
		if err != nil {
			return err
		}

		code, err := runExec(wsURL, client.ApiToken(), command)
		if err != nil {
			return err
		}
		os.Exit(code)
		return nil
	},
}

// runExec connects to the shell WebSocket, runs command non-interactively,
// streams its combined stdout/stderr to the local terminal as it arrives
// (the remote PTY interleaves them, so they can't be separated), and
// returns the command's exit code once it finishes.
//
// The exit code is recovered by appending a sentinel echo after the
// command; the sentinel line is parsed out of the stream and never shown.
func runExec(wsURL, token, command string) (int, error) {
	sentinel := fmt.Sprintf("__FUTROU_EXIT_%d__", rand.Int63())

	conn, err := dialShellWithRetry(wsURL, token)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	payload := command + fmt.Sprintf("; echo %s:$?\n", sentinel)
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte(payload)); err != nil {
		return 0, fmt.Errorf("sending command: %w", err)
	}

	code, err := pumpExec(conn, sentinel)
	if err != nil {
		return 0, err
	}
	return code, nil
}

// pumpExec reads the shell's output stream until it finds a line containing
// "sentinel:<exit code>", writing everything else to stdout as it arrives.
// It returns the parsed exit code.
func pumpExec(conn *websocket.Conn, sentinel string) (int, error) {
	var pending strings.Builder

	flushLine := func(line string) {
		fmt.Fprintln(os.Stdout, line)
	}

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return 0, fmt.Errorf("command did not complete: %w", err)
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}

		pending.WriteString(string(data))
		buf := pending.String()

		for {
			idx := strings.IndexAny(buf, "\r\n")
			if idx < 0 {
				break
			}
			line := buf[:idx]
			rest := buf[idx+1:]
			// Collapse a \r\n pair.
			if idx+1 < len(buf) && buf[idx] == '\r' && buf[idx+1] == '\n' {
				rest = buf[idx+2:]
			}

			if code, ok := parseSentinelLine(line, sentinel); ok {
				if rem := strings.TrimRight(rest, "\r\n"); rem != "" {
					flushLine(rem)
				}
				return code, nil
			}

			flushLine(line)
			buf = rest
		}
		pending.Reset()
		pending.WriteString(buf)
	}
}

// parseSentinelLine checks whether line is "<sentinel>:<exit code>" and
// returns the parsed code.
func parseSentinelLine(line, sentinel string) (int, bool) {
	prefix := sentinel + ":"
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return 0, false
	}
	rest := line[idx+len(prefix):]
	rest = strings.TrimSpace(rest)
	code, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return code, true
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
	for i, col := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.Repeat("-", widths[i]+len(col)-len(col)))
		_ = col
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
