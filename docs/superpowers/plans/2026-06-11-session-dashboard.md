# Session Dashboard (ctrl+o) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `ctrl+o` in the project list opens a `vmx-dashboard` tmux session showing every active `vmx-*` session as a live tiled pane, rebuilt fresh on every open.

**Architecture:** New helpers in `internal/tmux` (constant, nested-attach command builder, pure session filter, dashboard builder), a `ctrl+o` handler plus `openDashboard()` in `internal/app/update.go` reusing the existing `tea.ExecProcess` / `TmuxReturnedMsg` attach path, and a status-message passthrough on the project list for the empty case.

**Tech Stack:** Go, tmux (shelled out via `os/exec`), bubbletea v2, bubbles v2 list.

**Spec:** `docs/superpowers/specs/2026-06-11-session-dashboard-design.md`

**Committing:** Do NOT run `git commit` at any point (user rule). Each task ends with a verification checkpoint instead. The user commits via `/commita` after reviewing the finished work.

---

### Task 1: tmux helpers (constant, nested attach command, session filter)

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tmux/tmux_test.go`:

```go
// TestNestedAttachCommand verifies the shell command a dashboard pane runs:
// TMUX must be cleared (tmux refuses to nest otherwise), exec must tie the
// pane lifetime to the inner client, and the target must use exact-name
// matching like every other lookup in this package.
func TestNestedAttachCommand(t *testing.T) {
	got := nestedAttachCommand("vmx-foo")
	want := "TMUX= exec tmux attach-session -t '=vmx-foo'"
	if got != want {
		t.Errorf("nestedAttachCommand(%q) = %q, want %q", "vmx-foo", got, want)
	}
}

