package tuiclient

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempHome points ConfigPath at a throwaway directory.
func useTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestConfigRoundTrip(t *testing.T) {
	useTempHome(t)

	c := Config{Host: "box:5550", Scheme: "http", Theme: "light"}
	c.Saved = []SavedHost{{Name: "box", Host: "box:5550", Scheme: "http"}}
	if err := SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got := LoadConfig()
	if got.Host != "box:5550" || got.Scheme != "http" || got.Theme != "light" {
		t.Fatalf("LoadConfig = %+v", got)
	}
	if len(got.Saved) != 1 || got.Saved[0].Name != "box" {
		t.Fatalf("Saved = %+v", got.Saved)
	}

	info, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config mode = %o, want 600", perm)
	}
}

// A config written by the desktop client has no theme key; the TUI must load it
// without error and leave the theme empty.
func TestConfigDesktopCompatibility(t *testing.T) {
	home := useTempHome(t)
	path := filepath.Join(home, ".config", "dominion", "client.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	desktop := `{"host":"lan:5550","scheme":"http","saved":[{"name":"lan","host":"lan:5550","scheme":"http"}]}`
	if err := os.WriteFile(path, []byte(desktop), 0o600); err != nil {
		t.Fatal(err)
	}

	got := LoadConfig()
	if got.Host != "lan:5550" || got.Theme != "" || len(got.Saved) != 1 {
		t.Fatalf("LoadConfig = %+v", got)
	}
}

func TestConfigMissingFileDefaults(t *testing.T) {
	useTempHome(t)
	got := LoadConfig()
	if got.Scheme != "http" {
		t.Fatalf("default scheme = %q, want http", got.Scheme)
	}
}

func TestSaveHostUpsertAndDelete(t *testing.T) {
	useTempHome(t)

	c := Config{}
	if err := c.SaveHost(SavedHost{Name: "a", Host: "a:5550", Scheme: "http"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveHost(SavedHost{Name: "a", Host: "a2:5550", Scheme: "https"}); err != nil {
		t.Fatal(err)
	}
	if len(c.Saved) != 1 || c.Saved[0].Host != "a2:5550" {
		t.Fatalf("upsert failed: %+v", c.Saved)
	}
	if err := c.SaveHost(SavedHost{Name: "b", Host: "b:5550", Scheme: "http"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteHost("a"); err != nil {
		t.Fatal(err)
	}
	if len(c.Saved) != 1 || c.Saved[0].Name != "b" {
		t.Fatalf("after delete: %+v", c.Saved)
	}
	// Deleting a missing name is not an error.
	if err := c.DeleteHost("nope"); err != nil {
		t.Fatalf("DeleteHost(missing): %v", err)
	}
}

func TestMaxSavedHosts(t *testing.T) {
	useTempHome(t)
	c := Config{}
	for i := 0; i < maxSavedHosts+5; i++ {
		name := string(rune('a' + i))
		if err := c.SaveHost(SavedHost{Name: name, Host: name, Scheme: "http"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.Saved) != maxSavedHosts {
		t.Fatalf("saved hosts = %d, want %d", len(c.Saved), maxSavedHosts)
	}
}
