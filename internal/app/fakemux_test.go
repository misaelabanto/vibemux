package app

import (
	"os/exec"
	"strings"
)

// fakeMux is a mux.Multiplexer that records what it was asked to do instead of
// shelling out. Tests assert on the recorded calls, which is how "the
// pre-check returned before touching the multiplexer" becomes observable.
type fakeMux struct {
	installed      bool
	sessions       map[string]bool
	newSessionErr  error
	killSessionErr error

	newSessionCalls  []string
	killSessionCalls []string
}

func newFakeMux() *fakeMux {
	return &fakeMux{installed: true, sessions: map[string]bool{}}
}

func (f *fakeMux) Name() string      { return "fakemux" }
func (f *fakeMux) IsInstalled() bool { return f.installed }

func (f *fakeMux) SessionName(projectPath string) string {
	trimmed := strings.TrimSuffix(projectPath, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	return "vmx-" + trimmed
}

func (f *fakeMux) HasSession(name string) bool { return f.sessions[name] }

func (f *fakeMux) NewSession(name, dir string) error {
	f.newSessionCalls = append(f.newSessionCalls, name)
	if f.newSessionErr != nil {
		return f.newSessionErr
	}
	f.sessions[name] = true
	return nil
}

func (f *fakeMux) AttachCommand(name string) *exec.Cmd { return exec.Command("true") }

func (f *fakeMux) KillSession(name string) error {
	f.killSessionCalls = append(f.killSessionCalls, name)
	if f.killSessionErr != nil {
		return f.killSessionErr
	}
	delete(f.sessions, name)
	return nil
}

func (f *fakeMux) ListVibemuxSessions() (map[string]bool, error) {
	live := make(map[string]bool, len(f.sessions))
	for name, ok := range f.sessions {
		live[name] = ok
	}
	return live, nil
}
