package tmux

import (
	"bytes"
	"errors"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Pinned is the session that hosts the portal and is always shown first. It
// cannot be killed: doing so would take the server down.
const Pinned = "dominion"

// IsPinned reports whether name is the pinned portal session.
func IsPinned(name string) bool {
	return name == Pinned
}

// ErrExists reports that a session with the requested name already exists.
var ErrExists = errors.New("session already exists")

// ErrNotFound reports that the requested session does not exist.
var ErrNotFound = errors.New("session not found")

// ErrPinned reports an attempt to act on the pinned session in a way that is
// not allowed (killing it).
var ErrPinned = errors.New("the dominion session cannot be killed")

// Session is a tmux session as exposed to the portal.
type Session struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
	// AttachedClients is the raw tmux client count. It is not sent to the
	// client, which sees the portal-adjusted Attached bool instead.
	AttachedClients int `json:"-"`
}

const format = "#{session_name}\t#{session_windows}\t#{session_attached}"

// ListSessions returns the tmux sessions on the default server. A missing
// server or missing sessions is not an error: the caller gets an empty slice.
func ListSessions(bin string) ([]Session, error) {
	if bin == "" {
		bin = "tmux"
	}
	cmd := exec.Command(bin, "list-sessions", "-F", format)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.ToLower(stderr.String())
		for _, needle := range []string{
			"no server running",
			"failed to connect",
			"no such file or directory",
			"error connecting",
		} {
			if strings.Contains(msg, needle) {
				return []Session{}, nil
			}
		}
		// tmux exits non-zero when no sessions exist; treat any such case as empty.
		if stdout.Len() == 0 {
			return []Session{}, nil
		}
		return nil, err
	}
	return parseSessions(stdout.String()), nil
}

func parseSessions(out string) []Session {
	sessions := []Session{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		windows, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		clients, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
		sessions = append(sessions, Session{
			Name:            strings.TrimSpace(fields[0]),
			Windows:         windows,
			Attached:        clients > 0,
			AttachedClients: clients,
		})
	}
	// Pinned session first, then alphabetical (display order only).
	sort.Slice(sessions, func(i, j int) bool {
		pi, pj := sessions[i].Name == Pinned, sessions[j].Name == Pinned
		if pi != pj {
			return pi
		}
		return sessions[i].Name < sessions[j].Name
	})
	return sessions
}

// ValidName reports whether a session name is safe to pass to tmux. Names are
// passed as argv (never through a shell), but we still reject control
// characters and unreasonable lengths.
func ValidName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// ValidNewName reports whether a name may be used to create a session. In
// addition to ValidName it rejects tmux target separators, which would make
// the name ambiguous when passed to new-session or kill-session.
func ValidNewName(name string) bool {
	if !ValidName(name) {
		return false
	}
	return !strings.ContainsAny(name, ".:")
}

// target renders an exact tmux target for name. Without the "=" prefix tmux
// resolves a target by exact name, then prefix, then fnmatch, so a name like
// "web*" or "web" could match the wrong session.
func target(name string) string {
	return "=" + name
}

// HasSession reports whether a session named name currently exists.
func HasSession(bin, name string) bool {
	if !ValidName(name) {
		return false
	}
	if bin == "" {
		bin = "tmux"
	}
	return exec.Command(bin, "has-session", "-t", target(name)).Run() == nil
}

// CreateSession starts a new detached session named name.
func CreateSession(bin, name string) error {
	if !ValidNewName(name) {
		return errors.New("invalid session name")
	}
	if bin == "" {
		bin = "tmux"
	}
	out, err := run(bin, "new-session", "-d", "-s", name)
	if err != nil {
		msg := strings.ToLower(out)
		if strings.Contains(msg, "duplicate session") || strings.Contains(msg, "already exists") {
			return ErrExists
		}
		return err
	}
	return nil
}

// KillSession destroys the session named name along with everything in it. The
// pinned session is refused, since killing it would stop the portal.
func KillSession(bin, name string) error {
	if !ValidName(name) {
		return errors.New("invalid session name")
	}
	if IsPinned(name) {
		return ErrPinned
	}
	if bin == "" {
		bin = "tmux"
	}
	out, err := run(bin, "kill-session", "-t", target(name))
	if err != nil {
		msg := strings.ToLower(out)
		for _, needle := range []string{"can't find session", "no such session", "session not found"} {
			if strings.Contains(msg, needle) {
				return ErrNotFound
			}
		}
		return err
	}
	return nil
}

// run executes tmux with args and returns the combined output. A non-zero exit
// becomes an error whose text is the trimmed tmux output.
func run(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(buf.String()), err
	}
	return strings.TrimSpace(buf.String()), nil
}
