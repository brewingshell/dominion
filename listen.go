package main

import (
	"crypto/tls"
	"io"
	"net"
	"time"
)

// peekTimeout bounds how long we wait for the first byte that tells us whether
// a connection is TLS or plain HTTP.
const peekTimeout = 10 * time.Second

// peekConn is a connection whose first bytes were already read during
// classification. It replays them before reading from the underlying conn, so
// nothing is lost when the connection is handed to net/http or crypto/tls.
type peekConn struct {
	net.Conn
	prefix []byte
}

func (c *peekConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

// peekListener accepts connections on a single port and routes each one to TLS
// or plain HTTP based on its first byte: a TLS record starts with 0x16 (the
// handshake content type). This lets the same port answer both https:// and
// http:// without a separate redirect listener.
type peekListener struct {
	net.Listener
	tlsConfig *tls.Config
}

func (l *peekListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		conn, err := l.classify(c)
		if err != nil {
			_ = c.Close()
			continue
		}
		return conn, nil
	}
}

func (l *peekListener) classify(c net.Conn) (net.Conn, error) {
	var first [1]byte
	_ = c.SetReadDeadline(time.Now().Add(peekTimeout))
	if _, err := io.ReadFull(c, first[:]); err != nil {
		return nil, err
	}
	_ = c.SetReadDeadline(time.Time{})

	pc := &peekConn{Conn: c, prefix: first[:]}
	if first[0] == 0x16 {
		return tls.Server(pc, l.tlsConfig), nil
	}
	return pc, nil
}
