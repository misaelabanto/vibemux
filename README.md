# vibemux

A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent zellij sessions for your projects: detach without killing, reattach to resume exactly where you left off.

## What it does

`vibemux` lets you:
- **Register project directories** in a persistent config: pick an existing folder, create an empty one, or clone a GitHub repo
- **Open zellij sessions** for projects with a keystroke
- **Detach and switch** between projects without terminating shells
- **Reattach seamlessly** to running sessions with full history and state preserved
- **See active sessions** at a glance with visual indicators (`●`) in the project list
- **Filter to active-only** to focus on what's currently running
- **Type-to-filter** instantly by project name, or jump to a project by its index number

Each project gets its own zellij session that persists even after you quit vibemux. Sessions are plain zellij sessions, so you can use any zellij features, plugins, or keybindings you're familiar with.

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

- **zellij** 0.43+ (required: vibemux uses zellij for terminal emulation, and the web dashboard needs the zellij web client introduced in 0.43; 0.44+ recommended, tested against 0.44.3)
- Go 1.21+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)

## Usage

### Project list (startup screen)

```
enter          Open selected project (attach zellij session)
type           Filter projects by name (any printable character starts filtering)
1-9 / digits   Quick-select project by its index number (e.g. type "12" → project #12)
ctrl+a         Toggle "active only" view (show only projects with running sessions)
ctrl+o         Open the web dashboard (one browser window per active session)
ctrl+n         Add a new project (opens add-project menu)
ctrl+d         Delete project from config (kills session if active)
ctrl+x         Kill zellij session for selected project
ctrl+c         Quit vibemux (zellij sessions stay alive)
```

Notes:
- The `●` indicator next to a project name shows that an active zellij session exists for it.
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

### Dashboard (ctrl+o)

Press `ctrl+o` in the project list to open the web dashboard: vibemux starts
a local zellij web server (`127.0.0.1:8082`) and opens one browser window per
active vibemux session, a mission-control view for watching everything at
once and switching context fast. The TUI stays in the project list.

- On first use, zellij creates a login token and vibemux shows it once in a
  status message (it is also printed to stderr). Paste it into the browser
  login prompt. zellij stores only token hashes, so it cannot be shown again;
  if you lose it, create a new one with `zellij web --create-token`.
- Each browser window is a full zellij client attached to one session. Typing
  into it interacts with that session normally.
- The web server listens on 127.0.0.1 only and keeps running after vibemux
  exits. Stop it with `zellij web --stop`.
- Sessions created by older vibemux versions (including the tmux-based ones)
  are not web-shared and won't open in the browser. Kill them (`ctrl+x`) and
  reopen the project to recreate them with web sharing enabled.
- If no sessions are active, vibemux shows "no active sessions" and stays in
  the project list.

The web dashboard requires zellij 0.43+ (0.44+ recommended; tested against
0.44.3).

### Inside a zellij session

Once attached, you have full zellij control. Common bindings:

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
- **Kill** (`ctrl+x` from project list): Terminates the zellij session. Reopening the project starts a fresh session.
- **Quit** (`ctrl+c` from project list): Exits vibemux. All zellij sessions persist and can be reattached later.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/app` | Root TUI model, state machine, message routing |
| `internal/config` | XDG-compliant config store for projects (JSON) |
| `internal/model` | Project data structure |
| `internal/zellij` | zellij session management (create, attach, kill, list sessions, exact name matching so `vmx-foo` doesn't collide with `vmx-foo-bar`) plus the web dashboard (web server, login token, per-session URLs) |
| `internal/gitops` | Git URL normalization (HTTPS → SSH for GitHub) and `git clone` invocation |
| `internal/ui/projectlist` | Project list view with type-to-filter, digit quick-select, and active-only toggle |
| `internal/ui/addproject` | Add-project flow: pick existing / create empty / clone GitHub repo |

### How it works

vibemux is a **project launcher** that delegates terminal emulation to zellij:

```
┌─ vibemux (main app)
│  ├─ Project list with session status, type-to-filter, digit quick-select
│  └─ Key handler
│     ├─ "enter"  → zellij attach (hands terminal control to zellij)
│     ├─ "ctrl+o" → web dashboard (zellij web server + browser windows)
│     ├─ "ctrl+x" → zellij kill-session
│     ├─ "ctrl+d" → config.RemoveProject() (+ kill session if active)
│     ├─ "ctrl+n" → open add-project menu (pick / create / clone)
│     └─ "ctrl+a" → toggle active-only filter
│
└─ zellij (subprocess, full terminal control)
   ├─ Session: vmx-<project-path-slug>
   │  └─ Shell with full history and process state
   └─ Detach (Ctrl+o d) → returns control to vibemux
```

When you open a project, vibemux creates a zellij session (if needed, with web sharing enabled so the dashboard can reach it) and uses `tea.ExecProcess()` to hand terminal control to zellij. The session persists in the background even after you detach or quit vibemux. Session names are derived from the project path (prefixed `vmx-`) and matched exactly against the live session list, so similarly-named sessions don't collide.

### Session persistence

Sessions are managed by zellij's background server processes, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `zellij list-sessions`
- You can manually attach with `zellij attach vmx-<slug>`
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

- **zellij** 0.43+ must be installed, on `$PATH` or in `~/.local/bin` (0.44+ recommended)
- Go 1.21+
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
