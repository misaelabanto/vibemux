# Dual Multiplexer Support (tmux + zellij) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** vibemux supports both tmux and zellij behind one `Multiplexer` interface, picks the active one through a first-run/self-heal onboarding flow, and persists the choice; the dashboard is removed from the combined app.

**Architecture:** A new leaf package `internal/mux` defines a `Kind` enum and a `Multiplexer` interface; `internal/tmux` and `internal/zellij` are refactored from package-level functions into `Backend` structs that satisfy it structurally (they must NOT import `internal/mux`, or there is an import cycle). A registry in `internal/mux` (which DOES import both backends) maps `Kind` to a backend and reports which are installed. `main.go` resolves the active multiplexer from `settings.json`; if none is validly saved it injects `nil` and the app starts in a new `ViewOnboarding` sub-model that detects installs, guides/auto-selects/prompts, persists, and hands a live `Multiplexer` to the rest of the app via dependency injection.

**Tech Stack:** Go 1.24.2, bubbletea v2 (`charm.land/bubbletea/v2`), lipgloss v2, tmux + zellij shelled out via `os/exec`.

**Design source:** `CONTEXT.md` (glossary: Multiplexer, Session, Project, Active multiplexer, Onboarding) and the brainstorming decisions captured in the originating conversation.

## Global Constraints

- **Module path:** `github.com/misaelabanto/vibemux`. Go `1.24.2`.
- **No em dashes (—)** anywhere: code, comments, UI copy, README, commit messages. Use a hyphen, colon, comma, or two sentences. (User rule, absolute.)
- **Import direction:** `internal/tmux` and `internal/zellij` must NOT import `internal/mux` (cycle). `internal/mux` imports both. UI/app packages import `internal/mux`. Backend interface conformance is enforced at compile time by `mux.New` returning the structs typed as `Multiplexer` (Task 3), never by an assertion inside a backend package.
- **Dashboard is out of scope and removed:** delete `ctrl+o`, `openDashboard`, all `Dashboard*`/`BuildDashboard` functions, and `internal/zellij/web.go` + `web_test.go`. The `Multiplexer` interface has no dashboard methods.
- **Persistence model:** persist whatever is active (even an auto-selected single backend). Onboarding runs only when no valid choice is saved (first run or self-heal). Saved value wins on later launches unless its binary is gone.
- **Install policy:** onboarding NEVER runs an installer. It shows copy-paste commands and a re-check key only.
- **bubbletea v2 keys:** sub-models read keys via `key.String()` (e.g. `"enter"`, `"up"`, `"r"`, `"q"`, `"esc"`, `"ctrl+c"`). In tests, construct special keys as `tea.KeyPressMsg{Code: tea.KeyEnter}` / `{Code: tea.KeyDown}` and printable keys as `tea.KeyPressMsg{Code: 'r', Text: "r"}`.
- **Verification per task:** `gofmt -l .` (no output), `go vet ./...`, `go build ./...`, `go test ./...`. Live-multiplexer tests skip when the binary is absent.
- **Committing:** Each task's final step runs `/commita` (the user's AI-grouped commit+push command). This is the standing authorization for subagents executing this approved plan; it does NOT authorize ad-hoc commits in the main session. Never hand-craft `git add && git commit`.

---

## File Structure

**Create:**
- `internal/mux/mux.go` — `Kind`, `Multiplexer` interface, `All`, `Parse`, `Active`.
- `internal/mux/registry.go` — `New`, `Installed` (imports tmux + zellij).
- `internal/mux/mux_test.go` — `Parse`, `Active`, `New` tests (enforces conformance).
- `internal/config/settings.go` — `Settings`, `SettingsFile`, `LoadSettings`, `SaveSettings`.
- `internal/config/settings_test.go` — round-trip + missing-file tests.
- `internal/ui/onboarding/install.go` — `installHint(kind, goos)` copy table.
- `internal/ui/onboarding/onboarding.go` — bubbletea sub-model (install / confirm / select states).
- `internal/ui/onboarding/onboarding_test.go` — state-transition + selection tests.

**Modify:**
- `internal/tmux/tmux.go` + `internal/tmux/tmux_test.go` — convert to `Backend` struct, add `Name`, drop dashboard.
- `internal/zellij/zellij.go` + `internal/zellij/zellij_test.go` — convert to `Backend` struct, add `Name`, drop dashboard + web sharing.
- `internal/app/model.go` — DI fields, `ViewOnboarding`, new constructor.
- `internal/app/update.go` — DI the mux, onboarding routing, remove dashboard.
- `internal/app/view.go` — render `ViewOnboarding`.
- `internal/ui/projectlist/projectlist.go` — drop `ctrl+o` from help line; fix two zellij-specific comments.
- `main.go` — resolve active multiplexer, new constructor call.
- `README.md` — dual-backend + onboarding, dashboard removed.

**Delete:**
- `internal/zellij/web.go`, `internal/zellij/web_test.go`.

---

### Task 1: Convert tmux to a `Backend` struct, drop the dashboard

This is a behavior-preserving refactor of `internal/tmux`: package-level functions become methods on `Backend{}`, a `Name()` method is added, and the dashboard helpers are removed. Tests are updated to the method API first (so they fail to compile), then the implementation follows.

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

**Interfaces:**
- Produces: `tmux.Backend` struct with methods `Name() string`, `IsInstalled() bool`, `SessionName(projectPath string) string`, `HasSession(name string) bool`, `NewSession(name, dir string) error`, `AttachCommand(name string) *exec.Cmd`, `KillSession(name string) error`, `ListVibemuxSessions() (map[string]bool, error)`. Package-level `exactTarget` and `sessionPrefix` remain unexported. No `Dashboard*`/`BuildDashboard`/`nestedAttachCommand` remain.

- [ ] **Step 1: Rewrite the test file to the method API and drop dashboard tests**

Replace the entire contents of `internal/tmux/tmux_test.go` with:

```go
package tmux

import (
	"os/exec"
	"testing"
)

// b is the backend under test; tmux methods have no per-instance state.
var b = Backend{}

// TestHasSessionExactMatch verifies HasSession only matches exact session
// names, not prefixes. Regression test: tmux's default `-t name` target
// supports prefix matching, which caused vibemux to think `vmx-agendalo`
// already existed when only `vmx-agendalo-app-nuxt` did, and to attach to
// the wrong session.
func TestHasSessionExactMatch(t *testing.T) {
	if !b.IsInstalled() {
		t.Skip("tmux not installed")
	}

	const (
		longName  = "vmx-pfxtest-XYZ-long"
		shortName = "vmx-pfxtest-XYZ"
	)

	// Ensure clean state.
	_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	_ = exec.Command("tmux", "kill-session", "-t", "="+shortName).Run()

	if err := b.NewSession(longName, "/tmp"); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-session", "-t", "="+longName).Run()
	})

	if !b.HasSession(longName) {
		t.Fatalf("HasSession(%q) = false, want true (session was just created)", longName)
	}

	if b.HasSession(shortName) {
		t.Errorf("HasSession(%q) = true, want false: only %q exists; tmux prefix match leaked through",
			shortName, longName)
	}
}

// TestName verifies the backend reports its persisted/display name.
func TestName(t *testing.T) {
	if got := b.Name(); got != "tmux" {
		t.Errorf("Name() = %q, want %q", got, "tmux")
	}
}

// TestSessionName verifies the deterministic vmx- name derived from a path.
func TestSessionName(t *testing.T) {
	if got := b.SessionName("/home/u/code/myproject"); got != "vmx-myproject" {
		t.Errorf("SessionName() = %q, want %q", got, "vmx-myproject")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/tmux/ -run 'TestName|TestSessionName' -v`
