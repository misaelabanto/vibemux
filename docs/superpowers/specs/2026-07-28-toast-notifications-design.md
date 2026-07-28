# Toast notifications

## Problem

Every error path inside the running TUI writes to `os.Stderr` with `fmt.Fprintf`. The program runs in the alternate screen buffer, so that raw text lands on top of the rendered frame and corrupts the layout. Bubbletea has no idea the cells changed, so the damage persists until the next full repaint.

Opening a Project whose directory no longer exists is the reproducer. `tmux new-session -c <missing dir>` fails, `internal/app/update.go:262` prints `Error creating tmux session: exit status 1` over the dashboard, and the list is left with broken lines.

Two problems compound here. The delivery mechanism is wrong, and the message itself is useless: `exit status 1` says nothing about the directory being gone.

Four sites are affected, plus one error that is dropped on the floor:

| Site | Current behavior |
|---|---|
| `update.go:262` session creation failed | stderr write, screen corrupted |
| `update.go:253` multiplexer not installed | stderr write, screen corrupted |
| `update.go:236` add Project failed | stderr write, screen corrupted |
| `update.go:124` multiplexer init failed | stderr write, then quits |
| `MultiplexerReturnedMsg.Err` | silently discarded |

`main.go` also writes to stderr, but the TUI is not running at those points, so those writes are correct and stay as they are.

The multiplexer backends set `cmd.Stderr = os.Stderr` on their attach commands (`internal/tmux/tmux.go:62`, `internal/zellij/zellij.go:208`). This is **not** a corruption source: bubbletea's `ExecProcess` releases the terminal for the duration of the child process, so nothing is rendering while the child owns stderr. Those lines stay as they are.

## Solution

A floating toast rendered as an overlay on top of the existing view. Errors and confirmations both flow through it. The base view is never modified, so nothing shifts and nothing tears.

### Rendering approach

lipgloss v2 (already a dependency) provides `Canvas`, `Layer`, and `Compositor`. The view content is composed as the base layer and the toast box as a layer above it at an explicit x/y. This keeps the toast out of the layout entirely.

The two alternatives were rejected: a reserved bottom status line permanently steals a row and resizes the list every time a message appears, and manual string splicing requires ANSI-aware width math on styled rows, which is the same class of problem as the bug being fixed.

**`Compositor` is required, not optional.** `Layer.Draw` (`lipgloss/v2@v2.0.0/layer.go:153`) renders only its own content string and ignores both its children and its own X/Y/Z. Only `Compositor` flattens the hierarchy and draws each layer at its absolute position. Passing a parent layer straight to `Canvas.Compose` compiles cleanly and silently discards the toast.

## Components

### `internal/ui/toast`

A self-contained bubbletea sub-model, matching the shape of the other `internal/ui/*` packages.

```go
type Kind int

const (
    KindError Kind = iota
    KindInfo
)

type Model struct { ... }

func New() Model
func (m *Model) Show(kind Kind, text string) tea.Cmd
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)
func (m Model) Visible() bool
func (m Model) Message() string
func (m Model) Render(maxWidth int) string
```

`Show` sets the current toast and returns a `tea.Tick` command that emits an expiry message after 4 seconds. It has a pointer receiver while the rest of the model is updated by value; `m.toast` is addressable inside `AppModel` methods so this works, but call sites must call `m.toast.Show(...)` directly rather than on a copy.

**Every call site must batch the returned command into its return value.** A dropped command means the toast never expires and lingers until the next keypress.

**Sequence guard.** The model holds a monotonic counter. Each `Show` increments it, and the expiry message carries the value current at the time it was scheduled. `Update` clears the toast only when the incoming sequence matches the current one. Without this, a toast raised 3.9 seconds after another would be wiped 0.1 seconds later by the older timer.

**Dismissal without consumption.** `Update` clears the toast on any `tea.KeyPressMsg` but never reports the key as handled. The toast has no focus and no state machine, so `enter`, filter typing, and every other binding continue to work underneath it.

**Single slot.** One toast is shown at a time and a new one replaces the current one. A queue is more machinery than this number of sites warrants.

`Message` returns the raw, unwrapped text. It exists for tests: rendered output is wrapped and carries ANSI codes, so a long path never appears contiguously in it.

