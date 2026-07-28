package gitstatus_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

// countingGit puts a shim named "git" at the front of PATH that appends a line
// to a counter file on every invocation before delegating to the real git. It
// returns a func reporting how many times git was spawned.
func countingGit(t *testing.T) func() int {
	t.Helper()

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}

	shimDir := t.TempDir()
	counter := filepath.Join(shimDir, "count")
	script := fmt.Sprintf("#!/bin/sh\necho x >> %q\nexec %q \"$@\"\n", counter, realGit)
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}

	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return func() int {
		data, err := os.ReadFile(counter)
		if err != nil {
			return 0
		}
		count := 0
		for _, b := range data {
			if b == '\n' {
				count++
			}
		}
		return count
	}
}

// TestComputeSpawnsOneProcess pins the cost of Compute at a single git
// invocation. Process spawn dominates the status sweep (on macOS a bare git
// spawn costs ~6ms), and the sweep runs for every project on every refresh, so
// an extra spawn per project is a per-refresh regression measured in hundreds
// of milliseconds.
func TestComputeSpawnsOneProcess(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	dir := initRepo(t)

	count := countingGit(t)
	gitstatus.Compute(dir)

	if got := count(); got != 1 {
		t.Errorf("Compute spawned git %d times, want 1", got)
	}
}

// TestComputeAllSpawnsOnePerPath pins the same contract for the batch sweep.
func TestComputeAllSpawnsOnePerPath(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}
	paths := []string{initRepo(t), initRepo(t), initRepo(t)}

	count := countingGit(t)
	gitstatus.ComputeAll(paths)

	if got := count(); got != len(paths) {
		t.Errorf("ComputeAll spawned git %d times for %d paths, want %d", got, len(paths), len(paths))
	}
}

// TestComputeAllMatchesCompute verifies the batch sweep returns, for every
// path, exactly what Compute returns for that path on its own.
func TestComputeAllMatchesCompute(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not on PATH")
	}

	clean := initRepo(t)

	untracked := initRepo(t)
	if err := os.WriteFile(filepath.Join(untracked, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	modified := initRepo(t)
	if err := os.WriteFile(filepath.Join(modified, "README"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nonRepo := t.TempDir()

	paths := []string{clean, untracked, modified, nonRepo}
	got := gitstatus.ComputeAll(paths)

	if len(got) != len(paths) {
		t.Fatalf("ComputeAll returned %d entries, want %d", len(got), len(paths))
	}
	for _, path := range paths {
		want := gitstatus.Compute(path)
		if got[path] != want {
			t.Errorf("ComputeAll[%s] = %+v, want %+v", path, got[path], want)
		}
	}
}

// TestComputeAllEmpty guards the degenerate input.
func TestComputeAllEmpty(t *testing.T) {
	if got := gitstatus.ComputeAll(nil); len(got) != 0 {
		t.Errorf("ComputeAll(nil) = %v, want empty", got)
	}
}