Expected: build failure, `b.IsInstalled undefined (type Backend has no field or method ...)` / `undefined: Backend`.

- [ ] **Step 3: Convert `tmux.go` to the `Backend` struct and remove the dashboard**

Replace the entire contents of `internal/tmux/tmux.go` with:

```go
package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sessionPrefix = "vmx-"

// Backend drives tmux. It has no state; methods shell out to the tmux binary.
type Backend struct{}

// Name is the multiplexer's persisted, human-readable identity.
func (Backend) Name() string { return "tmux" }

// SessionName returns a deterministic tmux session name derived from the
// base directory name of the project path (e.g. "vmx-myproject").
func (Backend) SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

// IsInstalled checks whether tmux is available on PATH.
func (Backend) IsInstalled() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// exactTarget prefixes a session name with "=" so tmux performs an exact-name
// lookup instead of its default prefix match. Without this, `vmx-agendalo`
// would match an existing `vmx-agendalo-app-nuxt` and the wrong session would
// be returned/attached/killed.
func exactTarget(name string) string {
	return "=" + name
}

// HasSession checks whether a tmux session with the given name exists.
func (Backend) HasSession(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", exactTarget(name)).Run()
	return err == nil
}

// NewSession creates a new detached tmux session with the given name and
// working directory.
func (Backend) NewSession(name, dir string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", name, "-c", dir).Run()
}

// AttachCommand returns an *exec.Cmd that attaches to the named tmux session.
// Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors so that
// bubbletea's ExecProcess won't override them with its wrapped readers/writers,
// which tmux cannot use (it needs a real /dev/tty).
func (Backend) AttachCommand(name string) *exec.Cmd {
	cmd := exec.Command("tmux", "attach-session", "-t", exactTarget(name))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// KillSession destroys the named tmux session.
func (Backend) KillSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", exactTarget(name)).Run()
}

// ListVibemuxSessions returns a set of active tmux session names that have the
// vibemux prefix.
func (Backend) ListVibemuxSessions() (map[string]bool, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		// tmux returns error when no server is running: treat as empty.
		return map[string]bool{}, nil
	}

	sessions := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, sessionPrefix) {
			sessions[line] = true
		}
	}
	return sessions, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tmux/ -v`
Expected: PASS for `TestName`, `TestSessionName`; `TestHasSessionExactMatch` PASS or SKIP (no tmux).

- [ ] **Step 5: Checkpoint**

Run: `gofmt -l . && go vet ./internal/tmux/ && go build ./internal/tmux/ && go test ./internal/tmux/`
Expected: no gofmt output, clean vet/build, tests pass. (`go build ./...` will still fail until the app is rewired in later tasks; that is expected.)

- [ ] **Step 6: Commit**

Run `/commita` to commit this task.

---

### Task 2: Convert zellij to a `Backend` struct, drop the dashboard and web sharing

Same refactor for `internal/zellij`. Additionally, `NewSession` stops creating web-shared sessions (web sharing only existed to feed the removed web dashboard), and the entire web layer is deleted.

**Files:**
- Modify: `internal/zellij/zellij.go`
- Test: `internal/zellij/zellij_test.go`
- Delete: `internal/zellij/web.go`, `internal/zellij/web_test.go`

**Interfaces:**
- Produces: `zellij.Backend` struct with the same method set as `tmux.Backend` (Task 1 Interfaces block). Package-level `binaryPath`, `command`, `liveSessions`, `sessionPrefix` remain unexported. No `Dashboard*`/`BuildDashboard`/`mirrorScript`/`dashboardLayout`, no web functions, no `webSharingConfig` remain.

- [ ] **Step 1: Delete the web layer**

```bash
git rm internal/zellij/web.go internal/zellij/web_test.go
```

- [ ] **Step 2: Rewrite the test file to the method API and drop dashboard tests**

Replace the entire contents of `internal/zellij/zellij_test.go` with:

```go
package zellij

import (
	"testing"
	"time"
)

// b is the backend under test; zellij methods have no per-instance state.
var b = Backend{}

// isolate points zellij at throwaway socket and cache directories so the
// tests never touch (or see) the user's real sessions.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("ZELLIJ_SOCKET_DIR", t.TempDir())
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
```

- [ ] **Step 3: Run the tests to verify they fail to compile**

Run: `go test ./internal/zellij/ -run 'TestName|TestSessionName' -v`
Expected: build failure, `undefined: Backend` (and the deleted web symbols no longer referenced).

- [ ] **Step 4: Convert `zellij.go` to the `Backend` struct, remove dashboard + web sharing**

Replace the entire contents of `internal/zellij/zellij.go` with:

```go
package zellij

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sessionPrefix = "vmx-"

// Backend drives zellij. It has no state; methods shell out to the zellij
// binary resolved by binaryPath.
type Backend struct{}

// Name is the multiplexer's persisted, human-readable identity.
func (Backend) Name() string { return "zellij" }

// SessionName returns a deterministic zellij session name derived from the
// base directory name of the project path (e.g. "vmx-myproject").
func (Backend) SessionName(projectPath string) string {
	base := filepath.Base(filepath.Clean(projectPath))
	if base == "" || base == "." || base == "/" {
		return sessionPrefix + "unknown"
	}
	return sessionPrefix + base
}

// binaryPath resolves the zellij binary: PATH first, then ~/.local/bin,
// which is where zellij's own installer puts it and which is often missing
// from non-login-shell PATHs.
func binaryPath() string {
	if p, err := exec.LookPath("zellij"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".local", "bin", "zellij")
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p
	}
	return ""
}

// command builds an *exec.Cmd for the resolved zellij binary.
func command(args ...string) *exec.Cmd {
	return exec.Command(binaryPath(), args...)
}

// IsInstalled checks whether zellij is available on PATH or in ~/.local/bin.
func (Backend) IsInstalled() bool {
	return binaryPath() != ""
}

// liveSessions returns the set of currently live session names. zellij has
// no has-session command and `list-sessions -n` mixes live and EXITED
// (dead but resurrectable) sessions indistinguishably, so this parses the
// output and drops EXITED lines.
func liveSessions() map[string]bool {
	out, err := command("list-sessions", "-n").Output()
	if err != nil {
		// zellij exits non-zero when no sessions exist; treat as empty.
		return map[string]bool{}
	}

	live := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "EXITED") {
			continue
		}
		name, _, _ := strings.Cut(line, " ")
		live[name] = true
	}
	return live
}

// HasSession checks whether a live zellij session with the given name exists.
// Names are matched exactly; EXITED sessions do not count.
func (Backend) HasSession(name string) bool {
	return liveSessions()[name]
}

// NewSession creates a new detached zellij session with the given name and
// working directory. The working directory is set both on the process (zellij
// has no --cwd flag for session creation) and via the options subcommand so
// panes opened later in the session also start there.
func (Backend) NewSession(name, dir string) error {
	cmd := command("attach", "--create-background", name, "options", "--default-cwd", dir)
	cmd.Dir = dir
	return cmd.Run()
}

// AttachCommand returns an *exec.Cmd that attaches to the named zellij
// session. Stdin/Stdout/Stderr are pre-set to the real TTY file descriptors
// so that bubbletea's ExecProcess won't override them with its wrapped
// readers/writers, which zellij cannot use (it needs a real /dev/tty).
func (Backend) AttachCommand(name string) *exec.Cmd {
	cmd := command("attach", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// KillSession destroys the named zellij session. kill-session alone leaves a
// resurrectable EXITED corpse behind (serialized to the cache dir), so the
// session is also deleted, best effort, to match tmux semantics where a
// killed session is gone.
func (Backend) KillSession(name string) error {
	err := command("kill-session", name).Run()
	_ = command("delete-session", "--force", name).Run()
	return err
}

// ListVibemuxSessions returns a set of live zellij session names that have
// the vibemux prefix. Returns an empty map when no sessions exist.
func (Backend) ListVibemuxSessions() (map[string]bool, error) {
	sessions := map[string]bool{}
	for name := range liveSessions() {
		if strings.HasPrefix(name, sessionPrefix) {
			sessions[name] = true
		}
	}
	return sessions, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/zellij/ -v`
