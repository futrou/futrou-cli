package commands

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// reconnectServer is a shell endpoint that runs a different script for each connection and
// counts them.
type reconnectServer struct {
	*httptest.Server
	mu    sync.Mutex
	count atomic.Int32
}

func newReconnectServer(t *testing.T, scripts ...func(conn *websocket.Conn)) *reconnectServer {
	t.Helper()
	s := &reconnectServer{}
	upgrader := websocket.Upgrader{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.mu.Unlock()
		n := int(s.count.Add(1)) - 1
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if n < len(scripts) {
			scripts[n](conn)
		}
	}))
	t.Cleanup(s.Server.Close)
	return s
}

func (s *reconnectServer) url() string {
	return "ws" + strings.TrimPrefix(s.URL, "http") + "/v2/serverlets/sl-1/shell"
}

func fastShellTimes(t *testing.T) {
	t.Helper()
	reconnectUnit = time.Millisecond
	oldPing := shellPingInterval
	t.Cleanup(func() {
		reconnectUnit = time.Second
		shellPingInterval = oldPing
	})
}

// dropped ends a connection the way a network does: no close frame.
func dropped(conn *websocket.Conn) { conn.UnderlyingConn().Close() }

func finished(conn *websocket.Conn) {
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "remote closed"), time.Now().Add(time.Second))
}

func runSession(t *testing.T, s *reconnectServer) (string, error) {
	t.Helper()
	withPipedStdin(t)
	var err error
	out := captureStdout(t, func() { err = runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) })
	return out, err
}

// The reported bug: the connection was lost and the shell was just over. Now it dials again.
func TestShellReconnectsAfterALostConnection(t *testing.T) {
	fastShellTimes(t)
	s := newReconnectServer(t,
		func(conn *websocket.Conn) {
			conn.WriteMessage(websocket.BinaryMessage, []byte("first\r\n"))
			dropped(conn)
		},
		func(conn *websocket.Conn) {
			conn.WriteMessage(websocket.BinaryMessage, []byte("second\r\n"))
			finished(conn)
		},
	)
	out, err := runSession(t, s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") || !strings.Contains(out, "Reconnecting") {
		t.Fatalf("output = %q", out)
	}
	if s.count.Load() != 2 {
		t.Fatalf("connected %d times, want 2", s.count.Load())
	}
}

func TestShellDoesNotReconnectWhenTheShellExits(t *testing.T) {
	fastShellTimes(t)
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.BinaryMessage, []byte("bye\r\n"))
		finished(conn)
	})
	out, err := runSession(t, s)
	if err != nil || !strings.Contains(out, "bye") || strings.Contains(out, "Reconnecting") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if s.count.Load() != 1 {
		t.Fatalf("connected %d times", s.count.Load())
	}
}

func TestShellReconnectsWhenCloudCouldNotReachTheNodeButNotForOtherErrors(t *testing.T) {
	fastShellTimes(t)
	closeWith := func(text string) func(*websocket.Conn) {
		return func(conn *websocket.Conn) {
			time.Sleep(60 * time.Millisecond)
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4000, text), time.Now().Add(time.Second))
		}
	}
	s := newReconnectServer(t, closeWith("remote unavailable"), finished)
	if _, err := runSession(t, s); err != nil || s.count.Load() != 2 {
		t.Fatalf("remote unavailable: err = %v, connections = %d, want a second one", err, s.count.Load())
	}
	s = newReconnectServer(t, closeWith("remote error"), finished)
	if _, err := runSession(t, s); err != nil || s.count.Load() != 1 {
		t.Fatalf("remote error: err = %v, connections = %d, want it final", err, s.count.Load())
	}
}

// When the server cannot be reached again after the dial retries, the shell ends with an error.
func TestShellEndsWithAnErrorWhenItCannotReconnect(t *testing.T) {
	fastShellTimes(t)
	var s *reconnectServer
	s = newReconnectServer(t, func(conn *websocket.Conn) {
		s.Server.Listener.Close() // nothing accepts after this connection
		dropped(conn)
	})
	_, err := runSession(t, s)
	if err == nil || !strings.Contains(err.Error(), "could not connect") {
		t.Fatalf("err = %v", err)
	}
}

// An idle terminal keeps traffic flowing, like ssh's ServerAliveInterval.
func TestShellPingsTheServerWhileIdle(t *testing.T) {
	fastShellTimes(t)
	shellPingInterval = 30 * time.Millisecond
	var pings atomic.Int32
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		conn.SetPingHandler(func(string) error {
			pings.Add(1)
			return conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
		})
		go func() { // reading is what runs the ping handler
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		deadline := time.Now().Add(2 * time.Second)
		for pings.Load() < 3 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		finished(conn)
	})
	if _, err := runSession(t, s); err != nil {
		t.Fatal(err)
	}
	if pings.Load() < 3 {
		t.Fatalf("the server got %d pings, want at least 3", pings.Load())
	}
}

