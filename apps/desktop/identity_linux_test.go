//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppIconPathAppDir(t *testing.T) {
	dir := t.TempDir()
	icon := filepath.Join(dir, "dominion.png")
	if err := os.WriteFile(icon, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIR", dir)

	got := appIconPath()
	if got != icon {
		t.Fatalf("appIconPath() = %q, want %q", got, icon)
	}
}

func TestAppIconPathAppDirHicolor(t *testing.T) {
	dir := t.TempDir()
	hicolor := filepath.Join(dir, "usr", "share", "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(hicolor, 0o700); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(hicolor, "dominion.png")
	if err := os.WriteFile(icon, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIR", dir)

	if got := appIconPath(); got != icon {
		t.Fatalf("appIconPath() = %q, want %q", got, icon)
	}
}

func TestAppIconPathMissing(t *testing.T) {
	// No APPDIR, and the test binary has no sibling dominion.png.
	t.Setenv("APPDIR", "")
	// Do not assert empty unconditionally: a sibling icon could exist. Just
	// assert it never points at a directory and returns a stable result.
	got := appIconPath()
	if got != "" {
		if info, err := os.Stat(got); err != nil || info.IsDir() {
			t.Fatalf("appIconPath() = %q, which is not a file", got)
		}
	}
}
