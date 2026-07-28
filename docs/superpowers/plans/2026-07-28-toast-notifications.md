# Toast Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace every in-TUI `os.Stderr` write with a floating toast overlay, and make the messages say something useful.

**Architecture:** A new `internal/ui/toast` bubbletea sub-model holds one message at a time with a sequence-guarded auto-dismiss timer. `AppModel.View` composites it over the existing view with lipgloss `Compositor`, so the base view is never modified and nothing shifts. `openProject` stats the Project directory before creating a Session so the common failure names its cause.

**Tech Stack:** Go 1.24.2, charm.land/bubbletea/v2 v2.0.0, charm.land/lipgloss/v2 v2.0.0.

**Spec:** `docs/superpowers/specs/2026-07-28-toast-notifications-design.md`

## Global Constraints

- No em dashes anywhere: code, comments, commit messages, UI copy. Use a comma, colon, or two sentences.
- Meaningful variable names. No single-letter or throwaway names for domain values. `project`, `session`, `toastBox`, not `p`, `s`, `t`. Numeric loop counters (`i`, `j`) are the only exception.
- CONTEXT.md glossary is binding for user-visible copy. The word "folder" is banned for a Project's directory; use "Project directory". Never use "waiting" as a state name.
- Commits use `/commita` (or `commita --no-push`). Never plain `git add && git commit`.
- Every task ends green: `go test ./...` and `go build ./...` both pass before committing.
- lipgloss overlay MUST go through `lipgloss.NewCompositor`. `Layer.Draw` ignores children and X/Y, so `Canvas.Compose(parentLayer)` silently discards the overlay.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/ui/toast/toast.go` (create) | The toast sub-model: state, `Show`, `Update`, `Visible`, `Message`, `Render`. |
| `internal/ui/toast/toast_test.go` (create) | Unit tests for sequence guard, dismissal, width clamp. |
| `internal/tmux/tmux.go` (modify) | Add a stderr-capturing `run` helper; `NewSession` uses it. |
| `internal/tmux/tmux_test.go` (modify) | Test that `run` wraps child stderr into the error. |
| `internal/ui/onboarding/onboarding.go` (modify) | Add `ClearChoice()` so a failed multiplexer init can be retried. |
| `internal/ui/onboarding/onboarding_test.go` (modify) | Test `ClearChoice` resets `Chosen()`. |
| `internal/app/model.go` (modify) | `AppModel.toast` field, initialized in `NewAppModel`. |
| `internal/app/view.go` (modify) | Compositing the toast over the view. |
| `internal/app/update.go` (modify) | Forwarding at top of `Update`; all error and confirmation routing; the pre-check. |
| `internal/app/fakemux_test.go` (create) | A recording fake `mux.Multiplexer` for app tests. |
| `internal/app/toast_test.go` (create) | App-level tests: pre-check, forwarding order, view composition. |

Task order is dependency order. Tasks 1 through 3 are independent leaves and could be done in any order; tasks 4 onward depend on task 1.

---

### Task 1: The toast sub-model

**Files:**
- Create: `internal/ui/toast/toast.go`
- Test: `internal/ui/toast/toast_test.go`

**Interfaces:**
- Consumes: `internal/ui/styles` (`styles.Error`, `styles.Muted`).
- Produces:
  - `toast.Kind` with constants `toast.KindError`, `toast.KindInfo`
  - `toast.New() Model`
  - `(*Model) Show(kind Kind, text string) tea.Cmd`
  - `(Model) Update(msg tea.Msg) (Model, tea.Cmd)`
  - `(Model) Visible() bool`
  - `(Model) Message() string`
  - `(Model) Render(maxWidth int) string`
  - `toast.ExpiredMsg{Seq int}`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/toast/toast_test.go`:

