package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/brewingshell/dominion/internal/auth"
	"github.com/brewingshell/dominion/internal/ptybridge"
	"github.com/brewingshell/dominion/internal/tmux"
)

// Config configures the portal server.
type Config struct {
	PIN      string
	TmuxBin  string
	TokenTTL time.Duration
	WebFS    fs.FS
	Logger   *log.Logger
	// SecureCookies marks the auth cookie Secure; set when serving over TLS.
	SecureCookies bool
	// BrandingDir is an optional directory of image overrides. The embedded
	// assets under web/ are the defaults; see brandSlots.
	BrandingDir string
}

// brandSlot is a servable brand image with the override file names it accepts.
type brandSlot struct {
	path string // request path
	name string // per-slot override prefix, override_<name>.<ext>
	def  string // default file base name in the branding dir, <def>.<ext>
	// rasterOnly skips SVG, which is fine for the login mark but not valid for
	// a favicon (.ico) or an iOS home-screen icon.
	rasterOnly bool
}

// brandSlots are the images a deployment may override. Each falls back to the
// embedded file under web/ when no override is found on disk.
var brandSlots = []brandSlot{
	{path: "/dom-logo.png", name: "logo", def: "logo"},
	{path: "/apple-touch-icon.png", name: "apple_touch", def: "apple-touch-icon", rasterOnly: true},
	{path: "/favicon.ico", name: "favicon", def: "favicon", rasterOnly: true},
}

// brandExts is the search order for override files.
var brandExts = []string{"svg", "png", "webp", "jpg", "jpeg", "ico"}

// rasterExts is brandExts without SVG.
var rasterExts = []string{"png", "webp", "jpg", "jpeg", "ico"}

// Server serves the portal HTTP API and static frontend.
type Server struct {
	cfg     Config
	auth    *auth.Store
	limiter *auth.Limiter
	up      websocket.Upgrader

	// bridges counts live portal attachments per session, so that the
	// "attached" flag can report real external clients only.
	mu      sync.Mutex
	bridges map[string]int
	// socks tracks live WebSocket connections so they can be closed on
	// shutdown (http.Server.Shutdown does not close hijacked connections).
	socks map[*websocket.Conn]struct{}
}

// New builds a Server from cfg.
func New(cfg Config) *Server {
	if cfg.TmuxBin == "" {
		cfg.TmuxBin = "tmux"
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	s := &Server{
		cfg:     cfg,
		auth:    auth.NewStore(cfg.PIN, cfg.TokenTTL, cfg.SecureCookies),
		limiter: auth.NewLimiter(5, 5*time.Minute, 5*time.Minute),
		bridges: make(map[string]int),
		socks:   make(map[*websocket.Conn]struct{}),
	}
	s.up = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     sameOrigin,
	}
	if cfg.BrandingDir != "" {
		for _, slot := range brandSlots {
			if path, ok := s.resolveBrand(slot); ok {
				cfg.Logger.Printf("branding: %s served from %s", slot.path, path)
			}
		}
	}
	return s
}

// resolveBrand finds an override file for slot, in precedence order:
// override_<slot>.<ext>, then override_logo.<ext>, then logo.<ext>. It returns
// the first match that exists in BrandingDir.
func (s *Server) resolveBrand(slot brandSlot) (string, bool) {
	dir := s.cfg.BrandingDir
	if dir == "" {
		return "", false
	}
	exts := brandExts
	if slot.rasterOnly {
		exts = rasterExts
	}
	var candidates []string
	// A per-slot override wins, then the shared override_logo, then the
	// committed default for the slot.
	for _, ext := range exts {
		candidates = append(candidates, "override_"+slot.name+"."+ext)
	}
	for _, ext := range exts {
		candidates = append(candidates, "override_logo."+ext)
	}
	for _, ext := range exts {
		candidates = append(candidates, slot.def+"."+ext)
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

func (s *Server) serveBrand(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	})
}

// trackSocket records a live WebSocket and returns a func to untrack it.
func (s *Server) trackSocket(ws *websocket.Conn) func() {
	s.mu.Lock()
	s.socks[ws] = struct{}{}
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.socks, ws)
		s.mu.Unlock()
	}
}

// CloseSockets sends a "going away" close to every live WebSocket. http.Server
// does not close hijacked connections, so this must run before Shutdown.
func (s *Server) CloseSockets() {
	s.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.socks))
	for ws := range s.socks {
		conns = append(conns, ws)
	}
	s.mu.Unlock()
	for _, ws := range conns {
		_ = ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			time.Now().Add(2*time.Second),
		)
	}
}

// addBridge records a portal attachment to name, returning a release func.
func (s *Server) addBridge(name string) func() {
	s.mu.Lock()
	s.bridges[name]++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			if s.bridges[name] <= 1 {
				delete(s.bridges, name)
			} else {
				s.bridges[name]--
			}
			s.mu.Unlock()
		})
	}
}

