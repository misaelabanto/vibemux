package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const sessionPrefix = "vmx-"

// Backend drives tmux. It has no state; methods shell out to the tmux binary.
type Backend struct{}

// Name is the multiplexer's persisted, human-readable identity.
func (Backend) Name() string { return "tmux" }

// SessionName returns a deterministic tmux session name derived from the
// base directory name of the project path (e.g. "vmx-myproject").
func (Backend) SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

// IsInstalled checks whether tmux is available on PATH.
func (Backend) IsInstalled() bool {
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

// controlTimeout bounds every short-lived tmux control command: probing,
// creating, listing and killing sessions.
//
// A multiplexer server that has wedged still holds its socket open but never
// answers, and a client talking to it then blocks forever. vibemux runs these
// commands from its status sweep every few seconds and synchronously from key
// handlers, so one dead server would freeze the entire UI with no way out.
// Bounding them turns that permanent freeze into a brief stall.
//
// Attaching is deliberately not bounded: an attached session is long-lived by
// nature and its command is handed to bubbletea, not run here.
// It is a var so tests can shrink it.
var controlTimeout = 5 * time.Second

// killGrace is how long a timed-out command may take to release its output
// pipes after the context kills it. Without a WaitDelay, Run and Output wait on
// the pipes rather than on the process, so a grandchild that inherited them
// would reintroduce the very hang the timeout exists to prevent.
const killGrace = time.Second

// controlCommand builds a tmux command bounded by controlTimeout. The returned
// cancel must be called once the command has finished.
func controlCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), controlTimeout)
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.WaitDelay = killGrace
	return cmd, cancel
}

// run executes cmd and folds any stderr it produced into the returned error.
// Bare exec errors are just "exit status 1", which is useless in a toast, so
// the child's own diagnostic is carried out to the caller.
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

// HasSession checks whether a tmux session with the given name exists.
func (Backend) HasSession(name string) bool {
	cmd, cancel := controlCommand("has-session", "-t", exactTarget(name))
	defer cancel()
	return cmd.Run() == nil
}

// NewSession creates a new detached tmux session with the given name and
// working directory.
func (Backend) NewSession(name, dir string) error {
	cmd, cancel := controlCommand("new-session", "-d", "-s", name, "-c", dir)
	defer cancel()
	return run(cmd)
}

// AttachCommand returns an *exec.Cmd that attaches to the named tmux session.
// Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors so that
// bubbletea's ExecProcess won't override them with its wrapped readers/writers,
// which tmux cannot use (it needs a real /dev/tty).
func (Backend) AttachCommand(name string) *exec.Cmd {
	cmd := exec.Command("tmux", "attach-session", "-t", exactTarget(name))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// KillSession destroys the named tmux session.
func (Backend) KillSession(name string) error {
	cmd, cancel := controlCommand("kill-session", "-t", exactTarget(name))
	defer cancel()
	return run(cmd)
}

// ListVibemuxSessions returns a set of active tmux session names that have the
// vibemux prefix.
func (Backend) ListVibemuxSessions() (map[string]bool, error) {
	cmd, cancel := controlCommand("list-sessions", "-F", "#{session_name}")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		// tmux returns an error when no server is running, and a timed-out
		// command also lands here: both mean nothing usable, so treat as empty.
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
