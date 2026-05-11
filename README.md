# vibemux

A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent tmux sessions for your projects — detach without killing, reattach to resume exactly where you left off.

## What it does

`vibemux` lets you:
- **Register project directories** in a persistent config — pick an existing folder, create an empty one, or clone a GitHub repo
- **Open tmux sessions** for projects with a keystroke
- **Detach and switch** between projects without terminating shells
- **Reattach seamlessly** to running sessions with full history and state preserved
- **See active sessions** at a glance with visual indicators (`●`) in the project list
- **Filter to active-only** to focus on what's currently running
- **Type-to-filter** instantly by project name, or jump to a project by its index number

Each project gets its own tmux session that persists even after you quit vibemux. Full tmux compatibility means you can use any tmux features, plugins, or keybindings you're familiar with.

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

- **tmux** 2.6+ (required — vibemux uses tmux for terminal emulation)
- Go 1.21+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)

## Usage

### Project list (startup screen)

```
enter          Open selected project (attach tmux session)
type           Filter projects by name (any printable character starts filtering)
1-9 / digits   Quick-select project by its index number (e.g. type "12" → project #12)
ctrl+a         Toggle "active only" view (show only projects with running sessions)
ctrl+n         Add a new project (opens add-project menu)
ctrl+d         Delete project from config (kills session if active)
ctrl+x         Kill tmux session for selected project
ctrl+c         Quit vibemux (tmux sessions stay alive)
```

Notes:
- The `●` indicator next to a project name shows that an active tmux session exists for it.
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

### Inside a tmux session

Once attached, you have full tmux control. Detach using:

```
Ctrl+b d       Detach from session (return to vibemux)
Ctrl+b x       Kill pane/window
Ctrl+b c       Create new window
```

The default tmux prefix is `Ctrl+b`. See `man tmux` for the full command reference.

### Session behavior

- **Detach** (`Ctrl+b d`): Returns you to the vibemux project list. The shell process continues running in the background with full state preserved.
- **Reattach**: Select the same project again to reconnect to the background session. Everything you left running is still there.
- **Kill** (`ctrl+x` from project list): Terminates the tmux session. Reopening the project starts a fresh session.
- **Quit** (`ctrl+c` from project list): Exits vibemux. All tmux sessions persist and can be reattached later.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/app` | Root TUI model, state machine, message routing |
| `internal/config` | XDG-compliant config store for projects (JSON) |
| `internal/model` | Project data structure |
| `internal/tmux` | tmux session management (create, attach, kill, list sessions) — uses exact session-name matching so `vmx-foo` doesn't collide with `vmx-foo-bar` |
| `internal/gitops` | Git URL normalization (HTTPS → SSH for GitHub) and `git clone` invocation |
| `internal/ui/projectlist` | Project list view with type-to-filter, digit quick-select, and active-only toggle |
| `internal/ui/addproject` | Add-project flow: pick existing / create empty / clone GitHub repo |

### How it works

vibemux is a **project launcher** that delegates terminal emulation to tmux:

```
┌─ vibemux (main app)
│  ├─ Project list with session status, type-to-filter, digit quick-select
│  └─ Key handler
│     ├─ "enter"  → tmux attach (hands terminal control to tmux)
│     ├─ "ctrl+x" → tmux kill-session
│     ├─ "ctrl+d" → config.RemoveProject() (+ kill session if active)
│     ├─ "ctrl+n" → open add-project menu (pick / create / clone)
│     └─ "ctrl+a" → toggle active-only filter
│
└─ tmux (subprocess, full terminal control)
   ├─ Session: vmx-<project-path-slug>
   │  └─ Shell with full history and process state
   └─ Detach (Ctrl+b d) → returns control to vibemux
```

When you open a project, vibemux creates a tmux session (if needed) and uses `tea.ExecProcess()` to hand terminal control to tmux. The session persists in the tmux server even after you detach or quit vibemux. Session names are derived from the project path (prefixed `vmx-`) and looked up using tmux's exact-match syntax (`=name`) so similarly-named sessions don't collide.

### Session persistence

Sessions are managed by the tmux server, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `tmux list-sessions`
- You can manually attach with `tmux attach-session -t =vmx-<slug>` (the `=` forces exact-name matching)
- Sessions can accumulate if not cleaned up (use `ctrl+x` or `ctrl+d` in vibemux to kill them)

## Dependencies

### Direct

| Package | Why |
|---------|-----|
| `charm.land/bubbletea/v2` | TUI event loop — state machine, input handling, render cycle |
| `charm.land/bubbles/v2` | TUI components: `list` (project picker), `filepicker` (add project), `textinput` (folder name / repo URL), `spinner` (clone progress) |
| `charm.land/lipgloss/v2` | Styling for UI elements (ANSI colors, layouts) |
| `github.com/charmbracelet/x/ansi` | ANSI-safe string truncation in the highlight-while-filtering delegate |
| `github.com/google/uuid` | Project ID generation (stable, collision-free) |
| `git` (binary) | Invoked by `internal/gitops` to clone repositories |

### Requirements

- **tmux** must be installed and available on `$PATH`
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
