// Package mux defines the terminal multiplexer abstraction vibemux drives and
// the registry that maps a persisted Kind to a concrete backend.
package mux

import "os/exec"

// Kind is the persisted identity of a multiplexer backend.
type Kind string

const (
	Tmux   Kind = "tmux"
	Zellij Kind = "zellij"
)

// Multiplexer is the terminal multiplexer vibemux uses to host project
// sessions. tmux and zellij each provide an implementation.
type Multiplexer interface {
	// Name is the human-readable backend name, e.g. "tmux".
	Name() string
	// IsInstalled reports whether the backend's binary is available.
	IsInstalled() bool
	// SessionName returns the deterministic vmx- session name for a project.
	SessionName(projectPath string) string
	// HasSession reports whether a live session with the name exists.
	HasSession(name string) bool
	// NewSession creates a detached session with the name and working dir.
	NewSession(name, dir string) error
	// AttachCommand returns the command that hands the terminal to a session.
	AttachCommand(name string) *exec.Cmd
	// KillSession destroys the named session.
	KillSession(name string) error
	// ListVibemuxSessions returns the set of live vmx- session names.
	ListVibemuxSessions() (map[string]bool, error)
}

// All lists every multiplexer vibemux knows about, in display order.
func All() []Kind { return []Kind{Tmux, Zellij} }

// Parse validates a persisted string and returns its Kind.
func Parse(s string) (Kind, bool) {
	switch Kind(s) {
	case Tmux, Zellij:
		return Kind(s), true
	}
	return "", false
}

// Active resolves the multiplexer to use from a persisted value and the set
// of currently-installed kinds. It returns false when the saved value is
// empty, unrecognized, or no longer installed: the caller then onboards.
func Active(saved string, installed []Kind) (Kind, bool) {
	k, ok := Parse(saved)
	if !ok {
		return "", false
	}
	for _, ik := range installed {
		if ik == k {
			return k, true
		}
	}
	return "", false
}