// bridgeCount reports how many portal attachments name currently has.
func (s *Server) bridgeCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bridges[name]
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/lock", s.handleLock)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/sessions", s.requireAuth(s.handleSessions))
	mux.HandleFunc("/api/sessions/create", s.requireAuth(s.handleCreateSession))
	mux.HandleFunc("/api/sessions/kill", s.requireAuth(s.handleKillSession))
	mux.HandleFunc("/api/attach", s.requireAuth(s.handleAttach))
	// Branding overrides sit in front of the static handler and the embedded
	// defaults; when none is found the request falls through to web/.
	for _, slot := range brandSlots {
		if path, ok := s.resolveBrand(slot); ok {
			mux.Handle(slot.path, s.serveBrand(path))
		}
	}
	mux.Handle("/", s.staticHandler())
	return s.securityHeaders(mux)
}

func (s *Server) staticHandler() http.Handler {
	sub, err := fs.Sub(s.cfg.WebFS, "web")
	if err != nil {
		s.cfg.Logger.Fatalf("web assets missing: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// xterm vendored assets are immutable for this build.
		if strings.HasPrefix(r.URL.Path, "/vendor/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			// Everything else is embedded and changes on rebuild; make
			// browsers revalidate so a restart is enough to pick it up.
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
		"form-action 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		s.logRequests(next).ServeHTTP(w, r)
	})
}

// logRequests logs API mutations (not static assets, the session polling
// endpoint, or the long-lived WebSocket upgrade) with method, path, status,
// and duration.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/sessions" || r.URL.Path == "/api/attach" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.cfg.Logger.Printf("%s %s %s %d %s", clientKey(r), r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder captures the response status. It forwards Hijack and Flush so
// wrapping does not break WebSocket upgrades or streaming.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return false
	}
	return s.auth.Valid(c.Value)
}

// handleHealthz is an unauthenticated liveness probe. It does not touch tmux.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	key := clientKey(r)
	if !s.limiter.Allowed(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again later"})
		return
	}

	var body struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if !s.auth.CheckPIN(body.PIN) {
		s.limiter.Fail(key)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "incorrect pin"})
		return
	}

	tok, err := s.auth.NewToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.limiter.Reset(key)
	s.auth.SetCookie(w, tok)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLock marks the client's session token as locked: HTTP access is
// refused until the PIN is entered again, but the token is kept, so already
// open WebSocket terminals survive. This is the difference from logout, which
// revokes the token outright and closes those terminals.
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.auth.Lock(c.Value)
	}
	s.auth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.auth.Revoke(c.Value)
	}
	s.auth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := tmux.ListSessions(s.cfg.TmuxBin)
	if err != nil {
		s.cfg.Logger.Printf("list sessions: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
		return
	}
	// Report attachedness from the perspective of non-portal clients, so the
	// kill warning and the tab marker mean "someone else is on this session".
	for i := range sessions {
		external := sessions[i].AttachedClients - s.bridgeCount(sessions[i].Name)
		sessions[i].Attached = external > 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name, ok := decodeSessionName(w, r)
	if !ok {
		return
	}
	if !tmux.ValidNewName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session name"})
		return
	}
	switch err := tmux.CreateSession(s.cfg.TmuxBin, name); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, tmux.ErrExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "session already exists"})
	default:
		s.cfg.Logger.Printf("create session %q: %v", name, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name, ok := decodeSessionName(w, r)
	if !ok {
		return
	}
	if !tmux.ValidName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session name"})
		return
	}
	if tmux.IsPinned(name) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the dominion session cannot be killed"})
		return
	}
	switch err := tmux.KillSession(s.cfg.TmuxBin, name); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, tmux.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
	default:
		s.cfg.Logger.Printf("kill session %q: %v", name, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to kill session"})
	}
}

// decodeSessionName reads {"name": "..."} from the body, writing the error
// response and returning false if it cannot.
func decodeSessionName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return "", false
	}
	return body.Name, true
}

func (s *Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("session")
	if !tmux.ValidName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session"})
		return
	}
	var cols, rows uint16
	if v := r.URL.Query().Get("cols"); v != "" {
		cols = parseUint16(v)
	}
	if v := r.URL.Query().Get("rows"); v != "" {
		rows = parseUint16(v)
	}
	if !tmux.HasSession(s.cfg.TmuxBin, name) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	tok := ""
	if c, err := r.Cookie(auth.CookieName); err == nil {
		tok = c.Value
	}
	ws, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		s.cfg.Logger.Printf("upgrade: %v", err)
		return
	}
	untrack := s.trackSocket(ws)
	defer untrack()
	release := s.addBridge(name)
	defer release()
	validate := func() bool { return s.auth.SocketValid(tok) }
	if err := ptybridge.Bridge(ws, s.cfg.TmuxBin, name, cols, rows, validate); err != nil && !errors.Is(err, ptybridge.ErrRevoked) {
		s.cfg.Logger.Printf("bridge %q: %v", name, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func parseUint16(s string) uint16 {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return 65535
		}
	}
	return uint16(n)
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser client (curl, test harness); cookie auth still applies.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
