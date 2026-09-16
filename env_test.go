package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnv(t *testing.T) {
	content := `
# comment
DOMINION_PIN=1234

export DOMINION_ADDR=":5550"
QUOTED='single value'
WITH_EQUALS=a=b
NOVALUE
  SPACED = padded
`
	got := parseEnv(content)
	want := map[string]string{
		"DOMINION_PIN":  "1234",
		"DOMINION_ADDR": ":5550",
		"QUOTED":        "single value",
		"WITH_EQUALS":   "a=b",
		"SPACED":        "padded",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseEnv[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["NOVALUE"]; ok {
		t.Error("a line without '=' should be ignored")
	}
	if len(got) != len(want) {
		t.Errorf("parsed %d keys, want %d: %v", len(got), len(want), got)
	}
}

func TestPinFromEnvDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp) // no ./.env here
	t.Setenv("DOMINION_PIN", "")
	t.Setenv("DOMINION_ENV", "")

	if got := pinFromEnv(tmp); got != defaultPINValue {
		t.Fatalf("pinFromEnv = %q, want %q", got, defaultPINValue)
	}
}

func TestPinFromEnvVariableWins(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("DOMINION_PIN", "9999")

	// A .env file must not override an exported variable.
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("DOMINION_PIN=1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := pinFromEnv(tmp); got != "9999" {
		t.Fatalf("pinFromEnv = %q, want %q", got, "9999")
	}
}

func TestPinFromEnvFile(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("DOMINION_PIN", "")
	t.Setenv("DOMINION_ENV", "")

	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("# cfg\nDOMINION_PIN=4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := pinFromEnv(tmp); got != "4321" {
		t.Fatalf("pinFromEnv = %q, want %q", got, "4321")
	}
}

func TestPinFromEnvExplicitPath(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("DOMINION_PIN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("DOMINION_PIN=2468\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOMINION_ENV", path)

	if got := pinFromEnv(tmp); got != "2468" {
		t.Fatalf("pinFromEnv = %q, want %q", got, "2468")
	}
}