// What is typed while there is no connection is dropped, and typing works again after the reconnect.
func TestShellKeysReachTheNewConnectionAfterAReconnect(t *testing.T) {
	fastShellTimes(t)
	got := make(chan string, 1)
	s := newReconnectServer(t,
		func(conn *websocket.Conn) { time.Sleep(80 * time.Millisecond); dropped(conn) },
		func(conn *websocket.Conn) {
			_, data, err := conn.ReadMessage()
			if err == nil {
				got <- string(data)
			}
			finished(conn)
		},
	)
	stdin := withPipedStdin(t)
	go func() {
		for s.count.Load() < 2 {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		stdin.Write([]byte("x"))
	}()
	var err error
	captureStdout(t, func() { err = runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-got:
		if data != "x" {
			t.Fatalf("typed %q", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the key typed after the reconnect never arrived")
	}
}

// The reported bug (Windows): the remote program had switched mouse reporting on, the connection
// dropped inside an escape sequence, and the new shell got "69;12M35;71;12M..." typed into it.
// The terminal is reset before the new shell shows anything.
func TestShellResetsTheTerminalBeforeTheNewShell(t *testing.T) {
	fastShellTimes(t)
	s := newReconnectServer(t,
		func(conn *websocket.Conn) {
			conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[?25l\x1b[31mred \x1b[?10"))
			dropped(conn)
		},
		func(conn *websocket.Conn) {
			conn.WriteMessage(websocket.BinaryMessage, []byte("new shell$ "))
			finished(conn)
		},
	)
	out, err := runSession(t, s)
	if err != nil {
		t.Fatal(err)
	}
	reset := strings.Index(out, "\x18")
	fresh := strings.Index(out, "new shell$")
	if reset < 0 || fresh < 0 || reset > fresh {
		t.Fatalf("the terminal was not reset before the new shell: %q", out)
	}
	for _, want := range []string{"\x1b[?1000l", "\x1b[?1006l", "\x1b[?1049l", "\x1b[?25h", "\x1b[0m"} {
		if i := strings.Index(out, want); i < reset || i > fresh {
			t.Fatalf("%q is not between the drop and the new shell: %q", want, out)
		}
	}
}

// A shell that exits normally leaves the terminal as it found it: nothing is reset that was never set.
func TestShellExitLeavesAPlainTerminalAlone(t *testing.T) {
	fastShellTimes(t)
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		conn.WriteMessage(websocket.BinaryMessage, []byte("bye\r\n"))
		finished(conn)
	})
	out, _ := runSession(t, s)
	if strings.Contains(out, "\x1b[?") || strings.Contains(out, "\x18") {
		t.Fatalf("a plain session changed terminal modes: %q", out)
	}
}

// Like ssh: a proper end (the shell exited with `exit` or Ctrl+D, a normal close, a close without
// a status) is never dialed again, a connection lost to the network always is, once.
func TestShellReconnectsOnlyOnANetworkLoss(t *testing.T) {
	closeWith := func(code int) func(*websocket.Conn) {
		return func(conn *websocket.Conn) {
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""), time.Now().Add(time.Second))
		}
	}
	noStatus := func(conn *websocket.Conn) {
		conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
	}
	cases := map[string]struct {
		script    func(*websocket.Conn)
		reconnect bool
	}{
		"normal close (exit)":        {closeWith(websocket.CloseNormalClosure), false},
		"close without a status":     {noStatus, false},
		"policy violation":           {closeWith(websocket.ClosePolicyViolation), false},
		"cloud gave up (4000)":       {closeWith(4000), false},
		"no close frame (network)":   {dropped, true},
		"going away (restart)":       {closeWith(websocket.CloseGoingAway), true},
		"internal error (lost node)": {closeWith(websocket.CloseInternalServerErr), true},
		"service restart":            {closeWith(websocket.CloseServiceRestart), true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fastShellTimes(t)
			s := newReconnectServer(t, tc.script, finished)
			if _, err := runSession(t, s); err != nil {
				t.Fatal(err)
			}
			want := int32(1)
			if tc.reconnect {
				want = 2
			}
			if s.count.Load() != want {
				t.Fatalf("connected %d times, want %d", s.count.Load(), want)
			}
		})
	}
}

// Ctrl+C while the connection is down and the CLI is waiting to dial again gives up.
func TestShellCtrlCWhileReconnectingGivesUp(t *testing.T) {
	fastShellTimes(t)
	reconnectUnit = 5 * time.Second
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		dropped(conn)
	})
	stdin := withPipedStdin(t)
	go func() {
		time.Sleep(300 * time.Millisecond)
		stdin.Write([]byte{3})
	}()
	done := make(chan error, 1)
	go func() {
		captureStdout(t, func() { done <- runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) })
	}()
	select {
	case err := <-done:
		if err != nil || s.count.Load() != 1 {
			t.Fatalf("err = %v, connections = %d, want it to end after one", err, s.count.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl+C did not end the wait for the reconnect")
	}
}

// whatever close the servers on the way turn the shell's end into, typing exit ends the session
func TestShellEndsAfterExitWhateverTheCloseIs(t *testing.T) {
	closes := map[string]func(*websocket.Conn){
		"dropped": dropped,
		"lost 1011": func(c *websocket.Conn) {
			c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1011, "remote lost"), time.Now().Add(time.Second))
		},
		"going away": func(c *websocket.Conn) {
			c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1001, ""), time.Now().Add(time.Second))
		},
		"normal": finished,
	}
	for name, closeIt := range closes {
		for _, typed := range []string{"exit\r", "logout\r", "\x04", "  exit  \r", "ls\x7f\x7f\x7f\x7fexit\r"} {
			t.Run(name+"/"+strings.TrimSpace(typed), func(t *testing.T) {
				fastShellTimes(t)
				s := newReconnectServer(t, func(conn *websocket.Conn) {
					conn.ReadMessage() // the typed exit
					closeIt(conn)
				}, func(conn *websocket.Conn) { finished(conn) })
				stdin := withPipedStdin(t)
				done := make(chan error, 1)
				var out string
				go func() {
					out = captureStdout(t, func() { done <- runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) })
				}()
				time.Sleep(50 * time.Millisecond)
				stdin.Write([]byte(typed))
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("the session did not end")
				}
				if s.count.Load() != 1 || strings.Contains(out, "Reconnecting") {
					t.Fatalf("connected %d times, out = %q: exit must end the session", s.count.Load(), out)
				}
			})
		}
	}
}

