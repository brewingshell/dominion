package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestParseSessions(t *testing.T) {
	out := "adjutant\t1\t1\naiur\t2\t0\ncomfy\t1\t0\n"
	got := parseSessions(out)
	if len(got) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(got))
	}
	if got[0].Name != "adjutant" || got[0].Windows != 1 || !got[0].Attached {
		t.Errorf("unexpected first session: %+v", got[0])
	}
	if got[1].Name != "aiur" || got[1].Windows != 2 || got[1].Attached {
		t.Errorf("unexpected second session: %+v", got[1])
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	if got := parseSessions(""); len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestParseSessionsSkipsMalformed(t *testing.T) {
	got := parseSessions("good\t1\t0\nbad-line\n")
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("unexpected sessions: %+v", got)
	}
}

func TestParseSessionsSortsByName(t *testing.T) {
	got := parseSessions("zeta\t1\t0\nalpha\t1\t2\nmid\t1\t0\n")
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	want := []string{"alpha", "mid", "zeta"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if got[0].AttachedClients != 2 || !got[0].Attached {
		t.Fatalf("alpha should report 2 attached clients: %+v", got[0])
	}
}

func TestParseSessionsPinnedFirst(t *testing.T) {
	got := parseSessions("alpha\t1\t0\ndominion\t1\t0\nzeta\t1\t0\n")
	if got[0].Name != Pinned {
		t.Fatalf("pinned session should sort first, got %q", got[0].Name)
	}
	if got[1].Name != "alpha" || got[2].Name != "zeta" {
		t.Fatalf("remaining sessions should stay alphabetical: %+v", got)
	}
}

func TestIsPinned(t *testing.T) {
	if !IsPinned(Pinned) {
		t.Error("Pinned should be pinned")
	}
	if IsPinned("adjutant") {
		t.Error("ordinary session should not be pinned")
	}
}

func TestKillRefusesPinned(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	if err := KillSession("tmux", Pinned); !errors.Is(err, ErrPinned) {
		t.Fatalf("killing the pinned session: want ErrPinned, got %v", err)
	}
}

func TestValidName(t *testing.T) {
	cases := map[string]bool{
		"rabota":      true,
		"my session":  true,
		"":            false,
		"bad\nname":   false,
		"nul\x00name": false,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidNewName(t *testing.T) {
	cases := map[string]bool{
		"rabota":     true,
		"my session": true,
		"a-b_c.":     false,
		"a:b":        false,
		"":           false,
		"bad\nname":  false,
	}
	for name, want := range cases {
		if got := ValidNewName(name); got != want {
			t.Errorf("ValidNewName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCreateKillSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	name := fmt.Sprintf("dominion_tmux_%d", os.Getpid())
	t.Cleanup(func() { _ = KillSession("tmux", name) })

	if HasSession("tmux", name) {
		t.Fatalf("session %q should not exist yet", name)
	}
	if err := CreateSession("tmux", name); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !HasSession("tmux", name) {
		t.Fatalf("HasSession should report the created session")
	}
	if err := CreateSession("tmux", name); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate create: want ErrExists, got %v", err)
	}

	sessions, err := ListSessions("tmux")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.Name == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("created session %q not listed", name)
	}

	if err := KillSession("tmux", name); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := KillSession("tmux", name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kill missing: want ErrNotFound, got %v", err)
	}
}

// TestTargetsAreExact guards against tmux resolving a target by prefix or
// fnmatch: killing "ext" must not touch "extended".
func TestTargetsAreExact(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	base := fmt.Sprintf("dominion_exact_%d", os.Getpid())
	short, long := base+"_a", base+"_ab"
	t.Cleanup(func() {
		_ = KillSession("tmux", short)
		_ = KillSession("tmux", long)
	})

	if err := CreateSession("tmux", short); err != nil {
		t.Fatalf("create short: %v", err)
	}
	if err := CreateSession("tmux", long); err != nil {
		t.Fatalf("create long: %v", err)
	}

	if err := KillSession("tmux", short); err != nil {
		t.Fatalf("kill short: %v", err)
	}
	if !HasSession("tmux", long) {
		t.Fatal("killing a prefix must not affect a longer session name")
	}
	if HasSession("tmux", short) {
		t.Fatal("the exact session should be gone")
	}
}

func TestTargetPrefix(t *testing.T) {
	if got := target("web"); got != "=web" {
		t.Fatalf("target(%q) = %q, want exact marker", "web", got)
	}
}
