package tuiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsEcho upgrades and echoes binary frames; text frames are reported on resizes.
func wsEcho(t *testing.T, resizes chan<- controlMsg) *httptest.Server {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.TextMessage {
				var m controlMsg
				if json.Unmarshal(data, &m) == nil && resizes != nil {
					select {
					case resizes <- m:
					default:
					}
				}
				continue
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func TestPumpEchoesInput(t *testing.T) {
	srv := wsEcho(t, nil)
	ws := dialWS(t, srv)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- Pump(ctx, ws, inR, outW, nil) }()

	if _, err := inW.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("hello"))
	if _, err := io.ReadFull(outR, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("echo = %q", buf)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Pump returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Pump did not return after cancel")
	}
}

func TestPumpSendsResize(t *testing.T) {
	resizes := make(chan controlMsg, 1)
	srv := wsEcho(t, resizes)
	ws := dialWS(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inR, _ := io.Pipe()
	_, outW := io.Pipe()
	resizeCh := make(chan Winsize, 1)

	done := make(chan error, 1)
	go func() { done <- Pump(ctx, ws, inR, outW, resizeCh) }()

	resizeCh <- Winsize{Cols: 120, Rows: 40}

	select {
	case m := <-resizes:
		if m.Type != "resize" || m.Cols != 120 || m.Rows != 40 {
			t.Fatalf("resize = %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no resize frame received")
	}
	cancel()
	<-done
}

func TestPumpSurfacesCloseCode(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "session revoked"),
			time.Now().Add(time.Second),
		)
		_ = ws.Close()
	}))
	defer srv.Close()
	ws := dialWS(t, srv)

	inR, _ := io.Pipe()
	_, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := Pump(ctx, ws, inR, outW, nil)
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 4001 {
		t.Fatalf("Pump = %v, want CloseError 4001", err)
	}
}

func TestDetachReader(t *testing.T) {
	r := NewDetachReader(strings.NewReader("ab" + string(rune(DetachByte)) + "c"))
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "abc" {
		t.Fatalf("read = %q, want abc", got)
	}
	select {
	case <-r.Done():
	default:
		t.Fatal("Done not closed after detach byte")
	}
}
