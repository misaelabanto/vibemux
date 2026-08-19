package zellij

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sessionPrefix = "vmx-"

// maxSocketPath is the longest unix domain socket path zellij will accept. It
// is the platform's sun_path limit (104 bytes) minus the terminating NUL.
const maxSocketPath = 103

// Backend drives zellij. It has no state; methods shell out to the zellij
// binary resolved by binaryPath.
type Backend struct{}

// Name is the multiplexer's persisted, human-readable identity.
func (Backend) Name() string { return "zellij" }

// SessionName returns a deterministic zellij session name derived from the
// base directory name of the project path (e.g. "vmx-myproject").
func (Backend) SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

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

// socketDir returns the directory zellij should place its IPC sockets in.
//
// zellij builds a unix socket path of <socket dir>/contract_version_1/<session
// name>, and a unix socket path cannot exceed 103 bytes. zellij's default
// socket dir is $TMPDIR/zellij-<uid>, and on macOS $TMPDIR is a ~49-byte
// per-user path, which leaves only ~24 bytes for the session name: a project
// named "agendalo-nuxt-frontend" overflows the limit and zellij fails with a
// bare exit status 1. Anchoring the socket dir at /tmp buys the budget back so
// any realistic project name fits. On Linux, where $TMPDIR is normally /tmp
// already, this is the path zellij would have chosen anyway.
//
// A ZELLIJ_SOCKET_DIR already present in the environment is the user's own
// choice and is left alone.
func socketDir() string {
	if dir := os.Getenv("ZELLIJ_SOCKET_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("/tmp", "zellij-"+strconv.Itoa(os.Getuid()))
}

// controlTimeout bounds every short-lived zellij control command: listing,
// creating and killing sessions.
//
// A zellij server can wedge in a state where it still accepts connections on
// its session socket but never answers them, and a client talking to it then
// blocks forever. vibemux runs these commands from its status sweep every few
// seconds and synchronously from key handlers, so one dead server used to
// freeze the entire UI with no way out, not even ctrl+c. Bounding them turns
// that permanent freeze into a brief stall that the next sweep recovers from.
//
// Attaching is deliberately not bounded: an attached session is long-lived by
// nature and its command is handed to bubbletea, not run here.
// It is a var so tests can shrink it.
var controlTimeout = 5 * time.Second

// killGrace is how long a timed-out command may take to release its output
// pipes after the context kills it. Without a WaitDelay, Run and Output wait
// on the pipes rather than on the process, so a grandchild that inherited them
// would reintroduce the very hang the timeout exists to prevent.
const killGrace = time.Second

// commandEnv is the environment every zellij invocation runs under. They all
// share one socket dir so that sessions created by vibemux are the same ones
// it lists, attaches to, and kills.
func commandEnv() []string {
	return append(os.Environ(), "ZELLIJ_SOCKET_DIR="+socketDir())
}

// command builds an unbounded *exec.Cmd for the resolved zellij binary. Only
// AttachCommand should use it; everything else goes through controlCommand.
func command(args ...string) *exec.Cmd {
	cmd := exec.Command(binaryPath(), args...)
	cmd.Env = commandEnv()
	return cmd
}

// controlCommand builds a zellij command bounded by controlTimeout. The
// returned cancel must be called once the command has finished.
func controlCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
	cmd := exec.CommandContext(ctx, binaryPath(), args...)
	cmd.Env = commandEnv()
	cmd.WaitDelay = killGrace
	return cmd, cancel
}

// run executes cmd and, on failure, wraps the exit error with whatever zellij
// wrote to stderr. zellij reports the real cause there (a too-long socket
// path, an unreadable config) while exiting with an otherwise opaque status 1.
func run(cmd *exec.Cmd) error {
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}

// IsInstalled checks whether zellij is available on PATH or in ~/.local/bin.
func (Backend) IsInstalled() bool {
	return binaryPath() != ""
}

