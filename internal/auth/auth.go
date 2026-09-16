package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// CookieName is the cookie that carries a portal session token.
const CookieName = "dominion_auth"

// Store holds the configured PIN and the set of live session tokens.
type Store struct {
	pin    string
	ttl    time.Duration
	secure bool
	mu     sync.Mutex
	items  map[string]token
}

// token is a live session token. A locked token still authenticates WebSockets
// (so locking does not tear down open terminals) but is rejected for HTTP, so
// a reload or a new tab must re-enter the PIN.
type token struct {
	exp    time.Time
	locked bool
}

// NewStore creates a token store. Token lifetime is ttl. secure forces the
// Secure cookie attribute on every cookie, for a TLS-terminating proxy; when
// false, callers decide per request from the connection.
func NewStore(pin string, ttl time.Duration, secure bool) *Store {
	return &Store{
		pin:    pin,
		ttl:    ttl,
		secure: secure,
		items:  make(map[string]token),
	}
}

// CheckPIN compares the supplied PIN against the configured one in constant time.
func (s *Store) CheckPIN(pin string) bool {
	return subtle.ConstantTimeCompare([]byte(pin), []byte(s.pin)) == 1
}

// NewToken mints a random session token and records its expiry.
func (s *Store) NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.pruneLocked(time.Now())
	s.items[tok] = token{exp: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return tok, nil
}

// pruneLocked drops expired tokens. Callers must hold s.mu.
func (s *Store) pruneLocked(now time.Time) {
	for tok, t := range s.items {
		if now.After(t.exp) {
			delete(s.items, tok)
		}
	}
}

// lookupLocked returns the token record if present and unexpired. Callers must
// hold s.mu.
func (s *Store) lookupLocked(tok string, now time.Time) (token, bool) {
	t, ok := s.items[tok]
	if !ok {
		return token{}, false
	}
	if now.After(t.exp) {
		delete(s.items, tok)
		return token{}, false
	}
	return t, true
}

// Valid reports whether tok may be used for HTTP: live, unexpired, and not
// locked. A locked token is deliberately rejected here so locking forces the
// PIN again on reload or a new tab.
func (s *Store) Valid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.lookupLocked(tok, time.Now())
	return ok && !t.locked
}

// SocketValid reports whether an already-open WebSocket may stay attached:
// live and unexpired, regardless of lock state. Locking keeps terminals; only
// logout or expiry closes them.
func (s *Store) SocketValid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.lookupLocked(tok, time.Now())
	return ok
}

// Lock marks a token as locked: HTTP access is refused until the PIN is entered
// again, but existing WebSockets survive. The token is kept, not deleted.
func (s *Store) Lock(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.lookupLocked(tok, time.Now())
	if !ok {
		return
	}
	t.locked = true
	s.items[tok] = t
}

// Revoke invalidates a token outright, closing its WebSockets on the next
// revalidation.
func (s *Store) Revoke(tok string) {
	s.mu.Lock()
	delete(s.items, tok)
	s.mu.Unlock()
}

// SetCookie writes the auth cookie for a token. requestSecure is true when the
// request arrived over TLS; the store's configured secure flag forces it on
// regardless (for a TLS-terminating proxy in front of the server).
func (s *Store) SetCookie(w http.ResponseWriter, tok string, requestSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure || requestSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.ttl.Seconds()),
	})
}

// ClearCookie expires the auth cookie.
func (s *Store) ClearCookie(w http.ResponseWriter, requestSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure || requestSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Limiter throttles failed PIN attempts per client key.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*attempt
	max     int
	window  time.Duration
	lockout time.Duration
}

type attempt struct {
	count int
	last  time.Time
	until time.Time
}

// NewLimiter allows max failures per window before locking a key out for lockout.
func NewLimiter(max int, window, lockout time.Duration) *Limiter {
	return &Limiter{
		entries: make(map[string]*attempt),
		max:     max,
		window:  window,
		lockout: lockout,
	}
}

// Allowed reports whether key may currently attempt a login.
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.entries[key]
	if !ok {
		return true
	}
	return !time.Now().Before(a.until)
}

// Fail records a failed attempt and locks the key out once max is reached.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(now)
	a, ok := l.entries[key]
	if !ok || now.Sub(a.last) > l.window {
		a = &attempt{}
		l.entries[key] = a
	}
	a.count++
	a.last = now
	if a.count >= l.max {
		a.until = now.Add(l.lockout)
		a.count = 0
	}
}

// pruneLocked drops entries whose failure window and lockout have both passed.
// Callers must hold l.mu.
func (l *Limiter) pruneLocked(now time.Time) {
	for key, a := range l.entries {
		if now.After(a.until) && now.Sub(a.last) > l.window {
			delete(l.entries, key)
		}
	}
}

// Reset clears failures for a key after a successful login.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
