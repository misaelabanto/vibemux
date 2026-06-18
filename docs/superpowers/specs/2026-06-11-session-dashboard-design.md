# Session dashboard (ctrl+o)

Date: 2026-06-11
Status: approved

## Goal

Let the user see every active vibemux session at once, as live tiled panes in a
single tmux window, for focus and faster context switching.

## Behavior

- From the project list, `ctrl+o` opens a tmux session named `vmx-dashboard`.
- The dashboard contains one pane per active `vmx-*` session (the dashboard
  itself excluded), arranged with tmux's `tiled` layout, sorted alphabetically
  by session name for stable pane order.
- Each pane runs a nested tmux client attached to its session. Typing into a
  focused pane interacts with that session normally.
- The dashboard is rebuilt from scratch on every `ctrl+o`: any existing
  `vmx-dashboard` session is killed first, so the grid always reflects the
  currently active sessions.
- If no `vmx-*` sessions are active, stay in the project list and show a brief
  status message: "no active sessions".
- `Ctrl+b d` inside the dashboard detaches it and returns to the vibemux
  project list, the same as detaching from any project session.
- When an inner session dies, its nested client exits and the pane closes
  automatically (the pane command uses `exec`), so dead sessions self-clean.

## Architecture

### tmux layer (`internal/tmux/tmux.go`)

- `DashboardSession = "vmx-dashboard"` exported constant.
- `nestedAttachCommand(name string) string`: returns the shell command string a
  dashboard pane runs, in the form `TMUX= exec tmux attach-session -t '=NAME'`.
  Clearing `TMUX` permits nesting; `exec` ties the pane lifetime to the inner
  client; the `=` prefix keeps exact-name matching consistent with the rest of
  the package.
- `DashboardSessions(active map[string]bool) []string`: pure helper that takes
  the active-session set, drops `vmx-dashboard`, and returns the rest sorted
  alphabetically. This is the testable self-exclusion logic.
- `BuildDashboard(sessions []string) error` (panes in the order given):
  1. `tmux new-session -d -s vmx-dashboard <nestedAttachCommand(first)>`
  2. For each remaining session: `tmux split-window -t =vmx-dashboard:
     <nestedAttachCommand(name)>`, retiling after each split so panes never
     get too small to split again. The trailing `:` matters: split-window
     and select-layout take a pane target, where a bare `=name` only
     resolves as a session; `=name:` means "exact session, current window".
  3. `tmux select-layout -t =vmx-dashboard: tiled`
  4. On any error, kill the half-built `vmx-dashboard` and return the error.

### App layer (`internal/app/update.go`)

- New `ctrl+o` case in `updateProjectList` (non-filtering branch) calling
  `openDashboard()`.
- `openDashboard()`:
  1. `tmux.ListVibemuxSessions()`, drop `vmx-dashboard` from the result.
  2. Empty: return a status-message Cmd ("no active sessions"), stay in list.
  3. Kill any existing `vmx-dashboard` session, then call
     `tmux.BuildDashboard(...)`; on error print to stderr (matching existing
     tmux error handling) and stay in the list.
  4. Attach via `tea.ExecProcess(tmux.AttachCommand(...))` returning
     `TmuxReturnedMsg`, reusing the existing return-and-refresh path.
- The project list `●` indicators are unaffected: `mapSessionsToProjects` only
  maps sessions of registered projects, and `vmx-dashboard` matches none.

### UI layer (`internal/ui/projectlist`)

- Add a `StatusMessage(s string) tea.Cmd` passthrough on the projectlist Model
  that calls the wrapped bubbles `list.Model.NewStatusMessage`, used for the
  empty case.

## Edge cases and errors

- Pane too small: `split-window` fails on tiny terminals. `BuildDashboard`
  kills the partial dashboard and returns the error; the app prints it to
  stderr and remains in the project list.
- The dashboard is not a project, so `ctrl+x`/`ctrl+d` cannot kill it from the
  list. A leftover dashboard is harmless: the next `ctrl+o` kills and rebuilds
  it, and it can also be killed from inside tmux (`Ctrl+b :kill-session`).
- Nested prefix: `Ctrl+b` targets the dashboard session; `Ctrl+b Ctrl+b`
  targets the inner session under the focused pane. Documented in README.

## Testing

- Unit tests alongside the existing `tmux_test.go` style:
  - `nestedAttachCommand`: clears `TMUX`, uses `exec`, quotes the exact-match
    target.
  - Dashboard self-exclusion: filtering `vmx-dashboard` out of a session set.
  - `BuildDashboard` argument construction where feasible without a live tmux
    server.
- Verification: `go test ./...` and `go build -o vibemux .`.

## Documentation

- README: add `ctrl+o` to the project list key table.
- README: short "Dashboard" subsection covering the nested-prefix rule and the
  rebuilt-on-every-open lifecycle.
