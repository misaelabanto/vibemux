package tmux

import (
	"os/exec"
	"testing"
)

// b is the backend under test; tmux methods have no per-instance state.
var b = Backend{}

// TestHasSessionExactMatch verifies HasSession only matches exact session
// names, not prefixes. Regression test: tmux's default `-t name` target
// supports prefix matching, which caused vibemux to think `vmx-agendalo`
// already existed when only `vmx-agendalo-app-nuxt` did, and to attach to
// the wrong session.
func TestHasSessionExactMatch(t *testing.T) {
	if !b.IsInstalled() {
		t.Skip("tmux not installed")
	}

	const (
		longName  = "vmx-pfxtest-XYZ-long"
		shortName = "vmx-pfxtest-XYZ"
	)

	// Ensure clean state.
	_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	_ = exec.Command("tmux", "kill-session", "-t", "="+shortName).Run()

	if err := b.NewSession(longName, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	})

	if !b.HasSession(longName) {
		t.Fatalf("HasSession(%q) = false, want true (session was just created)", longName)
	}

	if b.HasSession(shortName) {
		t.Errorf("HasSession(%q) = true, want false: only %q exists; tmux prefix match leaked through",
			shortName, longName)
	}
}

// TestName verifies the backend reports its persisted/display name.
func TestName(t *testing.T) {
	if got := b.Name(); got != "tmux" {
		t.Errorf("Name() = %q, want %q", got, "tmux")
	}
}

// TestSessionName verifies the deterministic vmx- name derived from a path.
func TestSessionName(t *testing.T) {
	if got := b.SessionName("/home/u/code/myproject"); got != "vmx-myproject" {
		t.Errorf("SessionName() = %q, want %q", got, "vmx-myproject")
	}
}
