// Package gitstatus computes the git working-tree status for a given directory.
package gitstatus

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status holds the git working-tree status for a project directory.
type Status struct {
	IsRepo      bool
	Clean       bool
	Modified    bool
	Staged      bool
	Untracked   bool
	Stashed     bool
	Conflict    bool
	Ahead       int
	Behind      int
	HasUpstream bool
}

// gitEnv returns the current environment with GIT_OPTIONAL_LOCKS=0 appended.
func gitEnv() []string {
	return append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
}

// runGit runs git with the given arguments under the given directory,
// using a 5-second timeout. It returns stdout and whether the command succeeded.
func runGit(dir string, args ...string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// Stderr is intentionally ignored.

	err := cmd.Run()
	return stdout.Bytes(), err == nil
}

// Compute inspects path and returns its git status.
// If path is not inside a git work-tree, the returned Status has IsRepo=false
// and all other fields at their zero values.
//
// Everything is derived from a single git invocation. Spawning a process is the
// dominant cost here (a bare git spawn costs roughly 6 ms on macOS against
// roughly 1 ms on Linux), and Compute runs once per project on every refresh,
// so each extra invocation is multiplied by the project count. "status" alone
// covers all of it: it exits non-zero outside a work-tree, so no separate
// rev-parse probe is needed, and --show-stash adds the stash count to the
// porcelain v2 header instead of a second rev-parse.
func Compute(path string) Status {
	out, ok := runGit(path, "status", "--porcelain=v2", "--branch", "--show-stash")
	if !ok {
		return Status{IsRepo: false}
	}
	return parseStatus(out)
}

// parseStatus turns "git status --porcelain=v2 --branch --show-stash" output
// into a Status. The caller has already established that path is a work-tree.
func parseStatus(out []byte) Status {
	s := Status{IsRepo: true}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "# branch.ab "):
			// Format: "# branch.ab +A -B"
			s.HasUpstream = true
			parts := strings.Fields(line) // ["#", "branch.ab", "+A", "-B"]
			if len(parts) == 4 {
				aStr := strings.TrimPrefix(parts[2], "+")
				bStr := strings.TrimPrefix(parts[3], "-")
				if a, err := strconv.Atoi(aStr); err == nil {
					s.Ahead = a
				}
				if b, err := strconv.Atoi(bStr); err == nil {
					s.Behind = b
				}
			}

		case strings.HasPrefix(line, "# stash "):
			// Format: "# stash N", emitted only when at least one entry exists.
			parts := strings.Fields(line) // ["#", "stash", "N"]
			if len(parts) == 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
					s.Stashed = true
				}
			}

		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 "):
			// Changed tracked entry. Field index 1 (0-based after split) is XY.
			// Format: "1 XY sub mH mI mW hH hI path"
			//         "2 XY sub mH mI mW hH hI X score path\torigPath"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				xy := parts[1]
				if len(xy) >= 2 {
					x := xy[0]
					y := xy[1]
					// Staged if X is one of M A D R C (not '.')
					if x == 'M' || x == 'A' || x == 'D' || x == 'R' || x == 'C' {
						s.Staged = true
					}
					// Modified if Y is one of M D (not '.')
					if y == 'M' || y == 'D' {
						s.Modified = true
					}
				}
			}

		case strings.HasPrefix(line, "u "):
			s.Conflict = true

		case strings.HasPrefix(line, "? "):
			s.Untracked = true
		}
	}

	// Clean if repo has none of the dirty indicators.
	s.Clean = !s.Modified && !s.Staged && !s.Untracked && !s.Conflict && !s.Stashed

	return s
}

// maxParallelStatus caps how many git processes ComputeAll keeps in flight.
//
// The cap is deliberately small and not derived from the CPU count: the work is
// process-spawn and worktree-scan bound, not CPU bound, so it stops scaling
// well before the cores run out. Measured over 44 repositories on an 18-core
// machine, the sweep bottoms out around 6-8 workers (~225 ms) and gets steadily
// worse above that (~340 ms at 16), as the concurrent scans contend for the
// same disk.
const maxParallelStatus = 8

// ComputeAll returns the git status of every path, keyed by path. Duplicate
// paths are computed once.
//
// The paths are processed concurrently: each one costs a git spawn plus a
// worktree scan, so computing them in sequence makes the sweep as slow as the
// sum of every project, while computing them together makes it roughly as slow
// as the single slowest project.
func ComputeAll(paths []string) map[string]Status {
	unique := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !seen[path] {
			seen[path] = true
			unique = append(unique, path)
		}
	}

	results := make(map[string]Status, len(unique))
	if len(unique) == 0 {
		return results
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, maxParallelStatus)

	for _, path := range unique {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			status := Compute(path)

			mu.Lock()
			results[path] = status
			mu.Unlock()
		}(path)
	}
	wg.Wait()

	return results
}
