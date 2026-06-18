package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// FetchTimeout returns the recommended timeout for callers to use when
// building the context passed to Fetch.
func FetchTimeout() time.Duration {
	return 15 * time.Second
}

// HasRemote reports whether the git repository at path has at least one
// configured remote. Uses a 5-second timeout. Any error returns false.
func HasRemote(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "remote")
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")

	out, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

// Fetch runs "git fetch --quiet" in the repository at path, using the
// provided context for cancellation and timeout. If the repo has no remotes,
// it returns nil immediately without running fetch.
//
// The command is configured to never prompt for credentials:
//   - GIT_TERMINAL_PROMPT=0 disables terminal prompts
//   - GIT_SSH_COMMAND sets ssh to batch mode with a connect timeout
//   - GIT_OPTIONAL_LOCKS=0 avoids unnecessary lock files
//
// Errors are returned but callers typically ignore them (silent staleness).
func Fetch(ctx context.Context, path string) error {
	if !HasRemote(path) {
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "fetch", "--quiet")
	cmd.Dir = path
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=10",
		"GIT_OPTIONAL_LOCKS=0",
	)

	return cmd.Run()
}