Expected: PASS for `TestName`, `TestSessionName`; the live-zellij tests PASS or SKIP.

- [ ] **Step 6: Checkpoint**

Run: `gofmt -l . && go vet ./internal/zellij/ && go build ./internal/zellij/ && go test ./internal/zellij/`
Expected: no gofmt output, clean vet/build, tests pass. (`go build ./...` still fails until later tasks.)

- [ ] **Step 7: Commit**

Run `/commita` to commit this task.

---

### Task 3: `internal/mux` package (Kind, Multiplexer, registry)

Defines the abstraction and the registry that wires the two backends. `mux.New` returning the structs typed as `Multiplexer` is what enforces, at compile time, that both backends satisfy the interface.

**Files:**
- Create: `internal/mux/mux.go`
- Create: `internal/mux/registry.go`
- Test: `internal/mux/mux_test.go`

**Interfaces:**
- Consumes: `tmux.Backend`, `zellij.Backend` (Tasks 1-2).
- Produces:
  - `type Kind string`; consts `Tmux Kind = "tmux"`, `Zellij Kind = "zellij"`.
  - `type Multiplexer interface { Name() string; IsInstalled() bool; SessionName(projectPath string) string; HasSession(name string) bool; NewSession(name, dir string) error; AttachCommand(name string) *exec.Cmd; KillSession(name string) error; ListVibemuxSessions() (map[string]bool, error) }`
  - `func All() []Kind`
  - `func Parse(s string) (Kind, bool)`
  - `func Active(saved string, installed []Kind) (Kind, bool)`
  - `func New(k Kind) (Multiplexer, error)`
  - `func Installed() []Kind`

- [ ] **Step 1: Write the failing tests**

Create `internal/mux/mux_test.go`:

```go
package mux

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
		ok   bool
	}{
		{"tmux", Tmux, true},
		{"zellij", Zellij, true},
		{"fish", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestActive(t *testing.T) {
	both := []Kind{Tmux, Zellij}
	cases := []struct {
		name      string
		saved     string
		installed []Kind
		want      Kind
		ok        bool
	}{
		{"saved and installed", "zellij", both, Zellij, true},
		{"saved not installed", "zellij", []Kind{Tmux}, "", false},
		{"empty saved", "", []Kind{Tmux}, "", false},
		{"unknown saved", "fish", both, "", false},
	}
	for _, c := range cases {
		got, ok := Active(c.saved, c.installed)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: Active(%q, %v) = (%q, %v), want (%q, %v)",
				c.name, c.saved, c.installed, got, ok, c.want, c.ok)
		}
	}
}

// TestNewKnownKinds also enforces, at compile time, that both backends
// satisfy Multiplexer (New returns them typed as Multiplexer).
func TestNewKnownKinds(t *testing.T) {
	for _, k := range All() {
		m, err := New(k)
		if err != nil {
			t.Fatalf("New(%q) error: %v", k, err)
		}
		if m.Name() != string(k) {
			t.Errorf("New(%q).Name() = %q, want %q", k, m.Name(), string(k))
		}
	}
}

func TestNewUnknownKind(t *testing.T) {
	if _, err := New("fish"); err == nil {
		t.Error(`New("fish") = nil error, want error`)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/mux/ -v`
Expected: build failure, `undefined: Kind`, `undefined: New`, etc.

- [ ] **Step 3: Implement `mux.go`**

Create `internal/mux/mux.go`:

```go
// Package mux defines the terminal multiplexer abstraction vibemux drives and
// the registry that maps a persisted Kind to a concrete backend.
package mux

import "os/exec"

// Kind is the persisted identity of a multiplexer backend.
type Kind string

const (
	Tmux   Kind = "tmux"
	Zellij Kind = "zellij"
)

// Multiplexer is the terminal multiplexer vibemux uses to host project
// sessions. tmux and zellij each provide an implementation.
type Multiplexer interface {
	// Name is the human-readable backend name, e.g. "tmux".
	Name() string
	// IsInstalled reports whether the backend's binary is available.
	IsInstalled() bool
	// SessionName returns the deterministic vmx- session name for a project.
	SessionName(projectPath string) string
	// HasSession reports whether a live session with the name exists.
	HasSession(name string) bool
	// NewSession creates a detached session with the name and working dir.
	NewSession(name, dir string) error
	// AttachCommand returns the command that hands the terminal to a session.
	AttachCommand(name string) *exec.Cmd
	// KillSession destroys the named session.
	KillSession(name string) error
	// ListVibemuxSessions returns the set of live vmx- session names.
	ListVibemuxSessions() (map[string]bool, error)
}

// All lists every multiplexer vibemux knows about, in display order.
func All() []Kind { return []Kind{Tmux, Zellij} }

// Parse validates a persisted string and returns its Kind.
func Parse(s string) (Kind, bool) {
	switch Kind(s) {
	case Tmux, Zellij:
		return Kind(s), true
	}
	return "", false
}

// Active resolves the multiplexer to use from a persisted value and the set
// of currently-installed kinds. It returns false when the saved value is
// empty, unrecognized, or no longer installed: the caller then onboards.
func Active(saved string, installed []Kind) (Kind, bool) {
	k, ok := Parse(saved)
	if !ok {
		return "", false
	}
	for _, ik := range installed {
		if ik == k {
			return k, true
		}
	}
	return "", false
}
```

- [ ] **Step 4: Implement `registry.go`**

Create `internal/mux/registry.go`:

```go
package mux

import (
	"fmt"

	"github.com/misaelabanto/vibemux/internal/tmux"
	"github.com/misaelabanto/vibemux/internal/zellij"
)

// New constructs the backend for a Kind. The returned value implements
// Multiplexer; an unknown kind is an error.
func New(k Kind) (Multiplexer, error) {
	switch k {
	case Tmux:
		return tmux.Backend{}, nil
	case Zellij:
		return zellij.Backend{}, nil
	}
	return nil, fmt.Errorf("unknown multiplexer %q", k)
}

// Installed returns the subset of All() whose backend binary is present,
// preserving All()'s order.
func Installed() []Kind {
	var out []Kind
	for _, k := range All() {
		m, err := New(k)
		if err == nil && m.IsInstalled() {
			out = append(out, k)
		}
	}
	return out
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/mux/ -v`
Expected: PASS for all four tests.

