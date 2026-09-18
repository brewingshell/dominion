package tuiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// DetachByte is Ctrl-], the key that leaves an attached shell without killing
// the tmux session.
const DetachByte = 0x1d

// ErrDetach reports that the user pressed the detach key.
var ErrDetach = errors.New("detached")

// Winsize is a terminal size in character cells.
type Winsize struct {
	Cols uint16
	Rows uint16
}

// controlMsg is the JSON text frame the server understands (see
// internal/ptybridge). Only "resize" is defined.
type controlMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// CloseError carries a WebSocket close code so callers can distinguish a
// revoked token (4001) from a server shutdown (1001).
type CloseError struct {
	Code int
	Text string
}

func (e *CloseError) Error() string {
	if e.Text != "" {
		return e.Text
	}
	return "websocket closed"
}

// Pump bridges the local terminal streams and an attached WebSocket until
// either side stops, ctx is cancelled, or input ends. Terminal output arrives
// as binary frames and is written to out; input read from in is sent as binary
// frames; resizes are sent as JSON text frames.
func Pump(ctx context.Context, ws *websocket.Conn, in io.Reader, out io.Writer, resizes <-chan Winsize) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errc := make(chan error, 3)

	// WebSocket -> terminal.
	go func() {
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				errc <- asCloseError(err)
				return
			}
			if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
				if _, err := out.Write(data); err != nil {
					errc <- err
					return
				}
			}
		}
	}()

	// Terminal -> WebSocket.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	// Resizes -> WebSocket.
	if resizes != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case sz, ok := <-resizes:
					if !ok {
						return
					}
					msg, _ := json.Marshal(controlMsg{Type: "resize", Cols: sz.Cols, Rows: sz.Rows})
					if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
						errc <- err
						return
					}
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func asCloseError(err error) error {
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return &CloseError{Code: ce.Code, Text: ce.Text}
	}
	return err
}

// DetachReader removes the first DetachByte from the stream and closes Done.
// It lets an attached shell return to the TUI without forwarding Ctrl-].
type DetachReader struct {
	r    io.Reader
	once sync.Once
	done chan struct{}
}

// NewDetachReader wraps r.
func NewDetachReader(r io.Reader) *DetachReader {
	return &DetachReader{r: r, done: make(chan struct{})}
}

// Done is closed once the detach byte has been seen.
func (d *DetachReader) Done() <-chan struct{} { return d.done }

func (d *DetachReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if n > 0 {
		if i := bytes.IndexByte(p[:n], DetachByte); i >= 0 {
			copy(p[i:], p[i+1:n])
			n--
			d.once.Do(func() { close(d.done) })
		}
	}
	return n, err
}
