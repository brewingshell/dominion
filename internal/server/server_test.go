package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/brewingshell/dominion/internal/ptybridge"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := New(Config{
		PIN:      "3232",
		TmuxBin:  "tmux",
		TokenTTL: time.Hour,
		WebFS:    os.DirFS("../.."),
		Logger:   log.New(io.Discard, "", 0),
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestSessionsRequiresAuth(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestLoginFlow(t *testing.T) {
	ts := newTestServer(t)

	// Wrong PIN.
	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"0000"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong pin: want 401, got %d", res.StatusCode)
	}

	// Correct PIN.
	res, err = http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("correct pin: want 200, got %d", res.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("no auth cookie returned")
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authed sessions: want 200, got %d", res.StatusCode)
	}
	var body struct {
		Sessions []struct {
			Name string `json:"name"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestLockRevokesSession(t *testing.T) {
	ts := newTestServer(t)

	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("no auth cookie returned")
	}

	lockReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/lock", nil)
	lockReq.AddCookie(cookie)
	res, err = http.DefaultClient.Do(lockReq)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lock: want 200, got %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after lock: want 401, got %d", res.StatusCode)
	}
}

// TestLockKeepsTokenForSockets verifies that locking refuses HTTP but keeps the
// token valid for already-open sockets, while logout removes it entirely.
func TestLockKeepsTokenForSockets(t *testing.T) {
	srv := New(Config{
		PIN:      "3232",
		TmuxBin:  "tmux",
		TokenTTL: time.Hour,
		WebFS:    os.DirFS("../.."),
		Logger:   log.New(io.Discard, "", 0),
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cookie := loginCookie(t, ts)

	do := func(path string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+path, nil)
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}

	if code := do("/api/lock"); code != http.StatusOK {
		t.Fatalf("lock: want 200, got %d", code)
	}
	if srv.auth.Valid(cookie.Value) {
		t.Error("locked token must be rejected for HTTP")
	}
	if !srv.auth.SocketValid(cookie.Value) {
		t.Error("locked token must still validate sockets")
	}

	if code := do("/api/logout"); code != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", code)
	}
	if srv.auth.SocketValid(cookie.Value) {
		t.Error("logged-out token must invalidate sockets")
	}
}

func loginCookie(t *testing.T, ts *httptest.Server) *http.Cookie {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			return c
		}
	}
	t.Fatal("no auth cookie returned")
	return nil
}

func postJSON(t *testing.T, ts *httptest.Server, path string, cookie *http.Cookie, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionMutationRequiresAuth(t *testing.T) {
	ts := newTestServer(t)
	for _, path := range []string{"/api/sessions/create", "/api/sessions/kill"} {
		res := postJSON(t, ts, path, nil, `{"name":"x"}`)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d", path, res.StatusCode)
		}
	}
}

func TestCreateAndKillSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_srv_%d", os.Getpid())
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })

	ts := newTestServer(t)
	cookie := loginCookie(t, ts)

	res := postJSON(t, ts, "/api/sessions/create", cookie, fmt.Sprintf(`{"name":%q}`, name))
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create: want 200, got %d", res.StatusCode)
	}

	// Duplicate create is a conflict.
	res = postJSON(t, ts, "/api/sessions/create", cookie, fmt.Sprintf(`{"name":%q}`, name))
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate create: want 409, got %d", res.StatusCode)
	}

	// Invalid name is rejected before reaching tmux.
	res = postJSON(t, ts, "/api/sessions/create", cookie, `{"name":"bad:name"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid create: want 400, got %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Sessions []struct {
			Name string `json:"name"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	res.Body.Close()
	found := false
	for _, s := range list.Sessions {
		if s.Name == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("created session %q not listed", name)
	}

	res = postJSON(t, ts, "/api/sessions/kill", cookie, fmt.Sprintf(`{"name":%q}`, name))
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("kill: want 200, got %d", res.StatusCode)
	}

	res = postJSON(t, ts, "/api/sessions/kill", cookie, fmt.Sprintf(`{"name":%q}`, name))
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("kill missing: want 404, got %d", res.StatusCode)
	}
}

func TestSecureCookieWhenTLS(t *testing.T) {
	srv := New(Config{
		PIN:           "3232",
		TmuxBin:       "tmux",
		TokenTTL:      time.Hour,
		WebFS:         os.DirFS("../.."),
		Logger:        log.New(io.Discard, "", 0),
		SecureCookies: true,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			if !c.Secure {
				t.Error("auth cookie should be Secure when TLS is enabled")
			}
			return
		}
	}
	t.Fatal("no auth cookie returned")
}

// TestPlainHTTPCookieNotSecure verifies that over plain HTTP the cookie is not
// marked Secure, so a login over http:// still works when HTTP is accepted.
func TestPlainHTTPCookieNotSecure(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			if c.Secure {
				t.Error("cookie must not be Secure on a plain HTTP request")
			}
			return
		}
	}
	t.Fatal("no auth cookie returned")
}

func TestBrandingOverrideWins(t *testing.T) {
	dir := t.TempDir()
	png := []byte("\x89PNG\r\n\x1a\noverride")
	if err := os.WriteFile(filepath.Join(dir, "override_logo.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{
		PIN: "3232", TmuxBin: "tmux", TokenTTL: time.Hour,
		WebFS: os.DirFS("../.."), Logger: log.New(io.Discard, "", 0),
		BrandingDir: dir,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{"/dom-logo.png", "/favicon.ico", "/apple-touch-icon.png"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(body) != string(png) {
			t.Errorf("%s: override not served (%d bytes)", path, len(body))
		}
		if got := res.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q", path, got)
		}
	}
}

func TestBrandingFallsBackToEmbedded(t *testing.T) {
	srv := New(Config{
		PIN: "3232", TmuxBin: "tmux", TokenTTL: time.Hour,
		WebFS: os.DirFS("../.."), Logger: log.New(io.Discard, "", 0),
		BrandingDir: t.TempDir(),
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/dom-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("embedded logo: want 200, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if len(body) < 100 {
		t.Fatalf("embedded logo looks empty: %d bytes", len(body))
	}
}

func TestHealthzIsUnauthenticated(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
}

func TestKillPinnedSessionRefused(t *testing.T) {
	ts := newTestServer(t)
	cookie := loginCookie(t, ts)
	res := postJSON(t, ts, "/api/sessions/kill", cookie, `{"name":"dominion"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("killing the pinned session: want 403, got %d", res.StatusCode)
	}
}

func TestAttachBridgesPTY(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_test_%d", os.Getpid())
	if err := exec.Command("tmux", "new-session", "-d", "-s", name).Run(); err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })
	// Give the session a moment to settle.
	time.Sleep(150 * time.Millisecond)

	ts := newTestServer(t)

	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no auth cookie")
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") +
		"/api/attach?session=" + name + "&cols=80&rows=24"
	header := http.Header{}
	header.Set("Cookie", cookie.Name+"="+cookie.Value)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// A freshly attached session must emit terminal output.
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected terminal output, got empty frame")
	}

	// Send a command and expect the echoed text back.
	_ = ws.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("echo dominion-ok\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, data, err = ws.ReadMessage()
		if err != nil {
			t.Fatalf("read after input: %v", err)
		}
		if strings.Contains(string(data), "dominion-ok") {
			return
		}
	}
	t.Fatal("did not observe command output")
}

// TestLogoutClosesSocket verifies that a revoked token causes the bridge to
// close an already-attached socket with the revocation close code.
func TestLogoutClosesSocket(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_rev_%d", os.Getpid())
	if err := exec.Command("tmux", "new-session", "-d", "-s", name).Run(); err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })
	time.Sleep(150 * time.Millisecond)

	old := ptybridge.RevalidateInterval
	ptybridge.RevalidateInterval = 25 * time.Millisecond
	t.Cleanup(func() { ptybridge.RevalidateInterval = old })

	ts := newTestServer(t)
	cookie := loginCookie(t, ts)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") +
		"/api/attach?session=" + name + "&cols=80&rows=24"
	header := http.Header{}
	header.Set("Cookie", cookie.Name+"="+cookie.Value)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Revoke the token via logout.
	postJSON(t, ts, "/api/logout", cookie, "").Body.Close()

	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			if ce, ok := err.(*websocket.CloseError); ok {
				if ce.Code != ptybridge.CloseRevoked {
					t.Fatalf("close code = %d, want %d", ce.Code, ptybridge.CloseRevoked)
				}
				return
			}
			t.Fatalf("expected close error, got %v", err)
		}
	}
}

