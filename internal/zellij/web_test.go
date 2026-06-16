package zellij

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateWeb extends isolate with the directories the web server touches:
// tokens.db lives in XDG_DATA_HOME, and config resolution (XDG_CONFIG_HOME
// plus ZELLIJ_CONFIG_DIR) must not see the user's real config.
func isolateWeb(t *testing.T) {
	t.Helper()
	isolate(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("ZELLIJ_CONFIG_DIR", filepath.Join(cfgHome, "zellij"))
}

// freePort grabs a port that is currently free by binding a listener on :0
// and closing it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// stopWebServer stops the isolated web server and verifies it is gone.
// `zellij web --stop` rejects --port (the flag groups are mutually
// exclusive), but it targets the server of the current environment, which the
// isolation confines to this test.
func stopWebServer(t *testing.T, port int) {
	t.Helper()
	_ = command("web", "--stop").Run()
	if !waitFor(func() bool { return !WebServerRunning(port) }) {
		t.Errorf("web server on port %d still running after stop", port)
	}
}

// TestWebServerRunningDeadPort verifies WebServerRunning is false for a port
// nothing listens on. `zellij web --status` exits zero either way, so this
// guards the output parsing.
func TestWebServerRunningDeadPort(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolateWeb(t)

	if port := freePort(t); WebServerRunning(port) {
		t.Errorf("WebServerRunning(%d) = true, want false (nothing listens there)", port)
	}
}

// TestStartWebServer starts the web server, sees it report online, verifies
// a second start is a no-op, and stops it again.
func TestStartWebServer(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolateWeb(t)

	port := freePort(t)
	t.Cleanup(func() { stopWebServer(t, port) })

	if err := StartWebServer(port); err != nil {
		t.Fatalf("StartWebServer(%d) error: %v", port, err)
	}
	if !waitFor(func() bool { return WebServerRunning(port) }) {
		t.Fatalf("WebServerRunning(%d) = false after StartWebServer", port)
	}
	if err := StartWebServer(port); err != nil {
		t.Errorf("StartWebServer(%d) second call error: %v, want nil (idempotent)", port, err)
	}
}

// TestEnsureWebToken verifies that with a fresh tokens.db the first call
// creates a token and returns its plaintext, and the second call reports the
// existing token without creating another.
func TestEnsureWebToken(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolateWeb(t)

	token, created, err := EnsureWebToken()
	if err != nil {
		t.Fatalf("EnsureWebToken() first call error: %v", err)
	}
	if !created {
		t.Errorf("EnsureWebToken() created = false, want true (tokens.db was empty)")
	}
	if token == "" {
		t.Errorf("EnsureWebToken() token = %q, want the freshly created token", token)
	}

	token, created, err = EnsureWebToken()
	if err != nil {
		t.Fatalf("EnsureWebToken() second call error: %v", err)
	}
	if created {
		t.Errorf("EnsureWebToken() created = true on second call, want false")
	}
	if token != "" {
		t.Errorf("EnsureWebToken() token = %q on second call, want empty (plaintext is gone)", token)
	}
}

// TestSessionURL verifies the URL scheme the zellij web client uses,
// including escaping of names that are not path-safe.
func TestSessionURL(t *testing.T) {
	if got, want := SessionURL(8082, "vmx-myproject"), "http://127.0.0.1:8082/vmx-myproject"; got != want {
		t.Errorf("SessionURL = %q, want %q", got, want)
	}
	if got, want := SessionURL(9000, "vmx-a b"), "http://127.0.0.1:9000/vmx-a%20b"; got != want {
		t.Errorf("SessionURL = %q, want %q", got, want)
	}
}

// TestNewSessionWebShared verifies NewSession still creates a live session
// and that the session opted in to web sharing. The web_clients_allowed
// field in the session metadata is the cheap positive signal: it was
// verified end to end during development that sessions with
// web_clients_allowed true stream their screen to a web client while
// sessions without it are refused with "Web Clients are not allowed to
// attach to this session".
func TestNewSessionWebShared(t *testing.T) {
	if !IsInstalled() {
		t.Skip("zellij not installed")
	}
	isolateWeb(t)

	const name = "vmx-websharetest-XYZ"
	if err := NewSession(name, "/tmp"); err != nil {
		t.Fatalf("NewSession(%q) error: %v", name, err)
	}
	t.Cleanup(func() {
		_ = KillSession(name)
	})

	if !waitFor(func() bool { return HasSession(name) }) {
		t.Fatalf("HasSession(%q) = false after NewSession", name)
	}

	meta := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "zellij",
		"contract_version_1", "session_info", name, "session-metadata.kdl")
	if !waitFor(func() bool {
		b, err := os.ReadFile(meta)
		return err == nil && strings.Contains(string(b), "web_clients_allowed true")
	}) {
		b, _ := os.ReadFile(meta)
		t.Errorf("session %q is not web shared: metadata lacks web_clients_allowed true:\n%s", name, b)
	}
}

// TestBrowserCommandPrefersXdgOpen verifies xdg-open is used when present on
// PATH.
func TestBrowserCommandPrefersXdgOpen(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing fake xdg-open: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("BROWSER", "/bin/false")

	cmd, err := browserCommand("http://127.0.0.1:8082/vmx-x")
	if err != nil {
		t.Fatalf("browserCommand() error: %v", err)
	}
	if cmd.Path != fake {
		t.Errorf("browserCommand() path = %q, want %q", cmd.Path, fake)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "http://127.0.0.1:8082/vmx-x" {
		t.Errorf("browserCommand() args = %v, want [xdg-open url]", cmd.Args)
	}
}

// TestBrowserCommandFallsBackToBrowserEnv verifies the $BROWSER fallback when
// xdg-open is missing from PATH.
func TestBrowserCommandFallsBackToBrowserEnv(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("BROWSER", "/bin/echo")

	cmd, err := browserCommand("http://127.0.0.1:8082/vmx-x")
	if err != nil {
		t.Fatalf("browserCommand() error: %v", err)
	}
	if cmd.Path != "/bin/echo" {
		t.Errorf("browserCommand() path = %q, want /bin/echo", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "http://127.0.0.1:8082/vmx-x" {
		t.Errorf("browserCommand() args = %v, want [/bin/echo url]", cmd.Args)
	}
}

// TestBrowserCommandNoOpener verifies a clear error when neither xdg-open nor
// $BROWSER is available.
func TestBrowserCommandNoOpener(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("BROWSER", "")

	if _, err := browserCommand("http://127.0.0.1:8082/vmx-x"); err == nil {
		t.Error("browserCommand() error = nil, want error when no opener exists")
	}
}