```go
package toast

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShowMakesVisible(t *testing.T) {
	model := New()
	if model.Visible() {
		t.Fatal("a new toast model must not be visible")
	}
	cmd := model.Show(KindError, "boom")
	if !model.Visible() {
		t.Error("Visible() = false after Show, want true")
	}
	if model.Message() != "boom" {
		t.Errorf("Message() = %q, want %q", model.Message(), "boom")
	}
	if cmd == nil {
		t.Error("Show returned a nil cmd, want the auto-dismiss tick")
	}
}

// TestMatchingExpiryClears verifies the happy path of the auto-dismiss timer.
func TestMatchingExpiryClears(t *testing.T) {
	model := New()
	model.Show(KindError, "boom")

	model, _ = model.Update(ExpiredMsg{Seq: model.seq})
	if model.Visible() {
		t.Error("Visible() = true after a matching ExpiredMsg, want false")
	}
}

// TestStaleExpiryLeavesNewerToast is the regression guard for the sequence
// counter. Without it, a toast raised just before an older toast's timer
// fires would be wiped by that older timer.
func TestStaleExpiryLeavesNewerToast(t *testing.T) {
	model := New()
	model.Show(KindError, "first")
	staleSeq := model.seq
	model.Show(KindInfo, "second")

	model, _ = model.Update(ExpiredMsg{Seq: staleSeq})
	if !model.Visible() {
		t.Fatal("Visible() = false, the stale timer wiped the newer toast")
	}
	if model.Message() != "second" {
		t.Errorf("Message() = %q, want %q", model.Message(), "second")
	}
}

func TestKeyPressClears(t *testing.T) {
	model := New()
	model.Show(KindInfo, "hello")

	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if model.Visible() {
		t.Error("Visible() = true after a key press, want false")
	}
}

func TestRenderIncludesMessage(t *testing.T) {
	model := New()
	model.Show(KindError, "boom")

	out := model.Render(80)
	if !strings.Contains(out, "boom") {
		t.Errorf("Render(80) = %q, want it to contain %q", out, "boom")
	}
}

// TestRenderTinyWidthDoesNotAutosize guards the clamp lower bound. A plain
// min(60, maxWidth-4) goes non-positive below width 5, and lipgloss treats
// Width(0) as "autosize to content", so the cap would silently vanish.
func TestRenderTinyWidthDoesNotAutosize(t *testing.T) {
	model := New()
	model.Show(KindError, strings.Repeat("wide ", 40))

	out := model.Render(2)
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 40 {
			t.Fatalf("Render(2) produced a %d-rune line, want the clamp to hold it near the 20-column floor", len([]rune(line)))
		}
	}
}

func TestRenderNotVisibleIsEmpty(t *testing.T) {
	model := New()
	if out := model.Render(80); out != "" {
		t.Errorf("Render on an invisible toast = %q, want empty", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/toast/...`
Expected: FAIL, the package does not exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/toast/toast.go`:

```go
// Package toast renders a short-lived, floating notification over the rest of
// the UI. It exists so error and confirmation messages never reach os.Stderr
// while the alternate screen is active: a raw stderr write lands on top of the
// rendered frame, and bubbletea has no idea those cells changed, so the
// corruption persists until the next full repaint.
package toast

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/misaelabanto/vibemux/internal/ui/styles"
)

// Kind selects a toast's severity styling.
type Kind int

const (
	// KindError styles the toast for a failure the user should act on.
	KindError Kind = iota
	// KindInfo styles the toast for a confirmation of something that worked.
	KindInfo
)

// lifetime is how long a toast stays up before dismissing itself.
const lifetime = 4 * time.Second

// Box width bounds. The lower bound is load-bearing: a plain
// min(maxBoxWidth, maxWidth-borderAndMargin) goes non-positive on a very
// narrow terminal, and lipgloss treats Width(0) as "size to content", which
// turns the cap into no cap at all.
const (
	minBoxWidth      = 20
	maxBoxWidth      = 60
	borderAndPadding = 4
)

// ExpiredMsg asks the model to clear the toast identified by Seq. A toast is
// cleared only when Seq matches the current one, so an older timer cannot wipe
// a toast raised after it was scheduled.
type ExpiredMsg struct {
	Seq int
}

// Model holds the single toast currently on screen, if any. One toast is shown
// at a time and a new one replaces the current one.
type Model struct {
	message string
	kind    Kind
	visible bool
	seq     int
}

// New builds an empty, invisible toast model.
func New() Model { return Model{} }

// Show replaces the current toast and returns the command that dismisses it
// after lifetime elapses.
//
// Callers MUST batch the returned command into their own return value. A
// dropped command leaves the toast up until the next key press.
func (m *Model) Show(kind Kind, text string) tea.Cmd {
	m.message = text
	m.kind = kind
	m.visible = true
	m.seq++

	scheduledSeq := m.seq
	return tea.Tick(lifetime, func(time.Time) tea.Msg {
		return ExpiredMsg{Seq: scheduledSeq}
	})
}

// Update clears the toast on its own expiry or on any key press.
//
// A key press dismisses the toast but is never reported as handled: the toast
// has no focus, so every binding underneath it keeps working.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ExpiredMsg:
		if msg.Seq == m.seq {
			m.visible = false
		}
	case tea.KeyPressMsg:
		m.visible = false
	}
	return m, nil
}

// Visible reports whether a toast is currently on screen.
func (m Model) Visible() bool { return m.visible }

// Message returns the raw, unwrapped toast text. Rendered output is wrapped
// and carries ANSI codes, so tests assert against this instead.
func (m Model) Message() string { return m.message }

