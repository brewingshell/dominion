package ptybridge

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	// pongWait is how long we wait for any message (including a pong) before
	// considering the client gone.
	pongWait = 60 * time.Second
	// pingPeriod must be less than pongWait so a ping is sent before timeout.
	pingPeriod = 25 * time.Second
	// writeWait bounds a single control write.
	writeWait = 10 * time.Second
	// maxMessageSize bounds a single client frame.
	maxMessageSize = 1 << 20

	// CloseRevoked is the WebSocket close code sent when the session token is
	// no longer valid (logged out or expired). The client stops reconnecting.
	CloseRevoked = 4001
)

// RevalidateInterval is how often an attached socket re-checks its session
// token. It is a variable so tests can shorten it.
var RevalidateInterval = 30 * time.Second

// ErrRevoked is reported when the session token is no longer valid.
var ErrRevoked = errors.New("session revoked")

type controlMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Bridge attaches a tmux session to the given WebSocket. Terminal output is
// sent as binary frames; client input is expected as binary frames. Text
// frames are reserved for JSON control messages (currently only "resize").
//
// cols/rows are the client's initial terminal size. validate, if non-nil, is
// polled periodically; when it returns false the socket is closed with code
// CloseRevoked. The function blocks until the WebSocket or the PTY closes,
// then tears both down.
func Bridge(ws *websocket.Conn, tmuxBin, name string, cols, rows uint16, validate func() bool) error {
	if tmuxBin == "" {
		tmuxBin = "tmux"
	}
	cmd := exec.Command(tmuxBin, "attach-session", "-t", "="+name)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: clamp(cols, 80), Rows: clamp(rows, 24)})
	if err != nil {
		return err
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = f.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = ws.Close()
			_ = cmd.Wait()
		})
	}
	defer cleanup()

	errc := make(chan error, 3)

	// PTY -> WebSocket.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if rerr != nil {
				errc <- rerr
				return
			}
		}
	}()

	// Keepalive: ping periodically so a silently dropped link is detected.
	// WriteControl is safe to call concurrently with the other writers.
	stopPing := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			}
		}
	}()
	defer close(stopPing)

	// Revalidate the session token periodically. Locking leaves the token
	// valid for sockets, so this only fires after logout or expiry.
	if validate != nil {
		go func() {
			ticker := time.NewTicker(RevalidateInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stopPing:
					return
				case <-ticker.C:
					if !validate() {
						_ = ws.WriteControl(websocket.CloseMessage,
							websocket.FormatCloseMessage(CloseRevoked, "session revoked"),
							time.Now().Add(writeWait))
						errc <- ErrRevoked
						return
					}
				}
			}
		}()
	}

	// WebSocket -> PTY.
	go func() {
		ws.SetReadLimit(maxMessageSize)
		_ = ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error {
			return ws.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			typ, data, rerr := ws.ReadMessage()
			if rerr != nil {
				errc <- rerr
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(pongWait))
			if typ == websocket.TextMessage {
				var m controlMsg
				if json.Unmarshal(data, &m) == nil && m.Type == "resize" {
					_ = pty.Setsize(f, &pty.Winsize{
						Cols: clamp(m.Cols, 80),
						Rows: clamp(m.Rows, 24),
					})
				}
				continue
			}
			if _, werr := f.Write(data); werr != nil {
				errc <- werr
				return
			}
		}
	}()

	err = <-errc
	cleanup()
	return err
}

func clamp(v, def uint16) uint16 {
	if v == 0 {
		return def
	}
	return v
}
