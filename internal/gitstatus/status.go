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
func Compute(path string) Status {
	// Check if path is inside a git work-tree.
	_, ok := runGit(path, "-C", path, "rev-parse", "--is-inside-work-tree")
	if !ok {
		return Status{IsRepo: false}
	}

	s := Status{IsRepo: true}

	// Parse porcelain v2 status output.
	out, ok := runGit(path, "-C", path, "status", "--porcelain=v2", "--branch")
	if ok {
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
	}

	// Check for stash.
	_, stashOK := runGit(path, "-C", path, "rev-parse", "--verify", "--quiet", "refs/stash")
	if stashOK {
		s.Stashed = true
	}

	// Clean if repo has none of the dirty indicators.
	s.Clean = !s.Modified && !s.Staged && !s.Untracked && !s.Conflict && !s.Stashed

	return s
}