- [ ] **Step 6: Checkpoint**

Run: `gofmt -l . && go vet ./internal/mux/ && go build ./internal/mux/ && go test ./internal/mux/`
Expected: no gofmt output, clean vet/build, tests pass.

- [ ] **Step 7: Commit**

Run `/commita` to commit this task.

---

### Task 4: Settings persistence in `internal/config`

**Files:**
- Create: `internal/config/settings.go`
- Test: `internal/config/settings_test.go`

**Interfaces:**
- Consumes: existing `config.Dir()`, `config.EnsureDir()`.
- Produces: `type Settings struct { Multiplexer string \`json:"multiplexer"\` }`, `func SettingsFile() string`, `func LoadSettings() (Settings, error)`, `func SaveSettings(s Settings) error`.

- [ ] **Step 1: Write the failing tests**

Create `internal/config/settings_test.go`:

```go
package config

import "testing"

func TestLoadSettingsMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if s.Multiplexer != "" {
		t.Errorf("Multiplexer = %q, want empty for missing file", s.Multiplexer)
	}
}

func TestSaveLoadSettingsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SaveSettings(Settings{Multiplexer: "zellij"}); err != nil {
		t.Fatalf("SaveSettings() error: %v", err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if s.Multiplexer != "zellij" {
		t.Errorf("Multiplexer = %q, want %q", s.Multiplexer, "zellij")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'Settings' -v`
Expected: build failure, `undefined: LoadSettings` / `undefined: Settings`.

- [ ] **Step 3: Implement `settings.go`**

Create `internal/config/settings.go`:

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds vibemux's user-level preferences. Stored separately from
// projects.json because it is a single object, not a list.
type Settings struct {
	Multiplexer string `json:"multiplexer"`
}

// SettingsFile is the path to the settings JSON inside the config dir.
func SettingsFile() string {
	return filepath.Join(Dir(), "settings.json")
}

// LoadSettings reads settings.json. A missing file is not an error: it
// returns the zero Settings (no multiplexer chosen yet).
func LoadSettings() (Settings, error) {
	data, err := os.ReadFile(SettingsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings writes settings.json, creating the config dir if needed.
func SaveSettings(s Settings) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsFile(), data, 0o644)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -run 'Settings' -v`
Expected: PASS for both.

- [ ] **Step 5: Checkpoint**

Run: `gofmt -l . && go vet ./internal/config/ && go build ./internal/config/ && go test ./internal/config/`
Expected: no gofmt output, clean vet/build, tests pass.

- [ ] **Step 6: Commit**

Run `/commita` to commit this task.

---

### Task 5: Onboarding sub-model (`internal/ui/onboarding`)

A bubbletea sub-model that resolves the active multiplexer. `New` picks a screen from the installed count: 0 → install guidance (re-check with `r`), 1 → confirm "X will be used", 2+ → select. It never installs anything.

**Files:**
- Create: `internal/ui/onboarding/install.go`
- Create: `internal/ui/onboarding/onboarding.go`
- Test: `internal/ui/onboarding/onboarding_test.go`

**Interfaces:**
- Consumes: `mux.Kind`, `mux.All()`, `mux.Installed()`.
- Produces:
  - `func New(installed []mux.Kind) Model`
  - `func (m Model) Init() tea.Cmd`
  - `func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)`
  - `func (m Model) View() string`
  - `func (m Model) Chosen() (mux.Kind, bool)`
  - `func (m Model) Quit() bool`
  - `func installHint(k mux.Kind, goos string) string`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/onboarding/onboarding_test.go`:

```go
package onboarding

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/mux"
)

func enter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func down() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func press(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestConfirmSingleChoosesOnlyInstalled(t *testing.T) {
	m := New([]mux.Kind{mux.Tmux})
	if _, ok := m.Chosen(); ok {
		t.Fatal("Chosen() true before any key press")
	}
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Tmux {
		t.Errorf("after enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Tmux)
	}
}

func TestSelectMovesAndChooses(t *testing.T) {
	m := New([]mux.Kind{mux.Tmux, mux.Zellij})
	m, _ = m.Update(down())
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Zellij {
		t.Errorf("after down+enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Zellij)
	}
}

func TestInstallQuit(t *testing.T) {
	m := New(nil)
	if m.Quit() {
		t.Fatal("Quit() true before any key press")
	}
	m, _ = m.Update(press('q'))
	if !m.Quit() {
		t.Error("after q on install screen: Quit() = false, want true")
	}
}

func TestInstallRecheckReturnsCmd(t *testing.T) {
	m := New(nil)
	_, cmd := m.Update(press('r'))
	if cmd == nil {
		t.Error("pressing r on install screen returned a nil cmd, want a re-detect cmd")
	}
}

func TestReDetectAdvancesPastInstall(t *testing.T) {
	m := New(nil) // starts on the install screen
	m, _ = m.Update(reDetectedMsg{installed: []mux.Kind{mux.Zellij}})
	// Now exactly one is installed: enter should choose it.
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Zellij {
		t.Errorf("after re-detect+enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Zellij)
	}
}

func TestInstallHint(t *testing.T) {
	if got := installHint(mux.Zellij, "darwin"); !strings.Contains(got, "brew install zellij") {
		t.Errorf("installHint(zellij, darwin) = %q, want it to mention brew", got)
	}
	if got := installHint(mux.Tmux, "linux"); !strings.Contains(got, "tmux") {
		t.Errorf("installHint(tmux, linux) = %q, want it to mention tmux", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/onboarding/ -v`
Expected: build failure, `undefined: New`, `undefined: reDetectedMsg`, `undefined: installHint`.

- [ ] **Step 3: Implement `install.go`**

Create `internal/ui/onboarding/install.go`:

```go
package onboarding

import "github.com/misaelabanto/vibemux/internal/mux"

// installHint returns the recommended shell command to install the given
// multiplexer on the named OS (pass runtime.GOOS). It is copy-paste guidance
// only: vibemux never runs it.
func installHint(k mux.Kind, goos string) string {
	switch k {
	case mux.Tmux:
		switch goos {
		case "darwin":
			return "brew install tmux"
		case "linux":
			return "sudo apt install tmux   (or: sudo dnf install tmux / sudo pacman -S tmux)"
		default:
			return "see https://github.com/tmux/tmux/wiki/Installing"
		}
	case mux.Zellij:
		switch goos {
		case "darwin":
			return "brew install zellij"
		case "linux":
			return "cargo install --locked zellij   (or grab a binary from https://github.com/zellij-org/zellij/releases)"
		default:
			return "see https://zellij.dev/documentation/installation"
		}
	}
	return ""
}
```

- [ ] **Step 4: Implement `onboarding.go`**

Create `internal/ui/onboarding/onboarding.go`:

```go
package onboarding

import (
	"fmt"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/misaelabanto/vibemux/internal/mux"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type state int

const (
	stateInstall state = iota // zero multiplexers installed
	stateConfirm              // exactly one installed, confirm it
	stateSelect               // two or more installed, pick one
)

// reDetectedMsg carries a fresh installed-multiplexer scan triggered by the
// user pressing "r" on the install screen.
type reDetectedMsg struct {
	installed []mux.Kind
}

// Model resolves which multiplexer vibemux will use when none is validly
// saved, by inspecting which are installed.
type Model struct {
	state     state
	installed []mux.Kind
	cursor    int

	chosen    mux.Kind
	hasChosen bool
	quit      bool
}

// New builds the onboarding model from the set of currently-installed
// multiplexers and picks the starting screen from how many there are.
func New(installed []mux.Kind) Model {
	m := Model{installed: installed}
	m.syncState()
	return m
}

// syncState selects the screen that matches the current installed set.
func (m *Model) syncState() {
	switch len(m.installed) {
	case 0:
		m.state = stateInstall
	case 1:
		m.state = stateConfirm
	default:
		m.state = stateSelect
	}
	m.cursor = 0
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reDetectedMsg:
		m.installed = msg.installed
		m.syncState()
		return m, nil
	case tea.KeyPressMsg:
		switch m.state {
		case stateInstall:
			return m.updateInstall(msg)
		case stateConfirm:
			return m.updateConfirm(msg)
		case stateSelect:
			return m.updateSelect(msg)
		}
	}
	return m, nil
}

func (m Model) updateInstall(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "r":
		return m, reDetect
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

func (m Model) updateConfirm(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		m.chosen = m.installed[0]
		m.hasChosen = true
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

func (m Model) updateSelect(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.installed)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen = m.installed[m.cursor]
		m.hasChosen = true
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

// reDetect re-scans for installed multiplexers (the user may have installed
// one in another shell) and reports the result back into Update.
func reDetect() tea.Msg {
	return reDetectedMsg{installed: mux.Installed()}
}

// Chosen reports the multiplexer the user settled on, once they have.
func (m Model) Chosen() (mux.Kind, bool) {
	return m.chosen, m.hasChosen
}

// Quit reports that the user abandoned onboarding without choosing.
func (m Model) Quit() bool { return m.quit }

func (m Model) View() string {
	switch m.state {
	case stateInstall:
		return m.viewInstall()
	case stateConfirm:
		return m.viewConfirm()
	case stateSelect:
		return m.viewSelect()
	}
	return ""
}

func (m Model) viewInstall() string {
	out := "\n" + titleStyle.Render("  No terminal multiplexer found") + "\n\n"
	out += "  vibemux needs tmux or zellij. Install one in another shell, then press r.\n\n"
	for _, k := range mux.All() {
		out += "  " + string(k) + ":\n"
		out += dimStyle.Render("    "+installHint(k, runtime.GOOS)) + "\n\n"
	}
	out += helpStyle.Render("  r re-check  q quit")
	return out + "\n"
}

func (m Model) viewConfirm() string {
	only := m.installed[0]
	out := "\n" + titleStyle.Render(fmt.Sprintf("  %s will be used", only)) + "\n\n"
	out += dimStyle.Render(fmt.Sprintf("  %s is the only multiplexer installed.", only)) + "\n\n"
	out += helpStyle.Render("  enter continue  q quit")
	return out + "\n"
}

func (m Model) viewSelect() string {
	out := "\n" + titleStyle.Render("  Choose a multiplexer") + "\n\n"
	for i, k := range m.installed {
		if i == m.cursor {
			out += cursorStyle.Render("  > "+string(k)) + "\n"
		} else {
			out += "    " + string(k) + "\n"
		}
	}
	out += "\n" + helpStyle.Render("  ↑↓ move  enter select  q quit")
	return out + "\n"
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ui/onboarding/ -v`
Expected: PASS for all six tests.

- [ ] **Step 6: Checkpoint**

Run: `gofmt -l . && go vet ./internal/ui/onboarding/ && go build ./internal/ui/onboarding/ && go test ./internal/ui/onboarding/`
Expected: no gofmt output, clean vet/build, tests pass.

- [ ] **Step 7: Commit**

Run `/commita` to commit this task.

---

### Task 6: Wire the app to the injected multiplexer and onboarding

Inject `mux.Multiplexer` into `AppModel`, add the `ViewOnboarding` state, remove the dashboard. No new unit test: `internal/app` is thin bubbletea orchestration over side effects (the existing code ships no app-level unit tests, matching the prior session-dashboard plan's precedent); its pure logic is already covered by `mux` (Task 3) and `onboarding` (Task 5), and behavior is verified by the build plus the manual smoke test in Task 9.

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/update.go`
- Modify: `internal/app/view.go`
- Modify: `internal/ui/projectlist/projectlist.go`

**Interfaces:**
- Consumes: `mux.Multiplexer`, `mux.Kind`, `mux.New` (Task 3); `onboarding.New`, `onboarding.Model.Chosen/Quit` (Task 5); `config.SaveSettings`, `config.Settings` (Task 4).
- Produces: `func NewAppModel(projects []model.Project, active mux.Multiplexer, installed []mux.Kind) AppModel` (consumed by `main.go`, Task 7).

- [ ] **Step 1: Rewrite `model.go`**

Replace the entire contents of `internal/app/model.go` with:

```go
package app

import (
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/onboarding"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

type ViewState int

const (
	ViewProjectList ViewState = iota
	ViewAddProject
	ViewOnboarding
)

type AppModel struct {
	state       ViewState
	projectList projectlist.Model
	addProject  addproject.Model
	onboarding  onboarding.Model
	mux         mux.Multiplexer
	projects    []model.Project
	width       int
	height      int
}

// NewAppModel builds the root model. When active is nil (no validly-saved
// multiplexer) it starts in onboarding seeded with the installed set;
// otherwise it starts in the project list with active wired in.
func NewAppModel(projects []model.Project, active mux.Multiplexer, installed []mux.Kind) AppModel {
	m := AppModel{
		projectList: projectlist.New(projects, 80, 24),
		projects:    projects,
		mux:         active,
		width:       80,
		height:      24,
	}
	if active == nil {
		m.state = ViewOnboarding
		m.onboarding = onboarding.New(installed)
	} else {
		m.state = ViewProjectList
	}
	return m
}
```

- [ ] **Step 2: Rewrite `update.go`**

Replace the entire contents of `internal/app/update.go` with:

```go
package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

func (m AppModel) Init() tea.Cmd {
	if m.mux == nil {
		return m.onboarding.Init()
	}
	return refreshSessionStatus(m.mux)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MultiplexerReturnedMsg:
		// User detached or session ended: return to project list.
		projects, _ := config.LoadProjects()
		m.projects = projects
		prevActiveOnly := m.projectList.ShowActiveOnly()
		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetShowActiveOnly(prevActiveOnly)
		m.projectList.SetActiveSessions(m.projectList.ActiveSessions())
		m.state = ViewProjectList
		return m, refreshSessionStatus(m.mux)

	case SessionStatusMsg:
		active := mapSessionsToProjects(msg.ActiveSessions, m.projects, m.mux)
		m.projectList.SetActiveSessions(active)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.projectList.SetSize(msg.Width, msg.Height)
		return m, nil
	}

	switch m.state {
	case ViewOnboarding:
		return m.updateOnboarding(msg)
	case ViewProjectList:
		return m.updateProjectList(msg)
	case ViewAddProject:
		return m.updateAddProject(msg)
	}

	return m, nil
}

// updateOnboarding routes input to the onboarding sub-model and, once the
// user has chosen a multiplexer, persists it, builds the backend, and enters
// the project list. Quitting onboarding exits vibemux (it cannot run without
// a multiplexer).
func (m AppModel) updateOnboarding(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.onboarding, cmd = m.onboarding.Update(msg)

	if m.onboarding.Quit() {
		return m, tea.Quit
	}

	if k, ok := m.onboarding.Chosen(); ok {
		_ = config.SaveSettings(config.Settings{Multiplexer: string(k)})
		active, err := mux.New(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing %s: %v\n", k, err)
			return m, tea.Quit
		}
		m.mux = active
		m.state = ViewProjectList
		m.projectList.SetSize(m.width, m.height)
		return m, tea.Batch(cmd, refreshSessionStatus(m.mux))
	}

	return m, cmd
}

func (m AppModel) updateProjectList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		s := key.String()
		if s == "ctrl+c" {
			return m, tea.Quit
		}
		if s == "enter" && m.projectList.IsFiltering() {
			var cmd tea.Cmd
			m.projectList, cmd = m.projectList.Update(msg)
			if p, ok := m.projectList.SelectedProject(); ok {
				newModel, openCmd := m.openProject(p)
				return newModel, tea.Batch(cmd, openCmd)
			}
			return m, cmd
		}
		if !m.projectList.IsFiltering() {
			switch s {
			case "enter":
				if p, ok := m.projectList.SelectedProject(); ok {
					return m.openProject(p)
				}
			case "ctrl+n":
				m.state = ViewAddProject
				m.addProject = addproject.New()
				return m, m.addProject.Init()
			case "ctrl+d":
				if p, ok := m.projectList.SelectedProject(); ok {
					m.mux.KillSession(m.mux.SessionName(p.Path))
					config.RemoveProject(p.ID)
					projects, _ := config.LoadProjects()
					m.projects = projects
					cmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(cmd, refreshSessionStatus(m.mux))
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					m.mux.KillSession(m.mux.SessionName(p.Path))
					return m, refreshSessionStatus(m.mux)
				}
			case "ctrl+a":
				cmd := m.projectList.ToggleActiveOnly()
				return m, cmd
			}
		}
	}

	var cmd tea.Cmd
	m.projectList, cmd = m.projectList.Update(msg)
	return m, cmd
}