// liveSessions returns the set of currently live session names. zellij has
// no has-session command and `list-sessions -n` mixes live and EXITED
// (dead but resurrectable) sessions indistinguishably, so this parses the
// output and drops EXITED lines.
func liveSessions() map[string]bool {
	cmd, cancel := controlCommand("list-sessions", "-n")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		// zellij exits non-zero when no sessions exist, and a timed-out command
		// also lands here; both mean "nothing usable to report", so treat as empty.
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
func (Backend) HasSession(name string) bool {
	return liveSessions()[name]
}

// NewSession creates a new detached zellij session with the given name and
// working directory. The working directory is set both on the process (zellij
// has no --cwd flag for session creation) and via the options subcommand so
// panes opened later in the session also start there.
//
// Two steps give exited sessions tmux-like "gone means gone" semantics:
//
//   - session_serialization is turned off via a generated --config file.
//     zellij otherwise serializes sessions to disk and resurrects them as
//     EXITED corpses after they end. The setting MUST travel in a config file:
//     passing --session-serialization to the `options` subcommand at creation
//     is silently ignored (verified on 0.44.3), the same quirk that affects
//     web_sharing.
//   - any existing serialized corpse for this name is deleted first, so a
//     fresh session is created instead of resurrecting the old one with its
//     old tabs. NewSession is only called when no live session of this name
//     exists, so this never tears down a running session.
func (Backend) NewSession(name, dir string) error {
	cfg, err := noSerializeConfig(name)
	if err != nil {
		return err
	}
	deleteCmd, cancelDelete := controlCommand("delete-session", "--force", name)
	_ = deleteCmd.Run()
	cancelDelete()

	cmd, cancel := controlCommand("--config", cfg, "attach", "--create-background", name, "options", "--default-cwd", dir)
	defer cancel()
	cmd.Dir = dir
	return run(cmd)
}

// effectiveConfigPath returns the config file zellij would normally load, as
// reported by `zellij setup --check` (which knows the full resolution order,
// including the quirk that an existing ~/.config/zellij wins over
// XDG_CONFIG_HOME). Returns "" when it cannot be determined; the reported file
// may not exist.
func effectiveConfigPath() string {
	cmd, cancel := controlCommand("setup", "--check")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	const marker = "[LOOKING FOR CONFIG FILE FROM]:"
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), marker); ok {
			return strings.Trim(strings.TrimSpace(rest), "\"")
		}
	}
	return ""
}

// noSerializeConfig writes a config file that turns session_serialization off
// while preserving the user's own config, and returns its path. Passing
// --config replaces zellij's normal config resolution, so the user's settings
// are copied in; the override is prepended because zellij keeps the first
// occurrence of a duplicated option, so it wins over any session_serialization
// in the user config. The file lives in the temp dir for the session's
// lifetime: the zellij server re-reads it after creation, so it must outlive
// the creating client.
func noSerializeConfig(name string) (string, error) {
	content := "session_serialization false\n"
	if p := effectiveConfigPath(); p != "" {
		if user, err := os.ReadFile(p); err == nil {
			content += string(user)
		}
	}
	path := filepath.Join(os.TempDir(), "vmx-noserialize-"+name+".kdl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("writing session config: %w", err)
	}
	return path, nil
}

// AttachCommand returns an *exec.Cmd that attaches to the named zellij
// session. Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors
// so that bubbletea's ExecProcess won't override them with its wrapped
// readers/writers, which zellij cannot use (it needs a real /dev/tty).
func (Backend) AttachCommand(name string) *exec.Cmd {
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
func (Backend) KillSession(name string) error {
	killCmd, cancelKill := controlCommand("kill-session", name)
	err := run(killCmd)
	cancelKill()

	deleteCmd, cancelDelete := controlCommand("delete-session", "--force", name)
	_ = deleteCmd.Run()
	cancelDelete()

	return err
}

// ListVibemuxSessions returns a set of live zellij session names that have
// the vibemux prefix. Returns an empty map when no sessions exist.
func (Backend) ListVibemuxSessions() (map[string]bool, error) {
	sessions := map[string]bool{}
	for name := range liveSessions() {
		if strings.HasPrefix(name, sessionPrefix) {
			sessions[name] = true
		}
	}
	return sessions, nil
}