// a lost connection after something other than exit is still dialled again
func TestShellReconnectsWhenOtherInputPrecedesTheDrop(t *testing.T) {
	fastShellTimes(t)
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		conn.ReadMessage()
		dropped(conn)
	}, finished)
	stdin := withPipedStdin(t)
	done := make(chan error, 1)
	go func() { captureStdout(t, func() { done <- runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) }) }()
	time.Sleep(50 * time.Millisecond)
	stdin.Write([]byte("exits\rexit 1 \rls\r"))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end")
	}
	if s.count.Load() != 2 {
		t.Fatalf("connected %d times, want 2", s.count.Load())
	}
}

func TestShellExitIsForgottenAfterTheWindow(t *testing.T) {
	k := &shellKeys{}
	k.mu.Lock()
	k.track('x')
	for _, b := range []byte("\x7fexit\r") {
		k.track(b)
	}
	k.mu.Unlock()
	if !k.typedExit() {
		t.Fatal("exit was not noticed")
	}
	old := shellExitWindow
	shellExitWindow = time.Millisecond
	t.Cleanup(func() { shellExitWindow = old })
	time.Sleep(5 * time.Millisecond)
	if k.typedExit() {
		t.Fatal("an old exit must not end a later lost connection")
	}
}

func TestShellClosesOnTwoQuickCtrlCs(t *testing.T) {
	fastShellTimes(t)
	got := make(chan []byte, 1)
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		var typed []byte
		for {
			_, d, err := conn.ReadMessage()
			if err != nil {
				got <- typed
				return
			}
			typed = append(typed, d...)
		}
	}, finished)
	stdin := withPipedStdin(t)
	done := make(chan error, 1)
	go func() { captureStdout(t, func() { done <- runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) }) }()
	time.Sleep(50 * time.Millisecond)
	stdin.Write([]byte{3})
	select {
	case <-done:
		t.Fatal("one Ctrl+C must go to the remote shell, not close it")
	case <-time.After(150 * time.Millisecond):
	}
	stdin.Write([]byte{3})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("two quick Ctrl+C must close the session")
	}
	if s.count.Load() != 1 {
		t.Fatalf("connected %d times, want 1 (no reconnect after the user left)", s.count.Load())
	}
	if typed := <-got; string(typed) != "\x03" {
		t.Fatalf("the shell got %q, want only the first Ctrl+C", typed)
	}
}

func TestShellCtrlCsFarApartDoNotClose(t *testing.T) {
	fastShellTimes(t)
	old := shellCtrlCWindow
	shellCtrlCWindow = 30 * time.Millisecond
	t.Cleanup(func() { shellCtrlCWindow = old })
	s := newReconnectServer(t, func(conn *websocket.Conn) {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	stdin := withPipedStdin(t)
	done := make(chan error, 1)
	go func() { captureStdout(t, func() { done <- runShellSession(s.url(), "tok", newShellKeys(os.Stdin)) }) }()
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 3; i++ {
		stdin.Write([]byte{3})
		time.Sleep(120 * time.Millisecond)
	}
	select {
	case <-done:
		t.Fatal("slow Ctrl+C presses must not close the shell")
	default:
	}
	stdin.Write([]byte{3, 3})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("two quick presses must close it")
	}
}