// TestDashboardSessions verifies the dashboard never shows itself and that
// the result is sorted for stable pane order.
func TestDashboardSessions(t *testing.T) {
	active := map[string]bool{
		"vmx-beta":        true,
		DashboardSession:  true,
		"vmx-alpha":       true,
	}
	got := DashboardSessions(active)
	want := []string{"vmx-alpha", "vmx-beta"}
	if len(got) != len(want) {
		t.Fatalf("DashboardSessions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DashboardSessions = %v, want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tmux/ -run 'TestNestedAttachCommand|TestDashboardSessions' -v`
Expected: compile error, `undefined: nestedAttachCommand`, `undefined: DashboardSession`, `undefined: DashboardSessions`

- [ ] **Step 3: Implement the helpers**

In `internal/tmux/tmux.go`, add `"sort"` to the imports, then add below `SessionName`:

```go
// DashboardSession is the tmux session name for the all-sessions dashboard.
const DashboardSession = sessionPrefix + "dashboard"

// nestedAttachCommand returns the shell command a dashboard pane runs to
// attach a nested tmux client to the named session. TMUX= clears the env var
// so tmux allows nesting, and exec ties the pane lifetime to the inner client
// so the pane closes automatically when the session dies.
func nestedAttachCommand(name string) string {
	return "TMUX= exec tmux attach-session -t '" + exactTarget(name) + "'"
}

// DashboardSessions takes the active vibemux session set and returns the
// sessions the dashboard should display: everything except the dashboard
// itself, sorted alphabetically for stable pane order.
func DashboardSessions(active map[string]bool) []string {
	names := make([]string, 0, len(active))
	for name := range active {
		if name != DashboardSession {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmux/ -run 'TestNestedAttachCommand|TestDashboardSessions' -v`
Expected: PASS for both

- [ ] **Step 5: Checkpoint**

Run: `gofmt -l . && go test ./...`
Expected: no files listed by gofmt, all tests pass (the live-tmux test skips if tmux is absent). Do not commit.

---

### Task 2: BuildDashboard

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tmux/tmux_test.go` (add `"strings"` to the test file's imports):

```go
// TestBuildDashboard exercises the dashboard build against a real tmux
// server: two source sessions must produce a vmx-dashboard with two panes.
// Note: this kills any existing vmx-dashboard, so don't run the test suite
// while using a real dashboard.
func TestBuildDashboard(t *testing.T) {
	if !IsInstalled() {
		t.Skip("tmux not installed")
	}

	const (
		s1 = "vmx-dashtest-one"
		s2 = "vmx-dashtest-two"
	)
	cleanup := func() {
		for _, n := range []string{s1, s2, DashboardSession} {
			_ = exec.Command("tmux", "kill-session", "-t", "="+n).Run()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := NewSession(s1, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s1, err)
	}
	if err := NewSession(s2, "/tmp"); err != nil {
		t.Fatalf("failed to create test session %q: %v", s2, err)
	}

	if err := BuildDashboard([]string{s1, s2}); err != nil {
		t.Fatalf("BuildDashboard() error: %v", err)
	}
	if !HasSession(DashboardSession) {
		t.Fatal("dashboard session does not exist after BuildDashboard")
	}

	out, err := exec.Command("tmux", "list-panes", "-t", "="+DashboardSession, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}
	panes := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(panes) != 2 {
		t.Errorf("dashboard has %d panes, want 2", len(panes))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -run TestBuildDashboard -v`
Expected: compile error, `undefined: BuildDashboard`

- [ ] **Step 3: Implement BuildDashboard**

In `internal/tmux/tmux.go`, add `"errors"` to the imports, then add below `DashboardSessions`:

```go
// BuildDashboard creates a detached vmx-dashboard session containing one
// nested-client pane per given session, in the order given, tiled. On any
// failure the half-built dashboard is killed and the error returned. The
// caller is responsible for killing any pre-existing dashboard first.
func BuildDashboard(sessions []string) error {
	if len(sessions) == 0 {
		return errors.New("no sessions to show")
	}

	if err := exec.Command("tmux", "new-session", "-d", "-s", DashboardSession, nestedAttachCommand(sessions[0])).Run(); err != nil {
		return err
	}
	for _, name := range sessions[1:] {
		if err := exec.Command("tmux", "split-window", "-t", exactTarget(DashboardSession), nestedAttachCommand(name)).Run(); err != nil {
			KillSession(DashboardSession)
			return err
		}
		// Retile after every split: split-window halves the current pane, so
		// without rebalancing a handful of sessions hits "pane too small".
		_ = exec.Command("tmux", "select-layout", "-t", exactTarget(DashboardSession), "tiled").Run()
	}
	if err := exec.Command("tmux", "select-layout", "-t", exactTarget(DashboardSession), "tiled").Run(); err != nil {
		KillSession(DashboardSession)
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux/ -run TestBuildDashboard -v`
Expected: PASS (or SKIP where tmux is unavailable)

- [ ] **Step 5: Checkpoint**

Run: `gofmt -l . && go test ./...`
Expected: no files listed, all tests pass. Do not commit.

---

### Task 3: Project list status message and help line

**Files:**
- Modify: `internal/ui/projectlist/projectlist.go`

No new unit test: both changes are thin passthroughs over the bubbles list component; the compiler and existing tests cover them.

- [ ] **Step 1: Add the StatusMessage passthrough**

In `internal/ui/projectlist/projectlist.go`, add below `ShowActiveOnly` (around line 218):

```go
// StatusMessage shows a transient status message in the list (e.g. when the
// dashboard has nothing to show).
func (m *Model) StatusMessage(s string) tea.Cmd {
	return m.list.NewStatusMessage(s)
}
```

- [ ] **Step 2: Add ctrl+o to the help line**

In `View()` (line 158), change:

```go
help := fmt.Sprintf("enter open  type filter  %s  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit", toggle)
```

to:

```go
help := fmt.Sprintf("enter open  type filter  %s  ctrl+o dashboard  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit", toggle)
```

- [ ] **Step 3: Verify it compiles and tests pass**

Run: `go build ./... && go test ./...`
Expected: clean build, all tests pass. If `NewStatusMessage` does not exist on the bubbles v2 `list.Model`, the build fails here; in that case check `go doc charm.land/bubbles/v2/list.Model` for the v2 equivalent before improvising.

- [ ] **Step 4: Checkpoint**

Run: `gofmt -l .`
Expected: no files listed. Do not commit.

---

### Task 4: ctrl+o handler and openDashboard

**Files:**
- Modify: `internal/app/update.go`

No new unit test: `openDashboard` is thin orchestration over tmux side effects (same as the untested `openProject` beside it); its logic lives in the helpers tested in Tasks 1-2. Behavior is verified manually in Task 6.

- [ ] **Step 1: Add the key case**

In `internal/app/update.go`, in the non-filtering `switch s` block of `updateProjectList` (after the `ctrl+a` case, line 94-96):

```go
case "ctrl+o":
	return m.openDashboard()
```

- [ ] **Step 2: Add openDashboard**

Add below `openProject` (after line 162):

```go
func (m AppModel) openDashboard() (tea.Model, tea.Cmd) {
	if !tmux.IsInstalled() {
		fmt.Fprintf(os.Stderr, "tmux is not installed\n")
		return m, nil
	}

	sessions, _ := tmux.ListVibemuxSessions()
	names := tmux.DashboardSessions(sessions)
	if len(names) == 0 {
		return m, m.projectList.StatusMessage("no active sessions")
	}

	// Rebuild from scratch so the grid always matches the active sessions.
	tmux.KillSession(tmux.DashboardSession)
	if err := tmux.BuildDashboard(names); err != nil {
		fmt.Fprintf(os.Stderr, "Error building dashboard: %v\n", err)
		return m, nil
	}

	cmd := tea.ExecProcess(tmux.AttachCommand(tmux.DashboardSession), func(err error) tea.Msg {
		return TmuxReturnedMsg{Err: err}
	})
	return m, cmd
}
```

- [ ] **Step 3: Verify it compiles and tests pass**

Run: `go build ./... && go test ./...`
Expected: clean build, all tests pass

- [ ] **Step 4: Checkpoint**

Run: `gofmt -l .`
Expected: no files listed. Do not commit.

---

### Task 5: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add ctrl+o to the key table**

In the "Project list (startup screen)" code block, after the `ctrl+a` line, add:

```
ctrl+o         Open the dashboard (every active session as a live tiled pane)
```

- [ ] **Step 2: Add a Dashboard subsection**

After the "Adding a project" section and before "Inside a tmux session", add:

```markdown
### Dashboard (ctrl+o)

Press `ctrl+o` in the project list to open a `vmx-dashboard` tmux session
showing every active vibemux session as a live pane in a tiled grid: a
mission-control view for watching everything at once and switching context
fast.

- The dashboard is rebuilt from scratch every time you press `ctrl+o`, so it
  always reflects the sessions that are active right now.
- Each pane is a nested tmux client attached to one session. Typing into the
  focused pane interacts with that session normally.
- `Ctrl+b` targets the dashboard itself: `Ctrl+b d` detaches it and returns
  to vibemux, `Ctrl+b` + arrows move between panes. Press the prefix twice
  (`Ctrl+b Ctrl+b ...`) to send a command to the inner session under the
  focused pane.
- When a session ends, its pane closes automatically.
- If no sessions are active, vibemux shows "no active sessions" and stays in
  the project list.
```

- [ ] **Step 3: Checkpoint**

Reread both edits for accuracy against the implemented behavior. Do not commit.

---

### Task 6: Full verification

- [ ] **Step 1: Test suite, build, formatting**

Run: `gofmt -l . && go test ./... && go build -o vibemux .`
Expected: no gofmt output, all tests pass, clean build

- [ ] **Step 2: Manual smoke test (requires a terminal with tmux)**

1. Create two sessions: `tmux new-session -d -s vmx-smoke-a -c /tmp` and `tmux new-session -d -s vmx-smoke-b -c /tmp`
2. Run `./vibemux`, press `ctrl+o`
3. Expected: attached to `vmx-dashboard` with two tiled panes, one per session
4. `Ctrl+b d` to detach: back in the vibemux project list
5. Kill both smoke sessions (`tmux kill-session -t =vmx-smoke-a`, same for `-b`) plus `tmux kill-session -t =vmx-dashboard`, press `ctrl+o` again
6. Expected: "no active sessions" status message, still in the project list

If running headless without a usable TTY for step 2 onward, report that the manual smoke test needs the user and stop there.

- [ ] **Step 3: Report**

Summarize results to the user and ask whether to commit (via `/commita`). Do not commit unprompted.
