package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

// TestHasSessionExactMatch verifies HasSession only matches exact session
// names, not prefixes. Regression test: tmux's default `-t name` target
// supports prefix matching, which caused vibemux to think `vmx-agendalo`
// already existed when only `vmx-agendalo-app-nuxt` did, and to attach to
// the wrong session.
func TestHasSessionExactMatch(t *testing.T) {
	if !IsInstalled() {
		t.Skip("tmux not installed")
	}

	const (
		longName  = "vmx-pfxtest-XYZ-long"
		shortName = "vmx-pfxtest-XYZ"
	)

	// Ensure clean state.
	_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	_ = exec.Command("tmux", "kill-session", "-t", "="+shortName).Run()

	if err := NewSession(longName, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	})

	if !HasSession(longName) {
		t.Fatalf("HasSession(%q) = false, want true (session was just created)", longName)
	}

	if HasSession(shortName) {
		t.Errorf("HasSession(%q) = true, want false — only %q exists; tmux prefix match leaked through",
			shortName, longName)
	}
}

// TestNestedAttachCommand verifies the shell command a dashboard pane runs:
// TMUX must be cleared (tmux refuses to nest otherwise), exec must tie the
// pane lifetime to the inner client, and the target must use exact-name
// matching like every other lookup in this package.
func TestNestedAttachCommand(t *testing.T) {
	got := nestedAttachCommand("vmx-foo")
	want := "TMUX= exec tmux attach-session -t '=vmx-foo'"
	if got != want {
		t.Errorf("nestedAttachCommand(%q) = %q, want %q", "vmx-foo", got, want)
	}
}

// TestDashboardSessions verifies the dashboard never shows itself and that
// the result is sorted for stable pane order.
func TestDashboardSessions(t *testing.T) {
	active := map[string]bool{
		"vmx-beta":       true,
		DashboardSession: true,
		"vmx-alpha":      true,
	}
	got := DashboardSessions(active)
	want := []string{"vmx-alpha", "vmx-beta"}
	if len(got) != len(want) {
		t.Fatalf("DashboardSessions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DashboardSessions = %v, want %v", got, want)
		}
	}
}

// TestBuildDashboard exercises the dashboard build against a real tmux
// server: two source sessions must produce a vmx-dashboard with two panes.
// Note: this kills any existing vmx-dashboard, so don't run the test suite
// while using a real dashboard.
func TestBuildDashboard(t *testing.T) {
	if !IsInstalled() {
		t.Skip("tmux not installed")
	}

	const (
		s1 = "vmx-dashtest-one"
		s2 = "vmx-dashtest-two"
	)
	cleanup := func() {
		for _, n := range []string{s1, s2, DashboardSession} {
			_ = exec.Command("tmux", "kill-session", "-t", "="+n).Run()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := NewSession(s1, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s1, err)
	}
	if err := NewSession(s2, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s2, err)
	}

	if err := BuildDashboard([]string{s1, s2}); err != nil {
		t.Fatalf("BuildDashboard() error: %v", err)
	}
	if !HasSession(DashboardSession) {
		t.Fatal("dashboard session does not exist after BuildDashboard")
	}

	out, err := exec.Command("tmux", "list-panes", "-t", "="+DashboardSession, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}
	panes := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(panes) != 2 {
		t.Errorf("dashboard has %d panes, want 2", len(panes))
	}
}
