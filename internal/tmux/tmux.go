package tmux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// DashboardSession is the tmux session name for the all-sessions dashboard.
const DashboardSession = sessionPrefix + "dashboard"

// nestedAttachCommand returns the shell command a dashboard pane runs to
// attach a nested tmux client to the named session. TMUX= clears the env var
// so tmux allows nesting, and exec ties the pane lifetime to the inner client
// so the pane closes automatically when the session dies.
func nestedAttachCommand(name string) string {
	return "TMUX= exec tmux attach-session -t '" + exactTarget(name) + "'"
}

// DashboardSessions takes the active vibemux session set and returns the
// sessions the dashboard should display: everything except the dashboard
// itself, sorted alphabetically for stable pane order.
func DashboardSessions(active map[string]bool) []string {
	names := make([]string, 0, len(active))
	for name := range active {
		if name != DashboardSession {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// BuildDashboard creates a detached vmx-dashboard session containing one
// nested-client pane per given session, in the order given, tiled. On any
// failure the half-built dashboard is killed and the error returned. The
// caller is responsible for killing any pre-existing dashboard first.
func BuildDashboard(sessions []string) error {
	if len(sessions) == 0 {
		return errors.New("no sessions to show")
	}

	// split-window and select-layout take a pane target, where a bare
	// "=name" only resolves as a session; the trailing ":" means "exact
	// session, current window".
	windowTarget := exactTarget(DashboardSession) + ":"

	if err := exec.Command("tmux", "new-session", "-d", "-s", DashboardSession, nestedAttachCommand(sessions[0])).Run(); err != nil {
		return err
	}
	for _, name := range sessions[1:] {
		if err := exec.Command("tmux", "split-window", "-t", windowTarget, nestedAttachCommand(name)).Run(); err != nil {
			KillSession(DashboardSession)
			return err
		}
		// Retile after every split: split-window halves the current pane, so
		// without rebalancing a handful of sessions hits "pane too small".
		_ = exec.Command("tmux", "select-layout", "-t", windowTarget, "tiled").Run()
	}
	if err := exec.Command("tmux", "select-layout", "-t", windowTarget, "tiled").Run(); err != nil {
		KillSession(DashboardSession)
		return err
	}
	return nil
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