`Render` produces a rounded-border box. `KindError` uses `styles.Error`, `KindInfo` uses `styles.Muted`. Box width is `clamp(maxWidth-4, 20, 60)`: the lower bound matters because `min(60, maxWidth-4)` goes non-positive below width 5, and `Style.Width(0)` autosizes to the full content width rather than capping. Below a 24-column terminal the box is wider than the canvas; the canvas clips it without panicking. The message wraps within the box.

### `internal/app` compositing

`AppModel` gains a `toast toast.Model` field, initialized in `NewAppModel`.

**Message forwarding runs at the very top of `Update`, before both existing type switches.** Ordering is load-bearing in two ways:

- `Update`'s first switch returns early for `MultiplexerReturnedMsg`, `StatusComputedMsg`, `TickMsg`, `FetchDoneMsg`, and `WindowSizeMsg`. Forwarding placed between the switches would never see them, so the expiry message would never arrive.
- The toast clears on any `tea.KeyPressMsg`. If the `ctrl+x` / `ctrl+d` / `enter` keypress reached `toast.Update` *after* the handler called `Show`, every confirmation toast would be dismissed by the exact keystroke that raised it. Dismiss first, then show.

`View` builds `content` exactly as it does today. When the toast is visible it then composes:

```go
base := lipgloss.NewLayer(content)
box := lipgloss.NewLayer(m.toast.Render(m.width)).X(x).Y(y).Z(1)
content = lipgloss.NewCanvas(m.width, m.height).
    Compose(lipgloss.NewCompositor(base, box)).
    Render()
```

Position is bottom-right with a 1-cell margin: `x = width - boxWidth - 2`, `y = height - boxHeight - 2`. Both are clamped to a minimum of 0, so a terminal too small for the box degrades to top-left. Out-of-bounds and negative layer positions clip safely in lipgloss rather than panicking.

The toast composes over every view state, including `ViewOnboarding`, `ViewAddProject`, and `ViewConsent`. Before the first `WindowSizeMsg`, `m.width`/`m.height` hold the `defaultWidth`/`defaultHeight` seeds, which are valid canvas dimensions, so a toast raised that early still renders.

## Error routing

Each in-TUI stderr write is replaced by a toast.

| Site | After |
|---|---|
| `update.go:262` session creation failed | error: `Could not create session: <err>` |
| `update.go:253` multiplexer not installed | error: `<name> is not installed` |
| `update.go:236` add Project failed | error: `Could not add Project: <err>` |
| `update.go:124` multiplexer init failed | error: `Could not initialize <kind>: <err>`, stay in onboarding |
| `MultiplexerReturnedMsg.Err` non-nil | error: `Session exited: <err>` |

`MultiplexerReturnedMsg.Err` is worded neutrally on purpose. A clean detach exits 0, but a Session killed out from under an attached client, or a client closed after another vibemux instance deleted the Session, can surface a non-zero exit on what the user experienced as a normal exit. "Session exited" is accurate in both readings.

### Recovering from a failed multiplexer init

`update.go:116-135` currently saves the chosen Kind to settings and *then* builds the multiplexer. Two changes are needed before the quit can be replaced with a toast:

1. **Build before saving.** `mux.New` runs first; settings are saved only after it succeeds. Otherwise a Kind that fails to initialize is persisted, and the next launch dies at `main.go:123` with `os.Exit(1)` and the exact stderr write this spec exists to remove.
2. **Reset the onboarding choice.** `onboarding.Model.hasChosen` is sticky and has no reset. Staying in `ViewOnboarding` after a failure means every subsequent keypress and tick re-enters the `Chosen()` branch, re-calls `mux.New`, and re-raises the toast forever. A `ClearChoice()` method is added to the onboarding model and called on failure.

Worth noting: `mux.New` (`internal/mux/registry.go`) only errors on an unrecognized Kind, and onboarding only offers Kinds from the installed set, so this branch is effectively unreachable today. The two changes are cheap and keep it from becoming a live infinite loop if a backend later grows a fallible constructor.

### Confirmations

| Action | Toast |
|---|---|
| `ctrl+x` kill Session | info: `Killed session <name>`; error `Could not kill session: <err>` on failure |
| `ctrl+d` delete Project | info: `Removed <name>`; error `Could not remove Project: <err>` on failure |
| add Project succeeded | info: `Added <name>` |
| consent prompt accepted | info: `Agent status tracking enabled`; error `Could not install hooks: <err>` on failure |

Two existing bugs have to be fixed for these to tell the truth:

- **`ctrl+x` on an inactive Project.** `updateProjectList` (`update.go:198-202`) calls `KillSession` unconditionally, and `tmux kill-session` on a nonexistent session exits 1 (verified: `no server running on ...`, exit 1). Today that is a silent no-op; with a toast attached it would fire a useless `exit status 1` error on every kill of an inactive Project. `ctrl+x` gates on `HasSession` first and does nothing when there is no Session to kill.
- **`ctrl+d` drops two errors.** `config.RemoveProject` (`update.go:191`) returns an error that is discarded, and its `KillSession` call is unchecked. On a `RemoveProject` failure the Project vanishes from the in-memory slice but stays in projects.json and returns at next launch, while an info toast claims success. `RemoveProject`'s error is routed to an error toast and the in-memory removal is skipped. Its `KillSession` is gated on `HasSession` like `ctrl+x`.

### Errors that stay silent

Deliberately not toasted, listed so the omission reads as a decision rather than an oversight: `setHooksDeclined` (`update.go:154`), `config.SaveSettings` in onboarding, `config.TouchProject`, and the `ListVibemuxSessions` / `agent.LoadAll` errors in `sweepStatus`. The sweep runs every 3 seconds, so toasting from it would produce a permanent toast.

## Missing-directory pre-check

**The check gates Session creation only.** When a Session already exists for a Project whose directory was deleted, attach still proceeds: the Session is alive on the multiplexer server and the user may need to get at what is running inside it. Blocking that would prevent recovery without preventing any failure, since attaching to a live Session does not touch the missing path.

So the stat happens inside the branch that creates a Session, and `openProject` becomes:

1. Return an error toast if the multiplexer is not installed.
2. Resolve the Session name.
3. If no Session exists yet:
   - Stat `p.Path`. Not found: raise `Project directory not found: <path>` and return. Stat failed for another reason: raise `Cannot open <path>: <err>` and return.
   - Create the Session. Failure raises an error toast.
4. `config.TouchProject(p.ID)`.
5. Attach.

`config.TouchProject` moves from the top of the function to step 4, so a failed open does not bump the Project's last-used timestamp.

### Capturing tmux stderr

The pre-check turns one failure mode into a real sentence. Every other tmux failure (server will not start, bad config, permissions) still yields `exit status 1`, because `tmux.Backend.NewSession` (`internal/tmux/tmux.go:50-52`) calls bare `.Run()` and discards the child's stderr.

zellij already solves this: its `run` helper (`internal/zellij/zellij.go:85-95`) captures stderr and wraps it into the returned error. The tmux package gets the same helper, and `NewSession` uses it. Without this, the "useful messages" half of this change only holds for one failure mode of one backend.

## Testing

### `internal/ui/toast`

- `Show` makes the model visible and returns a non-nil command.
- An expiry message whose sequence matches the current one clears the toast.
- An expiry message carrying a stale sequence leaves a newer toast visible. This is the regression guard for the sequence counter.
- A key press clears the toast.
- `Render` includes short message text and stays within the width cap.
- `Render` at a very small `maxWidth` does not panic and does not autosize past the cap. This guards the `clamp` lower bound.

### `internal/app`

These tests need a fake `mux.Multiplexer`. None exists yet, so one is added as a test helper recording which methods were called. The interface is eight small methods and `AttachCommand` can return `exec.Command("true")`. `NewAppModel(projects, fake, nil, "")` injects it directly, and the tests live in package `app`, so no production seam is needed.

- Opening a Project whose path does not exist raises an error toast naming the path and never calls `NewSession`.
- Opening a Project whose path does not exist does not call `config.TouchProject`.
- Opening a Project whose path exists and whose Session exists proceeds to attach, confirming the pre-check does not gate the recovery case.
- `MultiplexerReturnedMsg` carrying a non-nil error raises an error toast.
- A confirmation toast raised by a keypress survives that keypress. This is the regression guard for the forwarding order.
- `View` output contains the toast box when a toast is visible, and the base content is still present. Assertions use short fragments or `Message()`, never a long path against wrapped output.

Path assertions use `Message()` rather than rendered output: macOS `t.TempDir()` paths run past 50 characters, so they wrap mid-path inside the box.

No existing test asserts on `View` output or on the quit at `update.go:124`, so nothing currently passing breaks. New tests that write config use the existing `tempXDGDir` helper, which sandboxes both `XDG_RUNTIME_DIR` and `XDG_CONFIG_HOME`.
