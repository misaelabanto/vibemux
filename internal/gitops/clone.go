package gitops

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var shorthandRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// NormalizeURL accepts a repo identifier in any of these forms:
//   - owner/repo                       → git@github.com:owner/repo.git
//   - https://github.com/owner/repo    → git@github.com:owner/repo.git
//   - git@host:owner/repo(.git)?       → returned as-is
//   - ssh://...                        → returned as-is
//
// GitHub HTTPS URLs are rewritten to SSH so cloning always uses the SSH remote.
// Non-GitHub URLs are passed through unchanged.
//
// It also returns the directory name to clone into (the repo's basename
// without a trailing .git).
func NormalizeURL(input string) (url, dirName string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", fmt.Errorf("repo URL is empty")
	}

	if shorthandRe.MatchString(s) {
		parts := strings.SplitN(s, "/", 2)
		return fmt.Sprintf("git@github.com:%s/%s.git", parts[0], parts[1]), parts[1], nil
	}

	if rewritten, ok := rewriteGithubHTTPSToSSH(s); ok {
		s = rewritten
	}

	dirName = repoDirNameFromURL(s)
	if dirName == "" {
		return "", "", fmt.Errorf("could not derive directory name from %q", s)
	}
	return s, dirName, nil
}

// rewriteGithubHTTPSToSSH converts https://github.com/owner/repo(.git)? into
// git@github.com:owner/repo.git. Returns ok=false for anything else.
func rewriteGithubHTTPSToSSH(s string) (string, bool) {
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(s, prefix) {
			path := strings.TrimPrefix(s, prefix)
			path = strings.TrimSuffix(path, "/")
			if path == "" || strings.Count(path, "/") < 1 {
				return "", false
			}
			if !strings.HasSuffix(path, ".git") {
				path += ".git"
			}
			return "git@github.com:" + path, true
		}
	}
	return "", false
}

// repoDirNameFromURL extracts the trailing path segment of a git URL,
// stripping a ".git" suffix. Handles SCP-style "git@host:path" and
// scheme-style "scheme://host/path".
func repoDirNameFromURL(url string) string {
	tail := url
	if strings.Contains(tail, "@") && !strings.Contains(tail, "://") {
		if i := strings.LastIndex(tail, ":"); i >= 0 {
			tail = tail[i+1:]
		}
	}
	if i := strings.LastIndex(tail, "/"); i >= 0 {
		tail = tail[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(tail, ".git"))
}

// Clone runs `git clone <url> <parentDir>/<dirName>`. The context is wired
// through exec.CommandContext so cancelling it kills the child process.
// Returns the full clone path on success.
func Clone(ctx context.Context, url, parentDir, dirName string) (string, error) {
	target := filepath.Join(parentDir, dirName)
	cmd := exec.CommandContext(ctx, "git", "clone", "--", url, target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("cancelled")
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}
	return target, nil
}
