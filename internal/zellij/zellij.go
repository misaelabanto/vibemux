package zellij

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const sessionPrefix = "vmx-"

// SessionName returns a deterministic zellij session name derived from the
// base directory name of the project path (e.g. "vmx-myproject").
func SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

// DashboardSession is the zellij session name for the all-sessions dashboard.
const DashboardSession = sessionPrefix + "dashboard"

// binaryPath resolves the zellij binary: PATH first, then ~/.local/bin,
// which is where zellij's own installer puts it and which is often missing
// from non-login-shell PATHs.
func binaryPath() string {
	if p, err := exec.LookPath("zellij"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".local", "bin", "zellij")
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p
	}
	return ""
}

// command builds an *exec.Cmd for the resolved zellij binary.
func command(args ...string) *exec.Cmd {
	return exec.Command(binaryPath(), args...)
}

// IsInstalled checks whether zellij is available on PATH or in ~/.local/bin.
func IsInstalled() bool {
	return binaryPath() != ""
}

// liveSessions returns the set of currently live session names. zellij has
// no has-session command and `list-sessions -ns` mixes live and EXITED
// (dead but resurrectable) sessions indistinguishably, so this parses the
// long format and drops EXITED lines.
func liveSessions() map[string]bool {
	out, err := command("list-sessions", "-n").Output()
	if err != nil {
		// zellij exits non-zero when no sessions exist; treat as empty.
		return map[string]bool{}
	}

	live := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "EXITED") {
			continue
		}
		name, _, _ := strings.Cut(line, " ")
		live[name] = true
	}
	return live
}

// HasSession checks whether a live zellij session with the given name exists.
// Names are matched exactly; EXITED sessions do not count.
func HasSession(name string) bool {
	return liveSessions()[name]
}

// NewSession creates a new detached zellij session with the given name and
// working directory. The working directory is set both on the process (zellij
// has no --cwd flag for session creation) and via the options subcommand so
// panes opened later in the session also start there.
//
// The session is created with web sharing enabled so it is reachable through
// the zellij web server (the vibemux dashboard). The option must travel in a
// config file passed via --config, and that file must outlive session
// creation; see webSharingConfig for both reasons.
func NewSession(name, dir string) error {
	cfg, err := webSharingConfig(name)
	if err != nil {
		return err
	}
	cmd := command("--config", cfg, "attach", "--create-background", name, "options", "--default-cwd", dir)
	cmd.Dir = dir
	return cmd.Run()
}

// AttachCommand returns an *exec.Cmd that attaches to the named zellij
// session. Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors
// so that bubbletea's ExecProcess won't override them with its wrapped
// readers/writers, which zellij cannot use (it needs a real /dev/tty).
func AttachCommand(name string) *exec.Cmd {
	cmd := command("attach", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// KillSession destroys the named zellij session. kill-session alone leaves a
// resurrectable EXITED corpse behind (serialized to the cache dir), so the
// session is also deleted, best effort, to match tmux semantics where a
// killed session is gone.
func KillSession(name string) error {
	err := command("kill-session", name).Run()
	_ = command("delete-session", "--force", name).Run()
	return err
}

// ListVibemuxSessions returns a set of live zellij session names that have
// the vibemux prefix. Returns an empty map when no sessions exist.
func ListVibemuxSessions() (map[string]bool, error) {
	sessions := map[string]bool{}
	for name := range liveSessions() {
		if strings.HasPrefix(name, sessionPrefix) {
			sessions[name] = true
		}
	}
	return sessions, nil
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

// mirrorScript returns the bash one-liner a dashboard pane runs to mirror the
// named session: poll dump-screen once a second and repaint in place. The
// pane id must be explicit because the focused pane of an unattached session
// can be a floating plugin (e.g. release notes), which dumps nothing;
// terminal_0 is the main shell pane in every session vibemux creates. The
// pane inherits ZELLIJ_SOCKET_DIR/XDG_CACHE_HOME from the server, so the
// dump targets the same server that hosts the dashboard.
func mirrorScript(bin, name string) string {
	return "while true; do out=$('" + bin + "' --session '" + name +
		"' action dump-screen --pane-id terminal_0 --ansi 2>/dev/null); " +
		"printf '\\033[H\\033[2J%s' \"$out\"; sleep 1; done"
}

// dashboardLayout generates a KDL layout with one mirror pane per session,
// tiled as a grid (rows of columns, filled breadth-first). zellij has no
// tmux-like "tiled" auto-layout command, so the grid is computed here.
func dashboardLayout(bin string, sessions []string) string {
	cols := int(math.Ceil(math.Sqrt(float64(len(sessions)))))
	var b strings.Builder
	b.WriteString("layout {\n")
	for i := 0; i < len(sessions); i += cols {
		end := i + cols
		if end > len(sessions) {
			end = len(sessions)
		}
		b.WriteString("    pane split_direction=\"vertical\" {\n")
		for _, name := range sessions[i:end] {
			fmt.Fprintf(&b, "        pane command=\"bash\" name=%s {\n", strconv.Quote(name))
			fmt.Fprintf(&b, "            args \"-c\" %s\n", strconv.Quote(mirrorScript(bin, name)))
			b.WriteString("        }\n")
		}
		b.WriteString("    }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// BuildDashboard creates a detached vmx-dashboard session containing one
// mirror pane per given session, in the order given, tiled. On any failure
// the half-built dashboard is killed and the error returned. The caller is
// responsible for killing any pre-existing dashboard first.
//
// Panes mirror their session via polling `zellij action dump-screen` rather
// than nested attach: nested attach is broken upstream (issues #3411, #3519,
// #3847; reproduced with 0.44.3, all inner clients render into the first
// pane). A plain subprocess printing ANSI is ordinary pane content, so the
// bug does not apply. The mirror is read-only; input forwarding is a
// follow-up (proven possible via `zellij action write-chars` / `write`
// with explicit --session and --pane-id).
func BuildDashboard(sessions []string) error {
	if len(sessions) == 0 {
		return errors.New("no sessions to show")
	}
	bin := binaryPath()
	if bin == "" {
		return errors.New("zellij not installed")
	}

	layoutPath := filepath.Join(os.TempDir(), "vmx-dashboard-layout.kdl")
	if err := os.WriteFile(layoutPath, []byte(dashboardLayout(bin, sessions)), 0o600); err != nil {
		return err
	}
	if err := command("attach", "--create-background", DashboardSession, "options", "--default-layout", layoutPath).Run(); err != nil {
		_ = KillSession(DashboardSession)
		return err
	}
	return nil
}