func (m AppModel) updateAddProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "ctrl+c" && !m.addProject.IsRunning() {
			m.addProject.Cancel()
			m.state = ViewProjectList
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.addProject, cmd = m.addProject.Update(msg)

	if m.addProject.Canceled() {
		m.state = ViewProjectList
		return m, nil
	}

	if path := m.addProject.SelectedPath(); path != "" {
		m.addProject.ClearSelection()
		p, err := config.AddProject(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding project: %v\n", err)
			m.state = ViewProjectList
			return m, nil
		}
		m.projects = append(m.projects, p)
		m.state = ViewProjectList
		setCmd := m.projectList.SetProjects(m.projects)
		return m, setCmd
	}

	return m, cmd
}

func (m AppModel) openProject(p model.Project) (tea.Model, tea.Cmd) {
	config.TouchProject(p.ID)

	if !m.mux.IsInstalled() {
		fmt.Fprintf(os.Stderr, "%s is not installed\n", m.mux.Name())
		return m, nil
	}

	name := m.mux.SessionName(p.Path)

	// Create a new session if one doesn't already exist.
	if !m.mux.HasSession(name) {
		if err := m.mux.NewSession(name, p.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s session: %v\n", m.mux.Name(), err)
			return m, nil
		}
	}

	cmd := tea.ExecProcess(m.mux.AttachCommand(name), func(err error) tea.Msg {
		return MultiplexerReturnedMsg{Err: err}
	})
	return m, cmd
}

// refreshSessionStatus returns a Cmd that queries the active multiplexer for
// active vibemux sessions and sends a SessionStatusMsg.
func refreshSessionStatus(mx mux.Multiplexer) tea.Cmd {
	return func() tea.Msg {
		sessions, _ := mx.ListVibemuxSessions()
		return SessionStatusMsg{ActiveSessions: sessions}
	}
}

// mapSessionsToProjects maps live multiplexer session names back to project
// IDs.
func mapSessionsToProjects(sessions map[string]bool, projects []model.Project, mx mux.Multiplexer) map[string]bool {
	active := map[string]bool{}
	for _, p := range projects {
		name := mx.SessionName(p.Path)
		if sessions[name] {
			active[p.ID] = true
		}
	}
	return active
}
```

- [ ] **Step 3: Update `view.go`**

In `internal/app/view.go`, add the onboarding case to the switch. Replace:

```go
	switch m.state {
	case ViewProjectList:
		content = m.projectList.View()
	case ViewAddProject:
		content = m.addProject.View()
	}
```

with:

```go
	switch m.state {
	case ViewProjectList:
		content = m.projectList.View()
	case ViewAddProject:
		content = m.addProject.View()
	case ViewOnboarding:
		content = m.onboarding.View()
	}
```

- [ ] **Step 4: Drop `ctrl+o` and fix zellij-specific comments in projectlist**

In `internal/ui/projectlist/projectlist.go`:

Replace the help line (currently line 162):

```go
	help := fmt.Sprintf("enter open  type filter  %s  ctrl+o dashboard  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit", toggle)
```

with:

```go
	help := fmt.Sprintf("enter open  type filter  %s  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit", toggle)
```

Replace the comment on the `activeSessions` field (currently line 54):

```go
	activeSessions map[string]bool // project ID → has active zellij session
```

with:

```go
	activeSessions map[string]bool // project ID -> has active multiplexer session
```

Replace the `SetActiveSessions` doc comment (currently lines 188-189):

```go
// SetActiveSessions updates which projects have running zellij sessions and
// rebuilds items so the active-only filter (if on) reflects the new set.
```

with:

```go
// SetActiveSessions updates which projects have running multiplexer sessions
// and rebuilds items so the active-only filter (if on) reflects the new set.
```

- [ ] **Step 5: Verify the whole module builds and all tests pass**

Run: `go build ./... && go test ./...`
Expected: clean build (main.go still uses the old constructor signature and will FAIL here). If the only build error is in `main.go` (`not enough arguments in call to app.NewAppModel`), that is expected and fixed in Task 7. Confirm there are no errors in any `internal/...` package, then proceed.

- [ ] **Step 6: Commit**

Run `/commita` to commit this task.

---

### Task 7: Resolve the active multiplexer in `main.go`

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `config.LoadProjects`, `config.LoadSettings` (Task 4); `mux.Installed`, `mux.Active`, `mux.New` (Task 3); `app.NewAppModel` (Task 6).

- [ ] **Step 1: Rewrite `main.go`**

Replace the entire contents of `main.go` with:

```go
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/app"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/mux"
)

