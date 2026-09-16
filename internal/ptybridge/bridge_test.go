package ptybridge

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
)

// fakeTmux writes a shell script that ignores its arguments and echoes stdin,
// standing in for `tmux attach-session`.
func fakeTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-tmux")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec cat\n"), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	return path
}

// dialBridge starts a server that bridges to a fake tmux and returns a client
// connection plus the server, so tests can exercise the bridge end to end.
func dialBridge(t *testing.T, validate func() bool) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	bin := fakeTmux(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		_ = Bridge(ws, bin, "fake", 80, 24, validate)
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		ws.Close()
		srv.Close()
	})
	return ws, srv
}

func TestBridgeRelaysInputToOutput(t *testing.T) {
	ws, _ := dialBridge(t, nil)

	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("hello-bridge\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(data), "hello-bridge") {
			return
		}
	}
}

func TestBridgeClosesOnRevoke(t *testing.T) {
	old := RevalidateInterval
	RevalidateInterval = 20 * time.Millisecond
	t.Cleanup(func() { RevalidateInterval = old })

	var valid atomic.Bool
	valid.Store(true)
	ws, _ := dialBridge(t, func() bool { return valid.Load() })

	// Revoke; the bridge should close with CloseRevoked on the next tick.
	valid.Store(false)
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			ce, ok := err.(*websocket.CloseError)
			if !ok {
				t.Fatalf("expected close error, got %v", err)
			}
			if ce.Code != CloseRevoked {
				t.Fatalf("close code = %d, want %d", ce.Code, CloseRevoked)
			}
			return
		}
	}
}

func TestBridgeAcceptsResizeControl(t *testing.T) {
	ws, _ := dialBridge(t, nil)
	// A text frame is a control message and must not be forwarded to the PTY
	// as input; the connection should stay healthy.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":100,"rows":40}`)); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("still-alive\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(data), "still-alive") {
			return
		}
	}
}

func TestClamp(t *testing.T) {
	cases := []struct {
		v, def, want uint16
	}{
		{0, 80, 80},
		{120, 80, 120},
		{24, 24, 24},
	}
	for _, c := range cases {
		if got := clamp(c.v, c.def); got != c.want {
			t.Errorf("clamp(%d, %d) = %d, want %d", c.v, c.def, got, c.want)
		}
	}
}
