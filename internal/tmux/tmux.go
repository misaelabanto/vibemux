package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sessionPrefix = "vmx-"

// SessionName returns a deterministic tmux session name derived from the
// base directory name of the project path (e.g. "vibemux-myproject").
func SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

// IsInstalled checks whether tmux is available on PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// exactTarget prefixes a session name with "=" so tmux performs an exact-name
// lookup instead of its default prefix match. Without this, `vmx-agendalo`
// would match an existing `vmx-agendalo-app-nuxt` and the wrong session would
// be returned/attached/killed.
func exactTarget(name string) string {
	return "=" + name
}

// HasSession checks whether a tmux session with the given name exists.
func HasSession(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", exactTarget(name)).Run()
	return err == nil
}

// NewSession creates a new detached tmux session with the given name and
// working directory.
func NewSession(name, dir string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", name, "-c", dir).Run()
}

// AttachCommand returns an *exec.Cmd that attaches to the named tmux session.
// Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors so that
// bubbletea's ExecProcess won't override them with its wrapped readers/writers,
// which tmux cannot use (it needs a real /dev/tty).
func AttachCommand(name string) *exec.Cmd {
	cmd := exec.Command("tmux", "attach-session", "-t", exactTarget(name))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// KillSession destroys the named tmux session.
func KillSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", exactTarget(name)).Run()
}

// ListVibemuxSessions returns a set of active tmux session names that have the
// vibemux prefix.
func ListVibemuxSessions() (map[string]bool, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		// tmux returns error when no server is running — treat as empty.
		return map[string]bool{}, nil
	}

	sessions := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, sessionPrefix) {
			sessions[line] = true
		}
	}
	return sessions, nil
}
