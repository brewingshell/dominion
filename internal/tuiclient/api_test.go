package tuiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testServer is a minimal dominion API double.
func testServer(t *testing.T, pin string) (*httptest.Server, *struct {
	created string
	killed  string
}) {
	t.Helper()
	state := &struct {
		created string
		killed  string
	}{}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PIN string `json:"pin"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.PIN != pin {
			http.Error(w, `{"error":"incorrect pin"}`, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "dominion_auth", Value: "tok", Path: "/"})
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		c, err := r.Cookie("dominion_auth")
		if err != nil || c.Value != "tok" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []Session{
			{Name: "dominion", Windows: 1},
			{Name: "work", Windows: 2, Attached: true},
		}})
	})
	mux.HandleFunc("/api/sessions/create", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		state.created = body.Name
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/sessions/kill", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Name == "dominion" {
			http.Error(w, `{"error":"the dominion session cannot be killed"}`, http.StatusForbidden)
			return
		}
		state.killed = body.Name
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/lock", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func TestClientFlow(t *testing.T) {
	const pin = "1234"
	srv, state := testServer(t, pin)

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Healthz(ctx); err != nil {
		t.Fatalf("Healthz: %v", err)
	}
	if err := c.Login(ctx, "0000"); !errors.Is(err, ErrBadPIN) {
		t.Fatalf("Login wrong pin = %v, want ErrBadPIN", err)
	}
	if err := c.Login(ctx, pin); err != nil {
		t.Fatalf("Login: %v", err)
	}

	sessions, err := c.Sessions(ctx)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 2 || sessions[1].Name != "work" || !sessions[1].Attached {
		t.Fatalf("Sessions = %+v", sessions)
	}

	if err := c.Create(ctx, "newone"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if state.created != "newone" {
		t.Fatalf("created = %q", state.created)
	}
	if err := c.Kill(ctx, "work"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if state.killed != "work" {
		t.Fatalf("killed = %q", state.killed)
	}
	if err := c.Kill(ctx, "dominion"); err == nil {
		t.Fatal("Kill(pinned) succeeded, want error")
	}
	if err := c.Lock(ctx); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("Logout: %v", err)
	}
}

func TestSessionsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Sessions(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Sessions = %v, want ErrUnauthorized", err)
	}
}

func TestNormalizeBase(t *testing.T) {
	tests := []struct {
		scheme, host, want string
		wantErr            bool
	}{
		{"http", "example.com", "http://example.com:5550", false},
		{"https", "example.com:8443", "https://example.com:8443", false},
		{"http", "192.168.1.5", "http://192.168.1.5:5550", false},
		{"http", "http://a.b:5550/", "http://a.b:5550", false},
		{"ftp", "example.com", "", true},
		{"http", "  ", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeBase(tt.scheme, tt.host)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NormalizeBase(%q,%q) = %q, want error", tt.scheme, tt.host, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeBase(%q,%q) = %q, %v; want %q", tt.scheme, tt.host, got, err, tt.want)
		}
	}
}