// Render draws the toast box, sized to fit within maxWidth. Returns the empty
// string when no toast is visible.
func (m Model) Render(maxWidth int) string {
	if !m.visible {
		return ""
	}

	boxWidth := maxWidth - borderAndPadding
	if boxWidth < minBoxWidth {
		boxWidth = minBoxWidth
	}
	if boxWidth > maxBoxWidth {
		boxWidth = maxBoxWidth
	}

	borderColor := styles.Error.GetForeground()
	if m.kind == KindInfo {
		borderColor = styles.Muted.GetForeground()
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(boxWidth)

	text := lipgloss.NewStyle().Foreground(borderColor).Render(m.message)
	return box.Render(text)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/toast/... -v`
Expected: PASS, all seven tests.

If `TestKeyPressClears` fails to compile, check the `tea.KeyPressMsg` literal against the bubbletea v2 source at `~/go/pkg/mod/charm.land/bubbletea/v2@v2.0.0/key.go` and adjust the construction, not the assertion.

- [ ] **Step 5: Verify the whole build is still green**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
commita --no-push
```

---

### Task 2: Capture tmux stderr

**Files:**
- Modify: `internal/tmux/tmux.go:50-52`
- Test: `internal/tmux/tmux_test.go`

**Why:** The Task 5 pre-check names one failure cause. Every other tmux failure (server will not start, bad config, permissions) still returns a bare `exit status 1`, because `NewSession` calls `.Run()` and discards the child's stderr. zellij already captures it in `internal/zellij/zellij.go:85-95`; tmux gets the same treatment so a toast can say something real.

**Interfaces:**
- Produces: `run(cmd *exec.Cmd) error` unexported in package `tmux`. Same shape as zellij's.

- [ ] **Step 1: Write the failing test**

Append to `internal/tmux/tmux_test.go`:

```go
// TestRunWrapsStderr verifies that a failing command's stderr is folded into
// the returned error. Without this a caller can only report "exit status 1",
// which tells the user nothing about what actually went wrong.
func TestRunWrapsStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'no such directory' >&2; exit 1")

	err := run(cmd)
	if err == nil {
		t.Fatal("run returned nil error for a command that exited 1")
	}
	if !strings.Contains(err.Error(), "no such directory") {
		t.Errorf("err = %q, want it to contain the child's stderr", err.Error())
	}
}

// TestRunSucceedsQuietly verifies the helper does not invent an error.
func TestRunSucceedsQuietly(t *testing.T) {
	if err := run(exec.Command("true")); err != nil {
		t.Errorf("run(true) = %v, want nil", err)
	}
}
```

Ensure `internal/tmux/tmux_test.go` imports `os/exec` and `strings`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tmux/... -run TestRun`
Expected: FAIL, `undefined: run`.

- [ ] **Step 3: Add the helper and use it**

Add to `internal/tmux/tmux.go` (imports gain `fmt`; `strings` is already imported):

```go
// run executes cmd and folds any stderr it produced into the returned error.
// Bare exec errors are just "exit status 1", which is useless in a toast, so
// the child's own diagnostic is carried out to the caller.
func run(cmd *exec.Cmd) error {
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
```

Replace the body of `NewSession`:

```go
// NewSession creates a new detached tmux session with the given name and
// working directory.
func (Backend) NewSession(name, dir string) error {
	return run(exec.Command("tmux", "new-session", "-d", "-s", name, "-c", dir))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tmux/... -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole build is still green**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
commita --no-push
```

---

### Task 3: Let onboarding retry after a failed init

**Files:**
- Modify: `internal/ui/onboarding/onboarding.go`
- Test: `internal/ui/onboarding/onboarding_test.go`

**Why:** Task 6 replaces the `tea.Quit` at `update.go:124` with a toast and stays in onboarding. `Model.hasChosen` is sticky and has no reset, so without this every subsequent key press and tick would re-enter the `Chosen()` branch, re-call `mux.New`, and re-raise the toast forever.

**Interfaces:**
- Produces: `(*Model) ClearChoice()` in package `onboarding`.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/onboarding/onboarding_test.go`:

```go
// TestClearChoiceAllowsRetry verifies onboarding can be put back into a
// choosable state. AppModel needs this when building the chosen multiplexer
// fails: it stays in onboarding to show the error, and without a reset the
// sticky hasChosen flag would re-trigger the same failing path on every
// subsequent message.
func TestClearChoiceAllowsRetry(t *testing.T) {
	model := New([]mux.Kind{mux.Tmux, mux.Zellij})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if _, chosen := model.Chosen(); !chosen {
		t.Fatal("Chosen() reported false after pressing enter; test setup is wrong")
	}

	model.ClearChoice()

	if _, chosen := model.Chosen(); chosen {
		t.Error("Chosen() = true after ClearChoice, want false")
	}
}
```

Match the existing tests in this file for how they construct an enter key press and how they seed the model; copy their idiom rather than inventing one.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/onboarding/... -run TestClearChoice`
Expected: FAIL, `model.ClearChoice undefined`.

- [ ] **Step 3: Add the method**

Add to `internal/ui/onboarding/onboarding.go`, next to `Chosen`:

```go
// ClearChoice puts the model back into a choosable state. The caller uses this
// when it could not act on the chosen multiplexer: without it the sticky
// hasChosen flag makes every later message replay the same failing path.
func (m *Model) ClearChoice() {
	m.chosen = ""
	m.hasChosen = false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/onboarding/... -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole build is still green**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
commita --no-push
```

---

### Task 4: Composite the toast into the app view

**Files:**
- Modify: `internal/app/model.go` (struct field and `NewAppModel`)
- Modify: `internal/app/view.go`
- Modify: `internal/app/update.go` (forwarding only)
- Create: `internal/app/toast_test.go`

**Interfaces:**
- Consumes: everything Task 1 produces.
- Produces: `AppModel.toast` field, reachable from tests in package `app`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/toast_test.go`:

```go
package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/ui/toast"
)

// TestViewCompositesToastOverContent verifies the toast is actually drawn.
// lipgloss Layer.Draw renders only its own content and ignores children and
// X/Y, so composing a parent layer directly produces a frame with no toast in
// it at all. This test fails loudly if that mistake creeps back in.
func TestViewCompositesToastOverContent(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24
	model.toast.Show(toast.KindError, "kaboom")

	view := model.View()
	if !strings.Contains(view.Content, "kaboom") {
		t.Errorf("View content does not contain the toast message; got:\n%s", view.Content)
	}
}

// TestViewWithoutToastIsUnchanged verifies compositing is skipped entirely
// when nothing is showing, so the normal render path is untouched.
func TestViewWithoutToastIsUnchanged(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24

	if model.toast.Visible() {
		t.Fatal("a fresh AppModel must not have a visible toast")
	}
	if got := model.View().Content; strings.Contains(got, "╭") {
		t.Error("view contains a toast border with no toast showing")
	}
}

// TestToastSurvivesTheKeyPressThatRaisedIt is the regression guard for
// forwarding order. The toast clears on any key press, so if forwarding runs
// after the handler calls Show, every confirmation toast is destroyed by the
// exact keystroke that raised it and the user never sees one.
func TestToastSurvivesTheKeyPressThatRaisedIt(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24
	model.toast.Show(toast.KindInfo, "raised before the key")

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	appModel := updated.(AppModel)

	if appModel.toast.Visible() {
		t.Fatal("test premise is wrong: a key press should clear an already-showing toast")
	}
}

// TestToastClearsOnExpiry verifies the expiry message reaches the toast even
// though AppModel.Update's first type switch returns early for several
// message types. Forwarding must sit above that switch.
func TestToastClearsOnExpiry(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.toast.Show(toast.KindError, "temporary")
	seq := model.toast.CurrentSeq()

	updated, _ := model.Update(toast.ExpiredMsg{Seq: seq})
	if updated.(AppModel).toast.Visible() {
		t.Error("toast still visible after its ExpiredMsg reached AppModel.Update")
	}
}
```

This test file references `newFakeMux()`, built in Step 2, and `toast.CurrentSeq()`, added in Step 3.

- [ ] **Step 2: Create the fake multiplexer**

Create `internal/app/fakemux_test.go`:

```go
package app

import (
	"os/exec"
	"strings"
)

// fakeMux is a mux.Multiplexer that records what it was asked to do instead of
// shelling out. Tests assert on the recorded calls, which is how "the
// pre-check returned before touching the multiplexer" becomes observable.
type fakeMux struct {
	installed      bool
	sessions       map[string]bool
	newSessionErr  error
	killSessionErr error

	newSessionCalls  []string
	killSessionCalls []string
}

func newFakeMux() *fakeMux {
	return &fakeMux{installed: true, sessions: map[string]bool{}}
}

func (f *fakeMux) Name() string      { return "fakemux" }
func (f *fakeMux) IsInstalled() bool { return f.installed }

func (f *fakeMux) SessionName(projectPath string) string {
	trimmed := strings.TrimSuffix(projectPath, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	return "vmx-" + trimmed
}

func (f *fakeMux) HasSession(name string) bool { return f.sessions[name] }

func (f *fakeMux) NewSession(name, dir string) error {
	f.newSessionCalls = append(f.newSessionCalls, name)
	if f.newSessionErr != nil {
		return f.newSessionErr
	}
	f.sessions[name] = true
	return nil
}

func (f *fakeMux) AttachCommand(name string) *exec.Cmd { return exec.Command("true") }

func (f *fakeMux) KillSession(name string) error {
	f.killSessionCalls = append(f.killSessionCalls, name)
	if f.killSessionErr != nil {
		return f.killSessionErr
	}
	delete(f.sessions, name)
	return nil
}

func (f *fakeMux) ListVibemuxSessions() (map[string]bool, error) {
	live := make(map[string]bool, len(f.sessions))
	for name, ok := range f.sessions {
		live[name] = ok
	}
	return live, nil
}
```

- [ ] **Step 3: Expose the sequence for tests**

Add to `internal/ui/toast/toast.go`, next to `Message`:

```go
// CurrentSeq returns the sequence number of the current toast. It exists so
// callers and tests can construct the ExpiredMsg that matches it.
func (m Model) CurrentSeq() int { return m.seq }
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestView|TestToast'`
Expected: FAIL, `model.toast undefined`.

- [ ] **Step 5: Add the field**

In `internal/app/model.go`, add the import `"github.com/misaelabanto/vibemux/internal/ui/toast"`, add the field to `AppModel` after `onboarding`:

```go
	toast       toast.Model
```

and initialize it in `NewAppModel`, inside the `AppModel{...}` literal:

```go
		toast:       toast.New(),
```

- [ ] **Step 6: Composite in the view**

Rewrite `internal/app/view.go`'s `View` method. Add the import `"charm.land/lipgloss/v2"`:

```go
func (m AppModel) View() tea.View {
	var content string

	switch m.state {
	case ViewProjectList:
		content = m.projectList.View()
	case ViewAddProject:
		content = m.addProject.View()
	case ViewConsent:
		content = consentPrompt
	case ViewOnboarding:
		content = m.onboarding.View()
	}

	v := tea.NewView(m.withToast(content))
	v.AltScreen = true
	return v
}

// withToast composites the toast over content at the bottom right, leaving
// content itself untouched so nothing in the layout shifts.
//
// The compositing MUST go through lipgloss.NewCompositor. Layer.Draw renders
// only its own content string and ignores both its children and its own X/Y,
// so handing a parent layer straight to Canvas.Compose draws the base and
// silently discards the toast.
func (m AppModel) withToast(content string) string {
	if !m.toast.Visible() {
		return content
	}

	rendered := m.toast.Render(m.width)
	toastX := m.width - lipgloss.Width(rendered) - 2
	toastY := m.height - lipgloss.Height(rendered) - 2
	if toastX < 0 {
		toastX = 0
	}
	if toastY < 0 {
		toastY = 0
	}

	base := lipgloss.NewLayer(content)
	toastBox := lipgloss.NewLayer(rendered).X(toastX).Y(toastY).Z(1)

	return lipgloss.NewCanvas(m.width, m.height).
		Compose(lipgloss.NewCompositor(base, toastBox)).
		Render()
}
```

- [ ] **Step 7: Forward messages at the very top of Update**

In `internal/app/update.go`, insert at the start of `Update`, above the existing `switch msg := msg.(type)`:

```go
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forwarded before anything else, for two reasons. The switch below
	// returns early for several message types, so forwarding placed after it
	// would never deliver ExpiredMsg. And the toast clears on any key press:
	// if the keystroke reached the toast after a handler called Show, every
	// confirmation toast would be dismissed by the key that raised it.
	m.toast, _ = m.toast.Update(msg)

	switch msg := msg.(type) {
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/app/... -run 'TestView|TestToast' -v`
Expected: PASS, all four tests.

- [ ] **Step 9: Verify the whole build is still green**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
commita --no-push
```

---

### Task 5: The missing-directory pre-check

**Files:**
- Modify: `internal/app/update.go:249-282` (`openProject`)
- Test: `internal/app/toast_test.go`

**Interfaces:**
- Consumes: `AppModel.toast` (Task 4), `fakeMux` (Task 4).

- [ ] **Step 1: Write the failing tests**

Append to `internal/app/toast_test.go`:

```go
// TestOpenMissingDirectoryToastsAndStops verifies the pre-check names the
// cause and never lets the multiplexer try to chdir into a directory that is
// not there. That attempt is what produced the bare "exit status 1" written
// straight to a live alternate screen.
func TestOpenMissingDirectoryToastsAndStops(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	missing := filepath.Join(t.TempDir(), "deleted-project")
	project := model.Project{ID: "p1", Name: "deleted", Path: missing}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, _ := appModel.openProject(project)
	result := updated.(AppModel)

	if !result.toast.Visible() {
		t.Fatal("no toast raised for a missing Project directory")
	}
	if !strings.Contains(result.toast.Message(), missing) {
		t.Errorf("toast = %q, want it to name the missing path %q", result.toast.Message(), missing)
	}
	if len(fake.newSessionCalls) != 0 {
		t.Errorf("NewSession was called %v times, want 0: the pre-check must return first", len(fake.newSessionCalls))
	}
}

// TestOpenMissingDirectoryDoesNotTouchProject verifies a failed open does not
// bump the Project's last-used timestamp, which would reorder the dashboard
// to reward a Project that could not be opened.
func TestOpenMissingDirectoryDoesNotTouchProject(t *testing.T) {
	tempXDGDir(t)
	existing := t.TempDir()
	project, err := config.AddProject(existing)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if err := os.RemoveAll(existing); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	before, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}

	appModel := NewAppModel([]model.Project{project}, newFakeMux(), nil, "")
	appModel.openProject(project)

	after, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("expected exactly one Project before and after, got %d and %d", len(before), len(after))
	}
	if !before[0].LastUsed.Equal(after[0].LastUsed) {
		t.Errorf("LastUsed changed from %v to %v after a failed open", before[0].LastUsed, after[0].LastUsed)
	}
}

// TestOpenMissingDirectoryWithLiveSessionStillAttaches verifies the pre-check
// gates Session creation only. When a Session is already alive its process is
// running on the multiplexer server, and the user may need to reach whatever
// is inside it. Blocking that would prevent recovery without preventing any
// failure, since attaching does not touch the missing path.
func TestOpenMissingDirectoryWithLiveSessionStillAttaches(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	missing := filepath.Join(t.TempDir(), "deleted-project")
	project := model.Project{ID: "p1", Name: "deleted", Path: missing}
	fake.sessions[fake.SessionName(missing)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, cmd := appModel.openProject(project)

	if updated.(AppModel).toast.Visible() {
		t.Errorf("toast raised for a live Session: %q", updated.(AppModel).toast.Message())
	}
	if cmd == nil {
		t.Error("openProject returned a nil cmd, want the attach command")
	}
}

// TestOpenNotInstalledToasts covers the multiplexer-not-installed path that
// used to write to stderr.
func TestOpenNotInstalledToasts(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	fake.installed = false
	project := model.Project{ID: "p1", Name: "proj", Path: t.TempDir()}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, _ := appModel.openProject(project)

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no toast raised when the multiplexer is not installed")
	}
	if !strings.Contains(result.toast.Message(), "not installed") {
		t.Errorf("toast = %q, want it to mention the multiplexer is not installed", result.toast.Message())
	}
}
```

Add imports to `internal/app/toast_test.go`: `os`, `path/filepath`, plus `"github.com/misaelabanto/vibemux/internal/config"` and `"github.com/misaelabanto/vibemux/internal/model"`.

If `model.Project` has no `LastUsed` field, open `internal/model/project.go` and use whatever the timestamp field is actually called; do not add a field.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run TestOpen`
Expected: FAIL, no toast is raised and `NewSession` is called.

- [ ] **Step 3: Rewrite openProject**

Replace `openProject` in `internal/app/update.go`. Note `config.TouchProject` moves down: a failed open must not bump the Project's last-used timestamp. Add `"os"` to the imports if it is not already there.

```go
func (m AppModel) openProject(p model.Project) (tea.Model, tea.Cmd) {
	if !m.mux.IsInstalled() {
		return m, m.toast.Show(toast.KindError, fmt.Sprintf("%s is not installed", m.mux.Name()))
	}

	name := m.mux.SessionName(p.Path)

	// The directory is only checked when a Session has to be created. An
	// existing Session is already running on the multiplexer server and
	// attaching to it does not touch the path, so a Project whose directory
	// was deleted can still be reached to recover whatever is inside it.
	if !m.mux.HasSession(name) {
		if _, err := os.Stat(p.Path); err != nil {
			if os.IsNotExist(err) {
				return m, m.toast.Show(toast.KindError, fmt.Sprintf("Project directory not found: %s", p.Path))
			}
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Cannot open %s: %v", p.Path, err))
		}
		if err := m.mux.NewSession(name, p.Path); err != nil {
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not create session: %v", err))
		}
	}

	config.TouchProject(p.ID)

	execCmd := tea.ExecProcess(m.mux.AttachCommand(name), func(err error) tea.Msg {
		return MultiplexerReturnedMsg{Err: err}
	})

	if m.settings.FetchOnEnter {
		fetchCmd := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), gitstatus.FetchTimeout())
			defer cancel()
			_ = gitstatus.Fetch(ctx, p.Path)
			return FetchDoneMsg{ProjectID: p.ID}
		}
		return m, tea.Batch(execCmd, fetchCmd)
	}

	return m, execCmd
}
```

Add the toast import to `internal/app/update.go`:

```go
	"github.com/misaelabanto/vibemux/internal/ui/toast"
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -run TestOpen -v`
Expected: PASS, all four tests.

- [ ] **Step 5: Verify the whole build is still green**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
commita --no-push
```

---

### Task 6: Route the remaining errors and add confirmations

**Files:**
- Modify: `internal/app/update.go` (`updateOnboarding`, `updateConsent`, `updateProjectList`, `updateAddProject`, the `MultiplexerReturnedMsg` branch)
- Test: `internal/app/toast_test.go`

**Interfaces:**
- Consumes: `AppModel.toast`, `fakeMux`, `onboarding.ClearChoice` (Task 3).

- [ ] **Step 1: Write the failing tests**

Append to `internal/app/toast_test.go`:

```go
// TestMultiplexerReturnedErrorToasts verifies the error that used to be
// dropped on the floor now surfaces.
func TestMultiplexerReturnedErrorToasts(t *testing.T) {
	tempXDGDir(t)
	appModel := NewAppModel(nil, newFakeMux(), nil, "")

	updated, _ := appModel.Update(MultiplexerReturnedMsg{Err: errors.New("session died")})

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no toast raised for a non-nil MultiplexerReturnedMsg.Err")
	}
	if !strings.Contains(result.toast.Message(), "session died") {
		t.Errorf("toast = %q, want it to carry the underlying error", result.toast.Message())
	}
}

// TestMultiplexerReturnedCleanDetachIsSilent verifies a normal detach, which
// exits zero, does not raise anything.
func TestMultiplexerReturnedCleanDetachIsSilent(t *testing.T) {
	tempXDGDir(t)
	appModel := NewAppModel(nil, newFakeMux(), nil, "")

	updated, _ := appModel.Update(MultiplexerReturnedMsg{Err: nil})

	if updated.(AppModel).toast.Visible() {
		t.Error("a clean detach raised a toast")
	}
}

// TestKillSessionOnInactiveProjectIsSilent guards a regression this change
// would otherwise introduce. tmux kill-session on a nonexistent session exits
// 1, so attaching a toast to an unconditional KillSession would fire a useless
// "exit status 1" every time ctrl+x is pressed on an inactive Project, where
// today it is a silent no-op.
func TestKillSessionOnInactiveProjectIsSilent(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	fake.killSessionErr = errors.New("no server running")
	project := model.Project{ID: "p1", Name: "proj", Path: t.TempDir()}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 0 {
		t.Errorf("KillSession called %d times for an inactive Project, want 0", len(fake.killSessionCalls))
	}
	if result.toast.Visible() && strings.Contains(result.toast.Message(), "no server running") {
		t.Errorf("raised a useless backend error toast: %q", result.toast.Message())
	}
}

// TestKillSessionOnActiveProjectConfirms verifies the confirmation path.
func TestKillSessionOnActiveProjectConfirms(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project := model.Project{ID: "p1", Name: "proj", Path: dir}
	fake.sessions[fake.SessionName(dir)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 1 {
		t.Fatalf("KillSession called %d times, want 1", len(fake.killSessionCalls))
	}
	if !result.toast.Visible() {
		t.Error("no confirmation toast after killing a live Session")
	}
}
```

Add `errors` to the imports of `internal/app/toast_test.go`.

These tests drive `updateProjectList` directly, because the list sub-model owns selection. If the selected Project is not `p1` when the test runs, check how `projectlist.New` orders items and seed the model accordingly; do not weaken the assertion.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestMultiplexer|TestKillSession'`
Expected: FAIL, no toasts and `KillSession` called unconditionally.

- [ ] **Step 3: Route MultiplexerReturnedMsg**

In `internal/app/update.go`, in the `MultiplexerReturnedMsg` branch, immediately after `m.state = ViewProjectList` and before the `return`:

```go
		// Worded neutrally on purpose. A clean detach exits zero, but a Session
		// killed out from under an attached client, or a client closed after
		// another vibemux instance deleted the Session, surfaces a non-zero exit
		// on what the user experienced as an ordinary exit.
		var toastCmd tea.Cmd
		if msg.Err != nil {
			toastCmd = m.toast.Show(toast.KindError, fmt.Sprintf("Session exited: %v", msg.Err))
		}
		return m, tea.Batch(computeStatus(m.projects, m.mux), toastCmd)
```

Replace the existing `return m, computeStatus(m.projects, m.mux)` in that branch.

- [ ] **Step 4: Fix the onboarding failure path**

Replace the `mux.New` block in `updateOnboarding`. The multiplexer is now built before the settings are saved: persisting a Kind that fails to initialize means the next launch dies at `main.go:123` with `os.Exit(1)` and the exact stderr write this change exists to remove.

```go
	if k, ok := m.onboarding.Chosen(); ok {
		active, err := mux.New(k)
		if err != nil {
			// ClearChoice matters: hasChosen is sticky, so without resetting it
			// every later message would replay this same failing path.
			m.onboarding.ClearChoice()
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not initialize %s: %v", k, err))
		}

		// Load, modify, save so the status-display settings are preserved.
		s, _ := config.LoadSettings()
		s.Multiplexer = string(k)
		_ = config.SaveSettings(s)

		m.mux = active
		if needsConsent() {
			m.state = ViewConsent
		} else {
			m.state = ViewProjectList
		}
		m.projectList.SetSize(m.width, m.height)
		return m, tea.Batch(cmd, computeStatus(m.projects, m.mux), tick(m.settings))
	}
```

- [ ] **Step 5: Route the consent install**

In `updateConsent`, replace the `"y", "Y"` case:

```go
	case "y", "Y":
		m.state = ViewProjectList
		if err := hookinstall.Install("vibemux"); err != nil {
			return m, tea.Batch(
				computeStatus(m.projects, m.mux),
				m.toast.Show(toast.KindError, fmt.Sprintf("Could not install hooks: %v", err)),
			)
		}
		return m, tea.Batch(
			computeStatus(m.projects, m.mux),
			m.toast.Show(toast.KindInfo, "Agent status tracking enabled"),
		)
```

- [ ] **Step 6: Gate the kills and confirm them**

In `updateProjectList`, replace the `ctrl+d` and `ctrl+x` cases:

```go
			case "ctrl+d":
				if selected, ok := m.projectList.SelectedProject(); ok {
					// Gated on HasSession: kill-session against a session that is
					// not there exits non-zero on both backends, and reporting
					// that as a failure would be noise, not information.
					name := m.mux.SessionName(selected.Path)
					if m.mux.HasSession(name) {
						m.mux.KillSession(name)
					}
					if err := config.RemoveProject(selected.ID); err != nil {
						return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not remove Project: %v", err))
					}
					projects, _ := config.LoadProjects()
					projects = model.ProjectsUnder(projects, m.scopeDir)
					m.projects = projects
					setCmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(
						setCmd,
						computeStatus(m.projects, m.mux),
						m.toast.Show(toast.KindInfo, fmt.Sprintf("Removed %s", selected.Name)),
					)
				}
			case "ctrl+x":
				if selected, ok := m.projectList.SelectedProject(); ok {
					name := m.mux.SessionName(selected.Path)
					if !m.mux.HasSession(name) {
						return m, nil
					}
					if err := m.mux.KillSession(name); err != nil {
						return m, tea.Batch(
							computeStatus(m.projects, m.mux),
							m.toast.Show(toast.KindError, fmt.Sprintf("Could not kill session: %v", err)),
						)
					}
					return m, tea.Batch(
						computeStatus(m.projects, m.mux),
						m.toast.Show(toast.KindInfo, fmt.Sprintf("Killed session %s", name)),
					)
				}
```

The `RemoveProject` error now short-circuits: previously the Project was dropped from the in-memory slice regardless, so a failed removal left it gone from the dashboard but still in projects.json, back at the next launch.

- [ ] **Step 7: Route the add-project failure and confirmation**

In `updateAddProject`, replace the `SelectedPath` block:

```go
	if path := m.addProject.SelectedPath(); path != "" {
		m.addProject.ClearSelection()
		m.state = ViewProjectList
		added, err := config.AddProject(path)
		if err != nil {
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not add Project: %v", err))
		}
		m.projects = append(m.projects, added)
		setCmd := m.projectList.SetProjects(m.projects)
		return m, tea.Batch(setCmd, m.toast.Show(toast.KindInfo, fmt.Sprintf("Added %s", added.Name)))
	}
```

- [ ] **Step 8: Confirm no stderr writes remain in the TUI**

Run: `grep -n "os.Stderr" internal/app/update.go`
Expected: no output. The remaining `os.Stderr` references in `internal/tmux/tmux.go:62` and `internal/zellij/zellij.go:208` are correct and stay: `ExecProcess` releases the terminal while the child runs, so nothing is rendering when the child owns stderr.

Also confirm `fmt` and `os` are still both used in `internal/app/update.go`, and drop either import if it is not.

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS, including every pre-existing test in the package.

- [ ] **Step 10: Verify the whole build is still green**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
commita --no-push
```

---

### Task 7: Manual verification

**Files:** none.

**Why:** Every prior task asserts against rendered strings. None of them prove a toast actually looks right in a real terminal, and the original bug was purely visual.

- [ ] **Step 1: Build**

Run: `go build -o main .`

- [ ] **Step 2: Reproduce the original bug scenario**

Register a Project pointing at a directory, delete the directory, then run `./main` and press enter on that Project.

Expected: a bordered red box in the bottom right reading `Project directory not found: <path>`, the list rendering intact behind it with no torn lines, and the box disappearing on its own after about 4 seconds.

- [ ] **Step 3: Check a confirmation toast**

Open a Project, detach, then press `ctrl+x` on it.

Expected: a muted box reading `Killed session vmx-<dir>`. Press `ctrl+x` again on the now-inactive Project: expected nothing at all, no error box.

- [ ] **Step 4: Check the toast under a resize**

Raise a toast, then resize the terminal narrower while it is up.

Expected: the box repositions to stay in view, and no rendering artifacts appear. On a very narrow terminal it may clip at the left edge; it must not corrupt the frame.

- [ ] **Step 5: Report findings**

If anything renders wrong, stop and report rather than patching over it. A visual defect here means the compositing is wrong, not the styling.

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| toast sub-model, sequence guard, dismissal, width clamp, `Message` | 1 |
| `Compositor` requirement | 4 (view) and constrained globally |
| forwarding order at top of `Update` | 4 |
| toast composes over every view state | 4 (single `withToast` on the shared path) |
| error routing table, all five sites | 5 (two sites) and 6 (three sites) |
| onboarding: build before save, `ClearChoice` | 3 and 6 |
| confirmations table | 6 |
| `ctrl+x` HasSession gate | 6 |
| `ctrl+d` RemoveProject error | 6 |
| errors that stay silent | untouched by design, no task needed |
| missing-directory pre-check, `TouchProject` move | 5 |
| tmux stderr capture | 2 |
| every listed test | 1, 2, 3, 4, 5, 6 |

No spec requirement is unassigned.

**Placeholder scan:** No TBD, TODO, "similar to Task N", or "add appropriate error handling". Every code step carries the actual code.

**Type consistency:** `toast.Model`, `toast.Kind`, `toast.KindError`, `toast.KindInfo`, `toast.ExpiredMsg{Seq}`, `Show`, `Update`, `Visible`, `Message`, `CurrentSeq`, `Render` are used identically in tasks 1, 4, 5, and 6. `fakeMux` fields (`installed`, `sessions`, `newSessionErr`, `killSessionErr`, `newSessionCalls`, `killSessionCalls`) are defined in task 4 and used in tasks 5 and 6 with matching names. `onboarding.ClearChoice` is defined in task 3 and called in task 6.

**Note for the implementer:** `CurrentSeq` was added in Task 4 Step 3 rather than Task 1 because that is where the first caller appears. If you implement Task 1 and Task 4 together, fold it into Task 1.
