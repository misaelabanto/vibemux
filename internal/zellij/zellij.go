package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sessionPrefix = "vmx-"

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

// command builds an *exec.Cmd for the resolved zellij binary.
func command(args ...string) *exec.Cmd {
	return exec.Command(binaryPath(), args...)
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
	_ = command("delete-session", "--force", name).Run()
	cmd := command("--config", cfg, "attach", "--create-background", name, "options", "--default-cwd", dir)
	cmd.Dir = dir
	return cmd.Run()
}

// effectiveConfigPath returns the config file zellij would normally load, as
// reported by `zellij setup --check` (which knows the full resolution order,
// including the quirk that an existing ~/.config/zellij wins over
// XDG_CONFIG_HOME). Returns "" when it cannot be determined; the reported file
// may not exist.
func effectiveConfigPath() string {
	out, err := command("setup", "--check").Output()
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
	err := command("kill-session", name).Run()
	_ = command("delete-session", "--force", name).Run()
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