func main() {
	projects, err := config.LoadProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading projects: %v\n", err)
		os.Exit(1)
	}

	settings, err := config.LoadSettings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading settings: %v\n", err)
		os.Exit(1)
	}

	// Non-interactive resolve: use the saved multiplexer when it is still
	// installed; otherwise leave active nil so the app onboards (first run or
	// self-heal).
	installed := mux.Installed()
	var active mux.Multiplexer
	if k, ok := mux.Active(settings.Multiplexer, installed); ok {
		active, err = mux.New(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing multiplexer: %v\n", err)
			os.Exit(1)
		}
	}

	m := app.NewAppModel(projects, active, installed)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify the whole module builds and all tests pass**

Run: `gofmt -l . && go vet ./... && go build -o vibemux . && go test ./...`
Expected: no gofmt output, clean vet, clean build (binary `vibemux` produced), all tests pass (live-multiplexer tests skip where the binary is absent).

- [ ] **Step 3: Commit**

Run `/commita` to commit this task.

---

### Task 8: README

Update `README.md` so it describes dual-backend support and onboarding, and no longer claims zellij-only or documents the dashboard.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Intro line**

Replace (line 3):

```markdown
A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent zellij sessions for your projects: detach without killing, reattach to resume exactly where you left off.
```

with:

```markdown
A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent tmux or zellij sessions for your projects: detach without killing, reattach to resume exactly where you left off.
```

- [ ] **Step 2: "What it does" bullet and the paragraph after the list**

Replace the bullet:

```markdown
- **Open zellij sessions** for projects with a keystroke
```

with:

```markdown
- **Open multiplexer sessions** (tmux or zellij) for projects with a keystroke
```

Replace the paragraph after the bullet list:

```markdown
Each project gets its own zellij session that persists even after you quit vibemux. Sessions are plain zellij sessions, so you can use any zellij features, plugins, or keybindings you're familiar with.
```

with:

```markdown
Each project gets its own session in your chosen multiplexer (tmux or zellij) that persists even after you quit vibemux. Sessions are plain multiplexer sessions, so you can use any features, plugins, or keybindings you're familiar with. On first run vibemux detects which multiplexers are installed and lets you pick one (see "First run" below).
```

- [ ] **Step 3: Both "Requirements" blocks**

Replace the first Requirements block (lines 40-44):

```markdown
- **zellij** 0.43+ (required: vibemux uses zellij for terminal emulation, and the web dashboard needs the zellij web client introduced in 0.43; 0.44+ recommended, tested against 0.44.3)
- Go 1.21+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)
```

with:

```markdown
- **tmux** or **zellij** (at least one): vibemux uses your chosen multiplexer for terminal emulation. zellij is resolved from `$PATH` or `~/.local/bin`; tmux from `$PATH`. zellij 0.44+ is recommended (tested against 0.44.3).
- Go 1.24+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)
```

Replace the second Requirements block (lines 181-185):

```markdown
- **zellij** 0.43+ must be installed, on `$PATH` or in `~/.local/bin` (0.44+ recommended)
- Go 1.21+
- A POSIX shell
```

with:

```markdown
- **tmux** (on `$PATH`) or **zellij** (on `$PATH` or in `~/.local/bin`), at least one. zellij 0.44+ recommended.
- Go 1.24+
- A POSIX shell
```

- [ ] **Step 4: Key table (remove the dashboard row)**

In the "Project list (startup screen)" code block, delete this line:

```
ctrl+o         Open the web dashboard (one browser window per active session)
```

and change the indicator/quit lines that name zellij so they are backend-neutral. Replace:

```
ctrl+x         Kill zellij session for selected project
ctrl+c         Quit vibemux (zellij sessions stay alive)
```

with:

```
ctrl+x         Kill the multiplexer session for selected project
ctrl+c         Quit vibemux (multiplexer sessions stay alive)
```

Then replace the note line:

```markdown
- The `●` indicator next to a project name shows that an active zellij session exists for it.
```

with:

```markdown
- The `●` indicator next to a project name shows that an active multiplexer session exists for it.
```

- [ ] **Step 5: Replace the "Dashboard (ctrl+o)" section with a "First run" section**

Delete the entire "### Dashboard (ctrl+o)" section (from the heading through the line `0.44.3).`, i.e. lines 79-101) and replace it with:

```markdown
### First run

The first time you launch vibemux (and any time your saved multiplexer is no
longer installed), it resolves which multiplexer to use:

- **Neither installed:** vibemux shows the install command for tmux and zellij
  on your OS. Install one in another shell, then press `r` to re-check (or `q`
  to quit). vibemux never runs an installer for you.
- **Exactly one installed:** vibemux tells you it will be used and continues on
  `enter`.
- **Both installed:** vibemux asks you to pick one.

Your choice is saved to `~/.config/vibemux/settings.json` and reused on every
later launch. To switch multiplexers, edit that file (set `"multiplexer"` to
`"tmux"` or `"zellij"`) or delete it to be asked again on the next launch.
```

- [ ] **Step 6: "Inside a zellij session" section heading and intro**

Replace the heading and first sentence:

```markdown
### Inside a zellij session

Once attached, you have full zellij control. Common bindings:
```

with:

```markdown
### Inside a session

Once attached, you have full control of your multiplexer. The bindings below
are zellij's defaults; tmux uses its own (`Ctrl+b` prefix). Common zellij
bindings:
```

- [ ] **Step 7: "Session behavior" zellij references**

In the "Session behavior" list, replace the three zellij mentions so they read backend-neutral:

- `The shell process continues running in the background` stays.
- Replace `**Kill** (`ctrl+x` from project list): Terminates the zellij session. Reopening the project starts a fresh session.` with `**Kill** (`ctrl+x` from project list): Terminates the multiplexer session. Reopening the project starts a fresh session.`
- Replace `**Quit** (`ctrl+c` from project list): Exits vibemux. All zellij sessions persist and can be reattached later.` with `**Quit** (`ctrl+c` from project list): Exits vibemux. All multiplexer sessions persist and can be reattached later.`

- [ ] **Step 8: Architecture table**

Replace the `internal/zellij` row:

```markdown
| `internal/zellij` | zellij session management (create, attach, kill, list sessions, exact name matching so `vmx-foo` doesn't collide with `vmx-foo-bar`) plus the web dashboard (web server, login token, per-session URLs) |
```

with these rows:

```markdown
| `internal/mux` | Multiplexer abstraction: the `Multiplexer` interface, `Kind` enum, registry (`New`, `Installed`), and resolution (`Active`) |
| `internal/tmux` | tmux backend: create, attach, kill, list sessions, exact name matching so `vmx-foo` doesn't collide with `vmx-foo-bar` |
| `internal/zellij` | zellij backend: same surface as tmux, resolving the binary from `$PATH` or `~/.local/bin` and dropping EXITED corpses on kill |
| `internal/ui/onboarding` | First-run / self-heal flow that detects installed multiplexers and selects the active one |
```

Also update the `internal/config` row:

```markdown
| `internal/config` | XDG-compliant config store for projects (JSON) |
```

to:

```markdown
| `internal/config` | XDG-compliant config store for projects (`projects.json`) and settings (`settings.json`, the chosen multiplexer) |
```

- [ ] **Step 9: "How it works" diagram and paragraph**

In the "How it works" section, replace the intro line:

```markdown
vibemux is a **project launcher** that delegates terminal emulation to zellij:
```

with:

```markdown
vibemux is a **project launcher** that delegates terminal emulation to your chosen multiplexer (tmux or zellij):
```

