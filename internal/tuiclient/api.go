// Package tuiclient is the terminal client for a dominion portal. It is a plain
// API client: it speaks the same HTTP + WebSocket routes the browser UI uses and
// carries the auth cookie in memory. No server code runs here.
package tuiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultPort is appended when the entered host has no port.
const DefaultPort = "5550"

var (
	// ErrUnauthorized means the session token is missing, locked, or expired.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrBadPIN means the server rejected the PIN.
	ErrBadPIN = errors.New("incorrect pin")
	// ErrTooMany means the login rate limiter rejected the attempt.
	ErrTooMany = errors.New("too many attempts")
)

// Session is a tmux session as reported by GET /api/sessions.
type Session struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
}

// Client talks to one dominion server.
type Client struct {
	baseURL string
	http    *http.Client
	tlsConf *tls.Config
}

// New builds a client for base ("http://host:port" or "https://..."). insecure
// skips certificate verification, matching the desktop client's plain-HTTP
// default while still allowing a self-signed server.
func New(base string, insecure bool) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid server %q: %w", base, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid server %q", base)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	tlsConf := &tls.Config{MinVersion: tls.VersionTLS12}
	if insecure {
		tlsConf.InsecureSkipVerify = true //nolint:gosec // explicit user opt-in
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   8 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: tlsConf,
	}
	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		http:    &http.Client{Jar: jar, Transport: tr, Timeout: 15 * time.Second},
		tlsConf: tlsConf,
	}, nil
}

// NormalizeBase turns a scheme and a host (with or without port) into a base
// URL, applying DefaultPort when none is given.
func NormalizeBase(scheme, host string) (string, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", scheme)
	}
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimRight(host, "/")
	if host == "" {
		return "", errors.New("enter a server address")
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		// Bracketed IPv6 without a port keeps its brackets for JoinHostPort.
		if !strings.HasPrefix(host, "[") {
			host = net.JoinHostPort(host, DefaultPort)
		} else {
			host = host + ":" + DefaultPort
		}
	}
	return scheme + "://" + host, nil
}

// BaseURL is the server this client talks to.
func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return apiError(res)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func apiError(res *http.Response) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&e)
	switch res.StatusCode {
	case http.StatusUnauthorized:
		if e.Error != "" {
			return fmt.Errorf("%w: %s", ErrUnauthorized, e.Error)
		}
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		if e.Error != "" {
			return fmt.Errorf("%w: %s", ErrTooMany, e.Error)
		}
		return ErrTooMany
	}
	if e.Error != "" {
		return errors.New(e.Error)
	}
	return fmt.Errorf("server returned %s", res.Status)
}

// Healthz probes the unauthenticated liveness endpoint. It is used to validate
// an address before logging in.
func (c *Client) Healthz(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/healthz", nil, nil)
}

// Login exchanges the PIN for a session cookie stored in the client jar.
func (c *Client) Login(ctx context.Context, pin string) error {
	err := c.do(ctx, http.MethodPost, "/api/login", map[string]string{"pin": pin}, nil)
	if errors.Is(err, ErrUnauthorized) {
		return ErrBadPIN
	}
	return err
}

// Lock requires the PIN again while keeping attached terminals alive.
func (c *Client) Lock(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/lock", nil, nil)
}

// Logout revokes the session token; attached terminals close.
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/logout", nil, nil)
}

// Sessions lists tmux sessions.
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var out struct {
		Sessions []Session `json:"sessions"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/sessions", nil, &out); err != nil {
		return nil, err
	}
	if out.Sessions == nil {
		out.Sessions = []Session{}
	}
	return out.Sessions, nil
}

// Create creates a tmux session.
func (c *Client) Create(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/api/sessions/create", map[string]string{"name": name}, nil)
}

// Kill terminates a tmux session.
func (c *Client) Kill(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/api/sessions/kill", map[string]string{"name": name}, nil)
}

// DialAttach opens the WebSocket PTY bridge for a session. The caller owns the
// returned connection.
func (c *Client) DialAttach(ctx context.Context, name string, cols, rows uint16) (*websocket.Conn, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/attach"
	q := url.Values{}
	q.Set("session", name)
	q.Set("cols", strconv.Itoa(int(cols)))
	q.Set("rows", strconv.Itoa(int(rows)))
	u.RawQuery = q.Encode()

	d := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  c.tlsConf,
		Jar:              c.http.Jar,
	}
	conn, res, err := d.DialContext(ctx, u.String(), nil)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusUnauthorized {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	return conn, nil
}
