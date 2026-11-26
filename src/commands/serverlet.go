package commands

import (
	"errors"
	"fmt"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"
	"futrou-cli/src/utils"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var serverletCommand = &cli.Command{
	Name:  "serverlet",
	Usage: "Manage serverlets",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List serverlets in a project (defaults to futrou.json|yaml, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{workspaceFlag, projectFlag},
			Action: func(c *cli.Context) error {
				query, err := projectQuery(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				_, err = client.RequestInto("GET", "/v2/serverlets"+query, nil, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "image", "status", "serverletPlanId", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a serverlet by ID",
			ArgsUsage: "<id>",
			Action:    showAction("serverlet", "/v2/serverlets"),
		},
		{
			Name:  "create",
			Usage: "Create a new serverlet (defaults to futrou.json|yaml, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Serverlet name"},
				&cli.StringFlag{Name: "image", Required: true, Usage: "Container image"},
				&cli.StringFlag{Name: "plan", Required: true, Usage: "Serverlet plan ID"},
				workspaceFlag,
				projectFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, projectID, err := resolveScope(c)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":            c.String("name"),
					"image":           c.String("image"),
					"serverletPlanId": c.String("plan"),
					"workspaceId":     workspaceID,
					"projectId":       projectID,
				}
				var result interface{}
				_, err = client.RequestInto("POST", "/v2/serverlets", body, &result)
				if err != nil {
					return err
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
				&cli.StringFlag{Name: "plan", Usage: "New serverlet plan ID"},
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
				if v := c.String("plan"); v != "" {
					body["serverletPlanId"] = v
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				_, err = client.RequestInto("PATCH", "/v2/serverlets/"+id, body, &result)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Serverlet updated")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a serverlet",
			ArgsUsage: "<id>",
			Action:    rmAction("serverlet", "/v2/serverlets"),
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
			Name:      "wake",
			Usage:     "Wake a sleeping serverlet",
			ArgsUsage: "<id>",
			Action:    serverletAction("wake"),
		},
		{
			Name:      "logs",
			Usage:     "View serverlet logs",
			ArgsUsage: "<id>",
			Flags:     logFlags,
			Action:    resourceLogsAction("serverlet", "/v2/serverlets", logOptions{}),
		},
		{
			Name:      "metrics",
			Usage:     "Show serverlet metrics",
			ArgsUsage: "<id>",
			Action:    serverletGet("/metrics"),
		},
		{
			Name:  "instance",
			Usage: "Manage serverlet instances",
			Subcommands: []*cli.Command{
				{
					Name:      "ls",
					Usage:     "List serverlet instances",
					ArgsUsage: "<id>",
					Action:    serverletInstancesAction,
				},
				{
					Name:      "start",
					Usage:     "Start a serverlet instance",
					ArgsUsage: "<id> <instance-id>",
					Action:    serverletInstanceAction("start"),
				},
				{
					Name:      "stop",
					Usage:     "Stop a serverlet instance",
					ArgsUsage: "<id> <instance-id>",
					Action:    serverletInstanceAction("stop"),
				},
				{
					Name:      "restart",
					Usage:     "Restart a serverlet instance",
					ArgsUsage: "<id> <instance-id>",
					Action:    serverletInstanceAction("restart"),
				},
				{
					Name:      "logs",
					Usage:     "View logs of a serverlet instance",
					ArgsUsage: "<id> <instance-id>",
					Flags:     logFlags,
					Action: func(c *cli.Context) error {
						id, instanceID := c.Args().Get(0), c.Args().Get(1)
						if id == "" || instanceID == "" {
							return fmt.Errorf("serverlet ID and instance ID required")
						}
						client, err := requireAuth(c)
						if err != nil {
							return err
						}
						return streamLogs(c, client, "/v2/serverlets/"+id+"/instances/"+instanceID, logOptions{})
					},
				},
				{
					Name:      "metrics",
					Usage:     "Show metrics of a serverlet instance",
					ArgsUsage: "<id> <instance-id>",
					Action:    serverletInstanceGet("/metrics"),
				},
			},
			// "instance <id>" keeps working as a shorthand for "instance ls <id>".
			ArgsUsage: "<id>",
			Action:    serverletInstancesAction,
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
		_, err = client.RequestInto("POST", "/v2/serverlets/"+id+"/"+action, nil, nil)
		if err != nil {
			return err
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

var serverletInstancesAction = func(c *cli.Context) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("serverlet ID required")
	}
	client, err := requireAuth(c)
	if err != nil {
		return err
	}
	var result interface{}
	_, err = client.RequestInto("GET", "/v2/serverlets/"+id+"/instances", nil, &result)
	if err != nil {
		return err
	}
	if isJSON(c) {
		return printJSON(result)
	}
	printTable(result, []string{"id", "status", "createdAt"})
	return nil
}

// serverletGet prints the JSON response of GET /v2/serverlets/<id><suffix>.
func serverletGet(suffix string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("serverlet ID required")
		}
		return getJSON(c, "/v2/serverlets/"+id+suffix)
	}
}

// serverletInstanceGet prints the JSON response of
// GET /v2/serverlets/<id>/instances/<instance-id><suffix>.
func serverletInstanceGet(suffix string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id, instanceID := c.Args().Get(0), c.Args().Get(1)
		if id == "" || instanceID == "" {
			return fmt.Errorf("serverlet ID and instance ID required")
		}
		return getJSON(c, "/v2/serverlets/"+id+"/instances/"+instanceID+suffix)
	}
}

// serverletInstanceAction POSTs /v2/serverlets/<id>/instances/<instance-id>/<action>.
func serverletInstanceAction(action string) cli.ActionFunc {
	return func(c *cli.Context) error {
		id, instanceID := c.Args().Get(0), c.Args().Get(1)
		if id == "" || instanceID == "" {
			return fmt.Errorf("serverlet ID and instance ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		_, err = client.RequestInto("POST", "/v2/serverlets/"+id+"/instances/"+instanceID+"/"+action, nil, nil)
		if err != nil {
			return err
		}
		if isJSON(c) {
			return printJSON(map[string]string{"status": action})
		}
		fmt.Printf("✓ Instance %s\n", action)
		return nil
	}
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

// shellPingInterval is how often the CLI pings the server, like ssh's ServerAliveInterval, so an
// idle terminal keeps traffic flowing through Cloudflare and any NAT. A variable so tests shorten it.
var shellPingInterval = 25 * time.Second

// runShell connects to the shell WebSocket and wires it up to the local
// terminal, like ssh: when the connection is lost (network change, idle timeout
// of a proxy, a restart of Cloud or the node) it reconnects by itself, up to
// shellMaxReconnects times in a row, and the server hands back the same shell
// with what it printed meanwhile. A shell that exits, a server that ends the
// session on purpose or the "~." escape end it for good.
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

	return runShellSession(wsURL, token, newShellKeys(os.Stdin))
}

// runShellSession keeps one shell session connected: it dials, pumps, and dials again when
// the connection is lost, until the session is over.
func runShellSession(wsURL, token string, keys *shellKeys) error {
	conn, err := dialShellWithRetry(wsURL, token)
	if err != nil {
		return err
	}
	modes := &utils.TermModes{}
	for {
		end := pumpShellConn(conn, keys, modes)
		conn.Close()
		// What the remote program left switched on in this terminal (mouse reporting, the alternate
		// screen, ...) is undone, or a new shell would get mouse reports typed into it
		os.Stdout.Write(modes.Reset())
		if end == shellEnded {
			return nil
		}
		os.Stdout.WriteString("\r\n\x1b[33m⟳ Connection closed. Reconnecting...\x1b[0m\r\n")
		time.Sleep(reconnectBackoff(1))
		conn, err = dialShellWithRetry(wsURL, token)
		if err != nil {
			return err
		}
	}
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

// reconnectUnit is the delay added per reconnect attempt.
var reconnectUnit = time.Second

func reconnectBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * reconnectUnit
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

// shellEnd says why a connection ended.
type shellEnd int

const (
	shellEnded shellEnd = iota // the shell exited, the server ended it, or the user left: done
	shellLost                  // the connection closed without the shell ending: reconnect
)

// shellKeys reads the keyboard for as long as the shell is used and sends it to the
// current connection, which changes when the connection is replaced. Keys typed while
// there is no connection are dropped, as they would be in ssh.
type shellKeys struct {
	mu            sync.Mutex
	conn          *websocket.Conn
	state         escapeState
	disconnecting bool
	left          chan struct{} // closed once the user typed "~."
}

// newShellKeys starts reading stdin.
func newShellKeys(stdin *os.File) *shellKeys {
	k := &shellKeys{state: escLine, left: make(chan struct{})}
	go k.read(stdin)
	return k
}

func (k *shellKeys) setConn(conn *websocket.Conn) {
	k.mu.Lock()
	k.conn = conn
	k.mu.Unlock()
}

func (k *shellKeys) userLeft() bool {
	select {
	case <-k.left:
		return true
	default:
		return false
	}
}

// write sends data to the current connection; false when there is none or it failed.
func (k *shellKeys) write(data []byte) bool {
	if k.conn == nil {
		return false
	}
	return k.conn.WriteMessage(websocket.BinaryMessage, data) == nil
}

func (k *shellKeys) read(stdin *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := stdin.Read(buf)
		k.mu.Lock()
		for i := 0; i < n; i++ {
			b := buf[i]

			switch k.state {
			case escLine:
				if b == '~' {
					k.state = escTilde
					continue // hold back; may be part of the escape
				}
			case escTilde:
				if b == '.' {
					if k.disconnecting {
						// Second "~.": the graceful close hasn't
						// finished yet, so bail out immediately.
						term.Restore(int(stdin.Fd()), nil)
						os.Exit(130)
					}
					k.disconnecting = true
					close(k.left)
					if k.conn != nil {
						k.conn.WriteControl(websocket.CloseMessage,
							websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
							time.Now().Add(time.Second))
					}
					k.state = escLine
					continue
				}
				// Not a disconnect: forward the held-back "~" first.
				if !k.disconnecting {
					k.write([]byte{'~'})
				}
			}

			k.state = escNone
			if b == '\r' || b == '\n' {
				k.state = escLine
			}

			if !k.disconnecting {
				k.write([]byte{b})
			}
		}
		k.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// pumpShell wires one WebSocket connection to stdin/stdout until it ends and returns.
// Used where a single connection is all there is (the tests); runShell keeps one
// shellKeys across connections.
func pumpShell(conn *websocket.Conn) shellEnd {
	return pumpShellConn(conn, newShellKeys(os.Stdin), &utils.TermModes{})
}

// closeEndsShell reports whether a close frame means the session is over (the shell
// exited, the server gave up or ended it on purpose) rather than a lost connection.
func closeEndsShell(err error) bool {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return false
	}
	// Cloud tried its own retries and could not reach the node; the shell is kept there for a
	// while, so trying again from here is worth it (any other reason Cloud gives is final).
	if closeErr.Code == 4000 && closeErr.Text == "remote unavailable" {
		return false
	}
	switch closeErr.Code {
	case websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseInternalServerErr,
		websocket.CloseServiceRestart, websocket.CloseTryAgainLater, 1014:
		return false // the server or something on the way went away
	}
	return true
}

// pumpShellConn prints what the server sends and pings to keep the connection open until it
// closes: the server ends it, it drops, or the user disconnects via the "~." escape.
//
// A second "~." sent while a graceful close is already in flight force-exits
// the process immediately, in case the close hangs.
func pumpShellConn(conn *websocket.Conn, keys *shellKeys, modes *utils.TermModes) shellEnd {
	keys.setConn(conn)
	defer keys.setConn(nil)

	pingEvery := shellPingInterval // read once: tests change it

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(pingEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
			}
		}
	}()

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if keys.userLeft() || closeEndsShell(err) {
				return shellEnded
			}
			return shellLost
		}
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			modes.Feed(data)
			os.Stdout.Write(data)
		}
	}
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