In the ASCII diagram, delete the `ctrl+o` line:

```
│     ├─ "ctrl+o" → web dashboard (zellij web server + browser windows)
```

and replace the two zellij-named lines:

```
│     ├─ "enter"  → zellij attach (hands terminal control to zellij)
```

with:

```
│     ├─ "enter"  → multiplexer attach (hands terminal control to it)
```

and:

```
│     ├─ "ctrl+x" → zellij kill-session
```

with:

```
│     ├─ "ctrl+x" → multiplexer kill-session
```

Then replace the bottom block of the diagram:

```
└─ zellij (subprocess, full terminal control)
   ├─ Session: vmx-<project-path-slug>
   │  └─ Shell with full history and process state
   └─ Detach (Ctrl+o d) → returns control to vibemux
```

with:

```
└─ multiplexer (subprocess, full terminal control)
   ├─ Session: vmx-<project-path-slug>
   │  └─ Shell with full history and process state
   └─ Detach (zellij Ctrl+o d / tmux Ctrl+b d) → returns control to vibemux
```

Finally, replace the paragraph after the diagram:

```markdown
When you open a project, vibemux creates a zellij session (if needed, with web sharing enabled so the dashboard can reach it) and uses `tea.ExecProcess()` to hand terminal control to zellij. The session persists in the background even after you detach or quit vibemux. Session names are derived from the project path (prefixed `vmx-`) and matched exactly against the live session list, so similarly-named sessions don't collide.
```

with:

```markdown
When you open a project, vibemux creates a session in your chosen multiplexer (if one doesn't already exist) and uses `tea.ExecProcess()` to hand terminal control to it. The session persists in the background even after you detach or quit vibemux. Session names are derived from the project path (prefixed `vmx-`) and matched exactly against the live session list, so similarly-named sessions don't collide.
```

- [ ] **Step 10: "Session persistence" and "Project storage" zellij references**

In "Session persistence", replace:

```markdown
Sessions are managed by zellij's background server processes, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `zellij list-sessions`
- You can manually attach with `zellij attach vmx-<slug>`
- Sessions can accumulate if not cleaned up (use `ctrl+x` or `ctrl+d` in vibemux to kill them)
```

with:

```markdown
Sessions are managed by your multiplexer's background server processes, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `tmux list-sessions` or `zellij list-sessions`
- You can manually attach with `tmux attach -t vmx-<slug>` or `zellij attach vmx-<slug>`
- Sessions can accumulate if not cleaned up (use `ctrl+x` or `ctrl+d` in vibemux to kill them)
```

In "Project storage", after the closing ``` of the JSON block, add:

```markdown

The chosen multiplexer is stored separately in `~/.config/vibemux/settings.json`:

```json
{
  "multiplexer": "zellij"
}
```
```

- [ ] **Step 11: Checkpoint**

Reread the README end to end. Confirm: no remaining "dashboard" or "ctrl+o" references, no claim that zellij is required, no em dashes introduced. Run `gofmt -l .` (no Go changed, but confirms nothing else is dirty). Do not skip the read-through.

- [ ] **Step 12: Commit**

Run `/commita` to commit this task.

---

### Task 9: Full verification

- [ ] **Step 1: Build, vet, format, test**

Run: `gofmt -l . && go vet ./... && go build -o vibemux . && go test ./...`
Expected: no gofmt output, clean vet, clean build, all tests pass (live tests skip when a binary is absent).

- [ ] **Step 2: Confirm the dashboard and web layer are gone**

Run: `grep -rn 'Dashboard\|ctrl+o\|web_sharing\|StartWebServer\|BuildDashboard' internal main.go; ls internal/zellij/`
Expected: no matches in code (matches only inside `docs/` are fine), and `internal/zellij/` contains only `zellij.go` and `zellij_test.go` (no `web.go`/`web_test.go`).

- [ ] **Step 3: Manual smoke test (requires a terminal; run for each installed backend)**

First-run / selection:
1. Back up and remove any existing settings: `mv ~/.config/vibemux/settings.json /tmp/vmx-settings.bak 2>/dev/null` (ignore if absent).
2. Run `./vibemux`.
   - If both tmux and zellij are installed: a "Choose a multiplexer" screen appears. Pick one with arrows + `enter`.
   - If exactly one is installed: a "X will be used" screen appears; press `enter`.
3. Expected: you land in the project list. `cat ~/.config/vibemux/settings.json` shows the chosen multiplexer.

Open / attach / detach:
4. With at least one project registered, press `enter` on it.
5. Expected: attached to a `vmx-<slug>` session in the chosen multiplexer. Detach (zellij `Ctrl+o d`, tmux `Ctrl+b d`): back in the project list, with a `●` next to that project.
6. Press `ctrl+x` on it: the `●` clears (session killed). Press `ctrl+c` to quit.

Self-heal (only if you can toggle installs):
7. Edit `~/.config/vibemux/settings.json` to a multiplexer you do NOT have installed (or an invalid value like `"none"`), relaunch `./vibemux`.
8. Expected: onboarding runs again rather than erroring; choosing re-writes the file.

Restore: `mv /tmp/vmx-settings.bak ~/.config/vibemux/settings.json 2>/dev/null` if you backed one up.

If running headless without a usable TTY, report that the manual smoke test needs the user and stop after Step 2.

- [ ] **Step 4: Report**

Summarize results to the user (what passed, what was skipped and why). The per-task commits already pushed via `/commita`; confirm the branch state and ask whether to open a PR.

---

## Self-Review

**1. Spec coverage:**
- "Support both tmux and zellij" → Tasks 1-3 (backends + interface + registry).
- "Use good data structures" → Task 3 (`Kind` enum, `Multiplexer` interface, registry); DI in Task 6.
- "Onboarding: detect setup" → Task 5 (`New` branches on `mux.Installed()`), Task 7 (resolve).
- "Offer to install (none)" → Task 5 `stateInstall` + `installHint`, guidance-only.
- "Select one (two installed)" → Task 5 `stateSelect`.
- "'this <tool> will be used' (one installed)" → Task 5 `stateConfirm`.
- "Persist whatever is active; resolve once; self-heal" → Task 4 (settings), Task 7 (`mux.Active`), Task 6 (save on choose; onboarding on nil).
- "Combine main (tmux) + this branch (zellij)" → Tasks 1-2 keep both backends; Task 6 injects either.
- "Dashboard not in scope" → removed in Tasks 2, 6; verified in Task 9 Step 2.

**2. Placeholder scan:** No TBD/TODO; every code step shows full code; every command states expected output. Pass.

**3. Type consistency:** `mux.Multiplexer` method set is identical in the interface (Task 3), both backends (Tasks 1-2), and the registry return (Task 3). `NewAppModel(projects, active, installed)` signature is defined in Task 6 and called identically in Task 7. `onboarding.New([]mux.Kind)`, `Chosen() (mux.Kind, bool)`, `Quit() bool` match between Task 5 (definition + tests) and Task 6 (use). `config.Settings{Multiplexer: string}` matches between Task 4 and Task 6. `refreshSessionStatus(mux.Multiplexer)` and `mapSessionsToProjects(..., mux.Multiplexer)` definitions and call sites all updated in Task 6. Pass.
