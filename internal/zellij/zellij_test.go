package zellij

import (
	"strings"
	"testing"
	"time"
)

// isolate points zellij at throwaway socket and cache directories so the
// tests never touch (or see) the user's real sessions.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("ZELLIJ_SOCKET_DIR", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// waitFor polls cond for up to 5 seconds. Session creation and layout
// application happen in the zellij server, so observable state can lag the
// CLI call slightly.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cond()
}

// TestHasSessionExactMatch verifies HasSession only matches exact session
// names, not prefixes. Mirrors the tmux regression test: a lookup for
// vmx-pfxtest-XYZ must not match vmx-pfxtest-XYZ-long.
func TestHasSessionExactMatch(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	const (
		longName  = "vmx-pfxtest-XYZ-long"
		shortName = "vmx-pfxtest-XYZ"
	)

	if err := NewSession(longName, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		_ = KillSession(longName)
	})

	if !waitFor(func() bool { return HasSession(longName) }) {
		t.Fatalf("HasSession(%q) = false, want true (session was just created)", longName)
	}

	if HasSession(shortName) {
		t.Errorf("HasSession(%q) = true, want false: only %q exists, prefix match leaked through",
			shortName, longName)
	}
}

// TestKillSessionLeavesNoCorpse verifies KillSession removes the session
// entirely. zellij kill-session alone leaves an EXITED resurrectable corpse
// that would make HasSession/ListVibemuxSessions semantics diverge from tmux
// if the EXITED filter or the delete-session follow-up regressed.
func TestKillSessionLeavesNoCorpse(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	const name = "vmx-killtest-XYZ"
	if err := NewSession(name, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	if !waitFor(func() bool { return HasSession(name) }) {
		t.Fatalf("session %q not live after NewSession", name)
	}

	if err := KillSession(name); err != nil {
		t.Fatalf("KillSession(%q) error: %v", name, err)
	}
	if !waitFor(func() bool { return !HasSession(name) }) {
		t.Errorf("HasSession(%q) = true after KillSession, want false", name)
	}

	sessions, err := ListVibemuxSessions()
	if err != nil {
		t.Fatalf("ListVibemuxSessions() error: %v", err)
	}
	if sessions[name] {
		t.Errorf("ListVibemuxSessions still contains %q after KillSession", name)
	}
}

// TestListVibemuxSessionsEmpty verifies the no-sessions case returns an empty
// map instead of an error (zellij exits non-zero when no sessions exist).
func TestListVibemuxSessionsEmpty(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	sessions, err := ListVibemuxSessions()
	if err != nil {
		t.Fatalf("ListVibemuxSessions() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListVibemuxSessions() = %v, want empty", sessions)
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

// TestDashboardLayout verifies the generated KDL: one mirror pane per
// session, grid rows of ceil(sqrt(n)) columns filled breadth-first.
func TestDashboardLayout(t *testing.T) {
	got := dashboardLayout("/usr/bin/zellij", []string{"vmx-a", "vmx-b", "vmx-c"})

	if rows := strings.Count(got, `split_direction="vertical"`); rows != 2 {
		t.Errorf("layout has %d rows, want 2 (3 sessions, 2 columns):\n%s", rows, got)
	}
	for _, name := range []string{"vmx-a", "vmx-b", "vmx-c"} {
		if !strings.Contains(got, `name="`+name+`"`) {
			t.Errorf("layout missing pane name for %q:\n%s", name, got)
		}
		if !strings.Contains(got, `--session '`+name+`' action dump-screen`) {
			t.Errorf("layout missing dump-screen mirror for %q:\n%s", name, got)
		}
	}
	if !strings.Contains(got, "'/usr/bin/zellij' --session") {
		t.Errorf("layout missing zellij binary path in mirror script:\n%s", got)
	}
	if !strings.Contains(got, `command="bash"`) {
		t.Errorf("layout panes should run bash mirror scripts:\n%s", got)
	}
}

// typeMarker types an echo command into a session's main shell pane and
// waits until its output is visible in that pane, so the dashboard mirror
// has distinctive content to pick up.
func typeMarker(t *testing.T, session, marker string) {
	t.Helper()
	// Keystrokes sent before the pane's shell has rendered its prompt are
	// dropped silently (verified empirically), so wait for a non-empty
	// viewport first.
	if !waitFor(func() bool {
		out, err := command("--session", session, "action", "dump-screen",
			"--pane-id", "terminal_0").Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}) {
		t.Fatalf("pane terminal_0 of %q never rendered a prompt", session)
	}
	if err := command("--session", session, "action", "write-chars",
		"--pane-id", "terminal_0", "echo "+marker).Run(); err != nil {
		t.Fatalf("write-chars to %q failed: %v", session, err)
	}
	// Enter is sent as a raw byte; write-chars has no escape syntax for it.
	if err := command("--session", session, "action", "write",
		"--pane-id", "terminal_0", "13").Run(); err != nil {
		t.Fatalf("write Enter to %q failed: %v", session, err)
	}
	if !waitFor(func() bool {
		out, err := command("--session", session, "action", "dump-screen",
			"--pane-id", "terminal_0").Output()
		return err == nil && strings.Contains(string(out), marker)
	}) {
		t.Fatalf("marker %q never appeared in session %q", marker, session)
	}
}

// dumpPane returns the current viewport of one dashboard pane.
func dumpPane(paneID string) string {
	out, _ := command("--session", DashboardSession, "action", "dump-screen",
		"--pane-id", paneID).Output()
	return string(out)
}

// TestBuildDashboard exercises the dashboard build against a real zellij
// server: two source sessions with distinctive content must produce a live
// vmx-dashboard whose panes each display their own session's content. This
// is the assertion the old nested-attach dashboard could never pass (all
// inner clients rendered into the first pane).
func TestBuildDashboard(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	const (
		s1      = "vmx-dashtest-one"
		s2      = "vmx-dashtest-two"
		marker1 = "VMX_MARK_ALPHA"
		marker2 = "VMX_MARK_BETA"
	)
	t.Cleanup(func() {
		for _, n := range []string{DashboardSession, s1, s2} {
			_ = KillSession(n)
		}
	})

	if err := NewSession(s1, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s1, err)
	}
	if err := NewSession(s2, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s2, err)
	}
	typeMarker(t, s1, marker1)
	typeMarker(t, s2, marker2)

	if err := BuildDashboard([]string{s1, s2}); err != nil {
		t.Fatalf("BuildDashboard() error: %v", err)
	}
	if !waitFor(func() bool { return HasSession(DashboardSession) }) {
		t.Fatal("dashboard session does not exist after BuildDashboard")
	}

	// Dashboard panes are created in layout order, so terminal_0 mirrors s1
	// and terminal_1 mirrors s2. Each mirror polls once a second; waitFor
	// absorbs the first-paint delay.
	if !waitFor(func() bool { return strings.Contains(dumpPane("terminal_0"), marker1) }) {
		t.Errorf("dashboard pane terminal_0 never showed %s content:\n%s", s1, dumpPane("terminal_0"))
	}
	if !waitFor(func() bool { return strings.Contains(dumpPane("terminal_1"), marker2) }) {
		t.Errorf("dashboard pane terminal_1 never showed %s content:\n%s", s2, dumpPane("terminal_1"))
	}
}
