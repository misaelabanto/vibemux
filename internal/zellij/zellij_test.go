package zellij

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// b is the backend under test; zellij methods have no per-instance state.
var b = Backend{}

// isolate points zellij at throwaway socket and cache directories so the
// tests never touch (or see) the user's real sessions.
//
// The socket dir is anchored at /tmp rather than t.TempDir() because the
// latter nests the test name under $TMPDIR, and zellij's socket path
// (<socket dir>/contract_version_1/<session name>) must stay under the 103-byte
// unix socket limit. On macOS that combination overflows on its own.
func isolate(t *testing.T) {
	t.Helper()
	sockets, err := os.MkdirTemp("/tmp", "vmxtest")
	if err != nil {
		t.Fatalf("creating socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockets) })
	t.Setenv("ZELLIJ_SOCKET_DIR", sockets)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// waitFor polls cond for up to 5 seconds. Session creation happens in the
// zellij server, so observable state can lag the CLI call slightly.
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

// TestName verifies the backend reports its persisted/display name.
func TestName(t *testing.T) {
	if got := b.Name(); got != "zellij" {
		t.Errorf("Name() = %q, want %q", got, "zellij")
	}
}

// TestSessionName verifies the deterministic vmx- name derived from a path.
func TestSessionName(t *testing.T) {
	if got := b.SessionName("/home/u/code/myproject"); got != "vmx-myproject" {
		t.Errorf("SessionName() = %q, want %q", got, "vmx-myproject")
	}
}

// TestSocketDirFitsLongProjectNames verifies the default socket dir leaves
// room for a realistically long project name. zellij's socket path is
// <socket dir>/contract_version_1/<session name> and cannot exceed
// maxSocketPath bytes; under macOS's per-user $TMPDIR the platform default
// left only ~24 bytes for the name, so opening a project like
// "agendalo-nuxt-frontend" failed with a bare exit status 1.
func TestSocketDirFitsLongProjectNames(t *testing.T) {
	t.Setenv("ZELLIJ_SOCKET_DIR", "")

	name := b.SessionName("/Users/someone/code/agendalo/agendalo-nuxt-frontend")
	path := filepath.Join(socketDir(), "contract_version_1", name)
	if len(path) > maxSocketPath {
		t.Errorf("socket path for %q is %d bytes, want at most %d: %s",
			name, len(path), maxSocketPath, path)
	}
}

// TestSocketDirRespectsEnv verifies a user-set ZELLIJ_SOCKET_DIR wins over
// vibemux's default, which is also what keeps the tests isolated.
func TestSocketDirRespectsEnv(t *testing.T) {
	t.Setenv("ZELLIJ_SOCKET_DIR", "/tmp/mine")

	if got := socketDir(); got != "/tmp/mine" {
		t.Errorf("socketDir() = %q, want %q", got, "/tmp/mine")
	}
}

// TestHasSessionExactMatch verifies HasSession only matches exact session
// names, not prefixes: a lookup for vmx-pfxtest-XYZ must not match
// vmx-pfxtest-XYZ-long.
func TestHasSessionExactMatch(t *testing.T) {
	if !b.IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	const (
		longName  = "vmx-pfxtest-XYZ-long"
		shortName = "vmx-pfxtest-XYZ"
	)

	if err := b.NewSession(longName, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		_ = b.KillSession(longName)
	})

	if !waitFor(func() bool { return b.HasSession(longName) }) {
		t.Fatalf("HasSession(%q) = false, want true (session was just created)", longName)
	}

	if b.HasSession(shortName) {
		t.Errorf("HasSession(%q) = true, want false: only %q exists, prefix match leaked through",
			shortName, longName)
	}
}

// TestKillSessionLeavesNoCorpse verifies KillSession removes the session
// entirely. zellij kill-session alone leaves an EXITED resurrectable corpse
// that would make HasSession/ListVibemuxSessions semantics diverge from tmux
// if the EXITED filter or the delete-session follow-up regressed.
func TestKillSessionLeavesNoCorpse(t *testing.T) {
	if !b.IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	const name = "vmx-killtest-XYZ"
	if err := b.NewSession(name, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	if !waitFor(func() bool { return b.HasSession(name) }) {
		t.Fatalf("session %q not live after NewSession", name)
	}

	if err := b.KillSession(name); err != nil {
		t.Fatalf("KillSession(%q) error: %v", name, err)
	}
	if !waitFor(func() bool { return !b.HasSession(name) }) {
		t.Errorf("HasSession(%q) = true after KillSession, want false", name)
	}

	sessions, err := b.ListVibemuxSessions()
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
	if !b.IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolate(t)

	sessions, err := b.ListVibemuxSessions()
	if err != nil {
		t.Fatalf("ListVibemuxSessions() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListVibemuxSessions() = %v, want empty", sessions)
	}
}
