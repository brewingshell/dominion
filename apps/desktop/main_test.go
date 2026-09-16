package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReachable(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("probe hit %s, want /healthz", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()

	if !reachable("http", strings.TrimPrefix(ok.URL, "http://")) {
		t.Error("live server should be reachable")
	}

	// A closed listener is unreachable.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadAddr := strings.TrimPrefix(dead.URL, "http://")
	dead.Close()
	if reachable("http", deadAddr) {
		t.Error("closed server should be unreachable")
	}
}

func TestPromptHTMLInlinesBootstrapAndSavedConfig(t *testing.T) {
	html := promptHTML(config{Host: "192.168.1.5", Scheme: "http"}, "", "")

	if strings.Contains(html, `<script src="bootstrap.js">`) {
		t.Error("external bootstrap script should have been inlined")
	}
	if !strings.Contains(html, "window.__dominionSaved=") {
		t.Error("saved config should be inlined")
	}
	if !strings.Contains(html, "192.168.1.5") {
		t.Error("saved host should be present")
	}
	if strings.Contains(html, `window.__dominionLoadError="`) {
		t.Error("no error was passed, so none should be inlined")
	}
	if strings.Contains(html, "window.__dominionView=") {
		t.Error("no view was requested, so none should be inlined")
	}
	// bootstrap.js must actually be present, not just referenced.
	if !strings.Contains(html, "dominionConnect") {
		t.Error("bootstrap body should be inlined verbatim")
	}
}

func TestPromptHTMLInlinesError(t *testing.T) {
	html := promptHTML(config{}, "Could not reach http://nope:5550.", "")
	if !strings.Contains(html, `window.__dominionLoadError="`) {
		t.Error("error message should be inlined for the desktop shell")
	}
	if !strings.Contains(html, "nope:5550") {
		t.Error("error text should be present")
	}
}

func TestPromptHTMLInlinesView(t *testing.T) {
	html := promptHTML(config{}, "", "settings")
	if !strings.Contains(html, `window.__dominionView="settings"`) {
		t.Error("requested view should be inlined")
	}
}

func TestSaveDeleteHost(t *testing.T) {
	// Isolate the config file in a temp HOME.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := saveHost(savedHost{Name: "home", Host: "10.0.0.5:5550", Scheme: "http"}); err != nil {
		t.Fatalf("saveHost: %v", err)
	}
	// Same name updates in place rather than duplicating.
	if err := saveHost(savedHost{Name: "home", Host: "10.0.0.6:5550", Scheme: "https"}); err != nil {
		t.Fatalf("saveHost update: %v", err)
	}
	// A connect must not wipe the address book.
	c := loadConfig()
	c.Host, c.Scheme = "10.0.0.9:5550", "http"
	if err := saveConfig(c); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got := loadConfig()
	if len(got.Saved) != 1 {
		t.Fatalf("saved = %+v, want 1 entry", got.Saved)
	}
	if got.Saved[0].Host != "10.0.0.6:5550" || got.Saved[0].Scheme != "https" {
		t.Errorf("entry not updated in place: %+v", got.Saved[0])
	}

	if err := deleteHost("home"); err != nil {
		t.Fatalf("deleteHost: %v", err)
	}
	if len(loadConfig().Saved) != 0 {
		t.Error("entry should be gone")
	}
	if err := deleteHost("missing"); err != nil {
		t.Errorf("deleting a missing name should be a no-op, got %v", err)
	}
}

func TestSavedListBounded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	for i := 0; i < maxSavedHosts+5; i++ {
		name := fmt.Sprintf("h%02d", i)
		if err := saveHost(savedHost{Name: name, Host: "h:5550", Scheme: "http"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(loadConfig().Saved); n != maxSavedHosts {
		t.Fatalf("saved hosts = %d, want %d", n, maxSavedHosts)
	}
}