func TestCloseSocketsOnShutdown(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_close_%d", os.Getpid())
	if err := exec.Command("tmux", "new-session", "-d", "-s", name).Run(); err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })
	time.Sleep(150 * time.Millisecond)

	srv := New(Config{PIN: "3232", TmuxBin: "tmux", TokenTTL: time.Hour, WebFS: os.DirFS("../.."), Logger: log.New(io.Discard, "", 0)})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	cookie := loginCookie(t, ts)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") +
		"/api/attach?session=" + name + "&cols=80&rows=24"
	header := http.Header{}
	header.Set("Cookie", cookie.Name+"="+cookie.Value)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	time.Sleep(100 * time.Millisecond)

	srv.CloseSockets()

	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			ce, ok := err.(*websocket.CloseError)
			if !ok {
				t.Fatalf("expected close error, got %v", err)
			}
			if ce.Code != websocket.CloseGoingAway {
				t.Fatalf("close code = %d, want %d", ce.Code, websocket.CloseGoingAway)
			}
			return
		}
	}
}

func TestAttachRejectsBadSession(t *testing.T) {
	ts := newTestServer(t)
	// Unauthenticated bad session should still be 401, not upgraded.
	res, err := http.Get(ts.URL + "/api/attach?session=x")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestAttachMissingSessionIs404(t *testing.T) {
	ts := newTestServer(t)
	cookie := loginCookie(t, ts)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/attach?session=definitely_missing_xyz", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", res.StatusCode)
	}
}

