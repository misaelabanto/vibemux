# vibemux

A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent tmux or zellij sessions for your projects: detach without killing, reattach to resume exactly where you left off.

## What it does

`vibemux` lets you:
- **Register project directories** in a persistent config: pick an existing folder, create an empty one, or clone a GitHub repo
- **Open multiplexer sessions** (tmux or zellij) for projects with a keystroke
- **Detach and switch** between projects without terminating shells
- **Reattach seamlessly** to running sessions with full history and state preserved
- **See active sessions** at a glance with visual indicators (`●`) in the project list
- **Filter to active-only** to focus on what's currently running
- **Type-to-filter** instantly by project name, or jump to a project by its index number

Each project gets its own session in your chosen multiplexer (tmux or zellij) that persists even after you quit vibemux. Sessions are plain multiplexer sessions, so you can use any features, plugins, or keybindings you're familiar with. On first run vibemux detects which multiplexers are installed and lets you pick one (see "First run" below).

## Installation

### Install via `go install`

```bash
go install github.com/misaelabanto/vibemux@latest
```

Then run:
```bash
vibemux
```

### Build from source

```bash
git clone https://github.com/misaelabanto/vibemux.git
cd vibemux
go build -o vibemux .
./vibemux
```

### Requirements

- **tmux** or **zellij** (at least one): vibemux uses your chosen multiplexer for terminal emulation. zellij is resolved from `$PATH` or `~/.local/bin`; tmux from `$PATH`. zellij 0.44+ is recommended (tested against 0.44.3).
- Go 1.24+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)

## Usage

### Project list (startup screen)

```
enter          Open selected project (attach multiplexer session)
type           Filter projects by name (any printable character starts filtering)
1-9 / digits   Quick-select project by its index number (e.g. type "12" → project #12)
ctrl+a         Toggle "active only" view (show only projects with running sessions)
ctrl+n         Add a new project (opens add-project menu)
ctrl+d         Delete project from config (kills session if active)
ctrl+x         Kill the multiplexer session for selected project
ctrl+c         Quit vibemux (multiplexer sessions stay alive)
```

Notes:
- The `●` indicator next to a project name shows that an active multiplexer session exists for it.
- Projects are sorted by their filesystem path and shown with an index prefix (`1.`, `2.`, …) for quick-select.
- While typing a filter, the top match stays highlighted so `enter` opens what you'd expect.

### Adding a project

Press `ctrl+n` to open the add-project menu:

```
Pick existing folder    Browse the filesystem and register an existing directory
Create empty folder     Create a new empty directory under a chosen parent
Clone GitHub repo       Clone a remote repo into a chosen parent directory
```

When cloning, vibemux normalizes the URL. GitHub HTTPS URLs are rewritten to SSH (`git@github.com:owner/repo.git`) so authentication uses your SSH key. The repository directory name is extracted automatically.

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

### Inside a session

Once attached, you have full control of your multiplexer. The bindings below
are zellij's defaults; tmux uses its own (`Ctrl+b` prefix). Common zellij
bindings:

```
Ctrl+o d       Detach from session (return to vibemux)
Ctrl+p n       Open a new pane
Ctrl+p x       Close the focused pane
```

zellij shows its keybinding hints in the on-screen status bar. See the zellij
documentation for the full reference.

### Session behavior

