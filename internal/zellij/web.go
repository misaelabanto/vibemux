package zellij

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultWebPort is the port the zellij web server listens on by default.
const DefaultWebPort = 8082

// WebServerRunning reports whether the zellij web server is up on the given
// port. `zellij web --status` exits zero whether the server is online or
// offline, so the printed status line is parsed instead.
func WebServerRunning(port int) bool {
	out, err := command("web", "--status", "--port", strconv.Itoa(port)).Output()
	return err == nil && strings.Contains(string(out), "Web server online")
}

// StartWebServer starts the zellij web server daemonized on 127.0.0.1 at the
// given port. Idempotent: if the server is already running there, returns nil.
func StartWebServer(port int) error {
	if WebServerRunning(port) {
		return nil
	}
	out, err := command("web", "--start", "--daemonize",
		"--ip", "127.0.0.1", "--port", strconv.Itoa(port)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting zellij web server on port %d: %w: %s",
			port, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureWebToken makes sure at least one web login token exists. zellij only
// stores token hashes, so the plaintext is available exactly once, at
// creation: when a token is created here it is returned so the app can show
// it to the user. When a token already exists, token is empty and created is
// false.
func EnsureWebToken() (token string, created bool, err error) {
	out, err := command("web", "--list-tokens").Output()
	if err != nil {
		return "", false, fmt.Errorf("listing zellij web tokens: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return "", false, nil
	}

	out, err = command("web", "--create-token").Output()
	if err != nil {
		return "", false, fmt.Errorf("creating zellij web token: %w", err)
	}
	// Output looks like:
	//   Created token successfully
	//
	//   token_1: <token>
	for _, line := range strings.Split(string(out), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.HasPrefix(name, "token") && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), true, nil
		}
	}
	return "", false, fmt.Errorf("no token in zellij web --create-token output: %q", string(out))
}

// SessionURL returns the web client URL that attaches to the named session.
// Only pass names of existing sessions: the zellij web server creates a new
// session when an unknown name is visited.
func SessionURL(port int, name string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s", port, url.PathEscape(name))
}

// browserCommand builds the command used to open a URL in the user's
// browser: xdg-open when available, otherwise the command named by $BROWSER.
func browserCommand(rawURL string) (*exec.Cmd, error) {
	if p, err := exec.LookPath("xdg-open"); err == nil {
		return exec.Command(p, rawURL), nil
	}
	if b := os.Getenv("BROWSER"); b != "" {
		return exec.Command(b, rawURL), nil
	}
	return nil, errors.New("cannot open a browser: xdg-open not found and $BROWSER not set")
}

// OpenInBrowser opens the URL in the user's browser without blocking: the
// opener process is started and reaped in the background, never waited on by
// the caller.
func OpenInBrowser(rawURL string) error {
	cmd, err := browserCommand(rawURL)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// effectiveConfigPath returns the config file zellij would load, as reported
// by `zellij setup --check` (which knows the full resolution order, including
// the quirk that an existing ~/.config/zellij wins over XDG_CONFIG_HOME).
// Returns "" when the path cannot be determined; the reported file may not
// exist.
func effectiveConfigPath() string {
	out, err := command("setup", "--check").Output()
	if err != nil {
		return ""
	}
	const marker = "[LOOKING FOR CONFIG FILE FROM]:"
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), marker); ok {
			return strings.Trim(strings.TrimSpace(rest), "\"")
		}
	}
	return ""
}

// webSharingConfig writes a config file for the named session that turns
// web_sharing on while preserving the user's own config, and returns its
// path. The file lives in the temp dir for the lifetime of the session (like
// the dashboard layout file): the zellij server re-reads its config file
// after session creation, so deleting it early reverts web sharing (verified
// empirically, web_clients_allowed flips back to false).
//
// A config file is required in the first place because zellij 0.44.3
// silently ignores `options --web-sharing on` on session creation (verified
// empirically: the session metadata keeps web_clients_allowed false, and a
// web client gets "Web Clients are not allowed to attach to this session").
// The override is prepended because zellij keeps the first occurrence of a
// duplicated config option, so it also wins over an explicit
// web_sharing "off" in the user config.
func webSharingConfig(name string) (string, error) {
	content := "web_sharing \"on\"\n"
	if p := effectiveConfigPath(); p != "" {
		if user, err := os.ReadFile(p); err == nil {
			content += string(user)
		}
	}
	path := filepath.Join(os.TempDir(), "vmx-web-config-"+name+".kdl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("writing web sharing config: %w", err)
	}
	return path, nil
}
