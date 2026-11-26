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
