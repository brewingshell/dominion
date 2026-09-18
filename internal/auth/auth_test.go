package auth

import (
	"testing"
	"time"
)

func TestCheckPIN(t *testing.T) {
	s := NewStore("3232", time.Hour, false)
	if !s.CheckPIN("3232") {
		t.Error("correct PIN rejected")
	}
	if s.CheckPIN("0000") {
		t.Error("incorrect PIN accepted")
	}
	if s.CheckPIN("323") {
		t.Error("partial PIN accepted")
	}
}

func TestSetPIN(t *testing.T) {
	s := NewStore("3232", time.Hour, false)
	s.SetPIN("5683")
	if s.CheckPIN("3232") {
		t.Error("old PIN still accepted after SetPIN")
	}
	if !s.CheckPIN("5683") {
		t.Error("new PIN rejected after SetPIN")
	}
}

func TestRevokeAllExcept(t *testing.T) {
	s := NewStore("3232", time.Hour, false)
	keep, _ := s.NewToken()
	other, _ := s.NewToken()
	s.RevokeAllExcept(keep)
	if !s.Valid(keep) {
		t.Error("the kept token should stay valid")
	}
	if s.Valid(other) {
		t.Error("other tokens should be revoked")
	}
}

func TestTokenLifecycle(t *testing.T) {
	s := NewStore("3232", time.Hour, false)
	tok, err := s.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if !s.Valid(tok) {
		t.Error("fresh token should be valid")
	}
	if s.Valid("nope") {
		t.Error("unknown token should be invalid")
	}
	s.Revoke(tok)
	if s.Valid(tok) {
		t.Error("revoked token should be invalid")
	}
}

func TestTokenExpiry(t *testing.T) {
	s := NewStore("3232", -time.Second, false)
	tok, _ := s.NewToken()
	if s.Valid(tok) {
		t.Error("expired token should be invalid")
	}
}

func TestLockKeepsSocketsAlive(t *testing.T) {
	s := NewStore("3232", time.Hour, false)
	tok, _ := s.NewToken()

	s.Lock(tok)
	if s.Valid(tok) {
		t.Error("locked token must be rejected for HTTP")
	}
	if !s.SocketValid(tok) {
		t.Error("locked token must still validate existing sockets")
	}

	s.Revoke(tok)
	if s.SocketValid(tok) {
		t.Error("revoked token must invalidate sockets")
	}
	if s.Valid(tok) {
		t.Error("revoked token must be rejected for HTTP")
	}
}

func TestExpiredTokenInvalidatesSockets(t *testing.T) {
	s := NewStore("3232", -time.Second, false)
	tok, _ := s.NewToken()
	if s.SocketValid(tok) {
		t.Error("expired token must invalidate sockets")
	}
}

func TestTokenPruning(t *testing.T) {
	s := NewStore("3232", -time.Second, false)
	expired, _ := s.NewToken()
	s.mu.Lock()
	n := len(s.items)
	s.mu.Unlock()
	if n == 0 {
		t.Fatal("expired token should still be stored until pruned")
	}
	s.NewToken() // triggers prune
	s.mu.Lock()
	_, present := s.items[expired]
	s.mu.Unlock()
	if present {
		t.Error("expired token should be pruned on the next mint")
	}
}

func TestLimiterPruning(t *testing.T) {
	l := NewLimiter(5, 10*time.Millisecond, 10*time.Millisecond)
	l.Fail("a")
	l.Fail("b")
	time.Sleep(30 * time.Millisecond)
	l.Fail("c") // triggers prune; a and b are now stale
	l.mu.Lock()
	_, hasA := l.entries["a"]
	_, hasB := l.entries["b"]
	_, hasC := l.entries["c"]
	l.mu.Unlock()
	if hasA || hasB {
		t.Error("stale limiter entries should be pruned")
	}
	if !hasC {
		t.Error("current entry should remain")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute, time.Minute)
	key := "1.2.3.4"
	if !l.Allowed(key) {
		t.Fatal("fresh key should be allowed")
	}
	l.Fail(key)
	l.Fail(key)
	if !l.Allowed(key) {
		t.Fatal("below threshold should stay allowed")
	}
	l.Fail(key)
	if l.Allowed(key) {
		t.Fatal("at threshold should be locked out")
	}
	l.Reset(key)
	if !l.Allowed(key) {
		t.Fatal("reset should clear lockout")
	}
}

func TestLimiterWindowReset(t *testing.T) {
	l := NewLimiter(2, 10*time.Millisecond, time.Minute)
	key := "1.2.3.4"
	l.Fail(key)
	time.Sleep(20 * time.Millisecond)
	l.Fail(key)
	if !l.Allowed(key) {
		t.Fatal("failures outside window should not accumulate")
	}
}