func TestStaticCacheHeaders(t *testing.T) {
	ts := newTestServer(t)
	cases := []struct {
		path string
		want string
	}{
		{"/", "no-cache"},
		{"/app.js", "no-cache"},
		{"/style.css", "no-cache"},
		{"/vendor/xterm.js", "public, max-age=86400"},
	}
	for _, tc := range cases {
		res, err := http.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("missing/weak CSP: %q", csp)
	}
	if pp := res.Header.Get("Permissions-Policy"); pp == "" {
		t.Error("missing Permissions-Policy")
	}
}

// TestPortalAttachNotCountedAsExternal verifies the "attached" flag ignores the
// portal's own bridge, so the kill warning only fires for other clients.
func TestPortalAttachNotCountedAsExternal(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_ext_%d", os.Getpid())
	if err := exec.Command("tmux", "new-session", "-d", "-s", name).Run(); err != nil {
		t.Skipf("cannot create tmux session: %v", err)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })
	time.Sleep(150 * time.Millisecond)

	ts := newTestServer(t)
	cookie := loginCookie(t, ts)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") +
		"/api/attach?session=" + name + "&cols=80&rows=24"
	header := http.Header{}
	header.Set("Cookie", cookie.Name+"="+cookie.Value)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	time.Sleep(200 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list struct {
		Sessions []struct {
			Name     string `json:"name"`
			Attached bool   `json:"attached"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range list.Sessions {
		if s.Name == name && s.Attached {
			t.Fatal("portal attachment should not count as an external client")
		}
	}
}

// newTestServerWithPINFile builds a test server that can persist a PIN change
// to path.
func newTestServerWithPINFile(t *testing.T, path string) *httptest.Server {
	t.Helper()
	srv := New(Config{
		PIN:      "3232",
		PINFile:  path,
		TmuxBin:  "tmux",
		TokenTTL: time.Hour,
		WebFS:    os.DirFS("../.."),
		Logger:   log.New(io.Discard, "", 0),
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// loginWithPIN logs in and returns the auth cookie, failing on a non-200.
func loginWithPIN(t *testing.T, ts *httptest.Server, pin string) *http.Cookie {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/login", "application/json",
		strings.NewReader(`{"pin":"`+pin+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login with %q: want 200, got %d", pin, res.StatusCode)
	}
	for _, c := range res.Cookies() {
		if c.Name == "dominion_auth" {
			return c
		}
	}
	t.Fatal("no auth cookie returned")
	return nil
}

func authedStatus(t *testing.T, ts *httptest.Server, cookie *http.Cookie) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestChangePINRequiresAuth(t *testing.T) {
	ts := newTestServerWithPINFile(t, filepath.Join(t.TempDir(), "pin"))
	res := postJSON(t, ts, "/api/pin", nil, `{"current":"3232","new":"5683"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestChangePINDisabledWithoutFile(t *testing.T) {
	ts := newTestServer(t) // no PINFile
	cookie := loginCookie(t, ts)
	res := postJSON(t, ts, "/api/pin", cookie, `{"current":"3232","new":"5683"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %d", res.StatusCode)
	}
}

func TestChangePINRequiresCurrent(t *testing.T) {
	pinPath := filepath.Join(t.TempDir(), "pin")
	ts := newTestServerWithPINFile(t, pinPath)
	cookie := loginCookie(t, ts)

	res := postJSON(t, ts, "/api/pin", cookie, `{"current":"0000","new":"5683"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current PIN: want 401, got %d", res.StatusCode)
	}
	if _, err := os.Stat(pinPath); !os.IsNotExist(err) {
		t.Fatal("a failed change must not write the pin file")
	}
	// The original PIN still works.
	if c := loginWithPIN(t, ts, "3232"); c == nil {
		t.Fatal("original PIN should still log in")
	}
}

func TestChangePINValidation(t *testing.T) {
	ts := newTestServerWithPINFile(t, filepath.Join(t.TempDir(), "pin"))
	cookie := loginCookie(t, ts)
	for _, body := range []string{
		`{"current":"3232","new":"123"}`,
		`{"current":"3232","new":"1234567890123"}`,
	} {
		res := postJSON(t, ts, "/api/pin", cookie, body)
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", body, res.StatusCode)
		}
	}
}

func TestChangePINSuccess(t *testing.T) {
	pinPath := filepath.Join(t.TempDir(), "pin")
	ts := newTestServerWithPINFile(t, pinPath)

	caller := loginCookie(t, ts)         // the client making the change
	other := loginWithPIN(t, ts, "3232") // a second client, same PIN

	res := postJSON(t, ts, "/api/pin", caller, `{"current":"3232","new":"5683"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("change: want 200, got %d", res.StatusCode)
	}

	// The caller stays logged in; the other session is revoked.
	if got := authedStatus(t, ts, caller); got != http.StatusOK {
		t.Fatalf("caller should stay authenticated, got %d", got)
	}
	if got := authedStatus(t, ts, other); got != http.StatusUnauthorized {
		t.Fatalf("other session should be revoked, got %d", got)
	}

	// Old PIN rejected, new PIN accepted.
	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"pin":"3232"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old PIN: want 401, got %d", res.StatusCode)
	}
	loginWithPIN(t, ts, "5683")

	// The new PIN is on disk with owner-only permissions.
	data, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatalf("read pin file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "5683" {
		t.Fatalf("pin file = %q, want 5683", strings.TrimSpace(string(data)))
	}
	info, err := os.Stat(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pin file mode = %o, want 600", perm)
	}
}

func TestChangePINWriteFailureLeavesPIN(t *testing.T) {
	// Point PINFile inside a regular file so MkdirAll fails.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithPINFile(t, filepath.Join(blocker, "pin"))
	cookie := loginCookie(t, ts)

	res := postJSON(t, ts, "/api/pin", cookie, `{"current":"3232","new":"5683"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("write failure: want 500, got %d", res.StatusCode)
	}
	// The in-memory PIN is unchanged.
	if c := loginWithPIN(t, ts, "3232"); c == nil {
		t.Fatal("PIN should be unchanged after a write failure")
	}
}

func TestWritePINFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "pin")
	if err := writePINFile(path, "2468"); err != nil {
		t.Fatalf("writePINFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "2468" {
		t.Fatalf("content = %q, want 2468", strings.TrimSpace(string(got)))
	}
}
