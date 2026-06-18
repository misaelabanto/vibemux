package gitstatus

import (
	"context"
	"os/exec"
	"testing"
)

// skipIfNoGit skips the test if git is not available on PATH.
func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

// initRepo creates a new git repo in dir.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
}

func TestHasRemote_NoRemote(t *testing.T) {
	skipIfNoGit(t)

	dir := t.TempDir()
	initRepo(t, dir)

	if HasRemote(dir) {
		t.Error("expected HasRemote to return false for a fresh git init repo")
	}
}

func TestHasRemote_WithRemote(t *testing.T) {
	skipIfNoGit(t)

	dir := t.TempDir()
	initRepo(t, dir)

	cmd := exec.Command("git", "-C", dir, "remote", "add", "origin", "/nonexistent/path")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v\n%s", err, out)
	}

	if !HasRemote(dir) {
		t.Error("expected HasRemote to return true after adding a remote")
	}
}

func TestFetch_NoRemote_ReturnsNil(t *testing.T) {
	skipIfNoGit(t)

	dir := t.TempDir()
	initRepo(t, dir)

	ctx := context.Background()
	err := Fetch(ctx, dir)
	if err != nil {
		t.Errorf("expected Fetch to return nil for repo with no remote, got: %v", err)
	}
}

func TestFetchTimeout(t *testing.T) {
	d := FetchTimeout()
	const want = 15e9 // 15 * time.Second in nanoseconds
	if d.Nanoseconds() != want {
		t.Errorf("FetchTimeout() = %v, want 15s", d)
	}
}
