package zellij

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubZellij puts a fake zellij binary that never exits first on PATH, standing
// in for a wedged zellij server: one that still accepts the client's connection
// but never answers it.
func stubZellij(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	script := filepath.Join(dir, "zellij")
	// /bin/sleep by absolute path: PATH is replaced below with the stub's own
	// directory, and the child inherits it.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec /bin/sleep 60\n"), 0o755); err != nil {
		t.Fatalf("writing stub zellij: %v", err)
	}
	t.Setenv("PATH", dir)
}

// A wedged zellij server used to hang liveSessions forever, which froze the
// whole dashboard: the status sweep never returned, so every later sweep
// joined the dead one, and the key handlers that call HasSession blocked the
// UI goroutine with no way out, not even ctrl+c.
func TestLiveSessionsGivesUpOnWedgedServer(t *testing.T) {
	stubZellij(t)

	controlTimeout = 200 * time.Millisecond
	t.Cleanup(func() { controlTimeout = 5 * time.Second })

	done := make(chan map[string]bool, 1)
	start := time.Now()
	go func() { done <- liveSessions() }()

	select {
	case sessions := <-done:
		if len(sessions) != 0 {
			t.Errorf("liveSessions() = %v, want empty on a wedged server", sessions)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("liveSessions() took %v, want it bounded by controlTimeout", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("liveSessions() never returned against a wedged server")
	}
}
