package main

import (
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
	html := promptHTML(config{Host: "192.168.1.5", Scheme: "http"}, "")

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
	// bootstrap.js must actually be present, not just referenced.
	if !strings.Contains(html, "dominionConnect") {
		t.Error("bootstrap body should be inlined verbatim")
	}
}

func TestPromptHTMLInlinesError(t *testing.T) {
	html := promptHTML(config{}, "Could not reach http://nope:5550.")
	if !strings.Contains(html, `window.__dominionLoadError="`) {
		t.Error("error message should be inlined for the desktop shell")
	}
	if !strings.Contains(html, "nope:5550") {
		t.Error("error text should be present")
	}
}