- **Detach** (`Ctrl+o d`): Returns you to the vibemux project list. The shell process continues running in the background with full state preserved.
- **Reattach**: Select the same project again to reconnect to the background session. Everything you left running is still there.
- **Kill** (`ctrl+x` from project list): Terminates the multiplexer session. Reopening the project starts a fresh session.
- **Quit** (`ctrl+c` from project list): Exits vibemux. All multiplexer sessions persist and can be reattached later.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/app` | Root TUI model, state machine, message routing |
| `internal/config` | XDG-compliant config store for projects (`projects.json`) and settings (`settings.json`, the chosen multiplexer) |
| `internal/model` | Project data structure |
| `internal/mux` | Multiplexer abstraction: the `Multiplexer` interface, `Kind` enum, registry (`New`, `Installed`), and resolution (`Active`) |
| `internal/tmux` | tmux backend: create, attach, kill, list sessions, exact name matching so `vmx-foo` doesn't collide with `vmx-foo-bar` |
| `internal/zellij` | zellij backend: same surface as tmux, resolving the binary from `$PATH` or `~/.local/bin` and dropping EXITED corpses on kill |
| `internal/ui/onboarding` | First-run / self-heal flow that detects installed multiplexers and selects the active one |
| `internal/gitops` | Git URL normalization (HTTPS → SSH for GitHub) and `git clone` invocation |
| `internal/ui/projectlist` | Project list view with type-to-filter, digit quick-select, and active-only toggle |
| `internal/ui/addproject` | Add-project flow: pick existing / create empty / clone GitHub repo |

### How it works

vibemux is a **project launcher** that delegates terminal emulation to your chosen multiplexer (tmux or zellij):

```
┌─ vibemux (main app)
│  ├─ Project list with session status, type-to-filter, digit quick-select
│  └─ Key handler
│     ├─ "enter"  → multiplexer attach (hands terminal control to it)
│     ├─ "ctrl+x" → multiplexer kill-session
│     ├─ "ctrl+d" → config.RemoveProject() (+ kill session if active)
│     ├─ "ctrl+n" → open add-project menu (pick / create / clone)
│     └─ "ctrl+a" → toggle active-only filter
│
└─ multiplexer (subprocess, full terminal control)
   ├─ Session: vmx-<project-path-slug>
   │  └─ Shell with full history and process state
   └─ Detach (zellij Ctrl+o d / tmux Ctrl+b d) → returns control to vibemux
```

When you open a project, vibemux creates a session in your chosen multiplexer (if one doesn't already exist) and uses `tea.ExecProcess()` to hand terminal control to it. The session persists in the background even after you detach or quit vibemux. Session names are derived from the project path (prefixed `vmx-`) and matched exactly against the live session list, so similarly-named sessions don't collide.

### Session persistence

Sessions are managed by your multiplexer's background server processes, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `tmux list-sessions` or `zellij list-sessions`
- You can manually attach with `tmux attach -t vmx-<slug>` or `zellij attach vmx-<slug>`
- Sessions can accumulate if not cleaned up (use `ctrl+x` or `ctrl+d` in vibemux to kill them)

## Dependencies

### Direct

| Package | Why |
|---------|-----|
| `charm.land/bubbletea/v2` | TUI event loop: state machine, input handling, render cycle |
| `charm.land/bubbles/v2` | TUI components: `list` (project picker), `filepicker` (add project), `textinput` (folder name / repo URL), `spinner` (clone progress) |
| `charm.land/lipgloss/v2` | Styling for UI elements (ANSI colors, layouts) |
| `github.com/charmbracelet/x/ansi` | ANSI-safe string truncation in the highlight-while-filtering delegate |
| `github.com/google/uuid` | Project ID generation (stable, collision-free) |
| `git` (binary) | Invoked by `internal/gitops` to clone repositories |

### Requirements

- **tmux** (on `$PATH`) or **zellij** (on `$PATH` or in `~/.local/bin`), at least one. zellij 0.44+ recommended.
- Go 1.24+
- A POSIX shell

## Project storage

Projects are stored in `~/.config/vibemux/projects.json` (XDG Base Directory compliant):

```json
[
  {
    "id": "uuid-here",
    "name": "my-project",
    "path": "/path/to/project",
    "lastOpened": "2025-02-27T12:34:56Z"
  }
]
```

The chosen multiplexer is stored separately in `~/.config/vibemux/settings.json`:

```json
{
  "multiplexer": "zellij"
}
```

## Development

### Running from source
```bash
go run .
```

### Building a release
```bash
go build -o vibemux .
./vibemux --version  # Not yet implemented
```

### Testing
```bash
go test ./...
```
