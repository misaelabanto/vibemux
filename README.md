# vibemux

A project-based terminal session manager written in Go with a bubbletea TUI. Launch and manage persistent tmux sessions for your projects — detach without killing, reattach to resume exactly where you left off.

## What it does

`vibemux` lets you:
- **Register project directories** in a persistent config
- **Open tmux sessions** for projects with a keystroke
- **Detach and switch** between projects without terminating shells
- **Reattach seamlessly** to running sessions with full history and state preserved
- **See active sessions** at a glance with visual indicators in the project list

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
Enter    Open selected project (attach tmux session)
a        Add a new project (directory picker)
x        Kill tmux session for selected project
d        Delete project from config (kills session if active)
q        Quit vibemux (tmux sessions stay alive)
```

Note: The `●` indicator next to a project name shows that an active tmux session exists for it.

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
- **Kill** (`x` from project list): Terminates the tmux session. Reopening the project starts a fresh session.
- **Quit** (`q` from project list): Exits vibemux. All tmux sessions persist and can be reattached later.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/app` | Root TUI model, state machine, message routing |
| `internal/config` | XDG-compliant config store for projects (JSON) |
| `internal/model` | Project data structure |
| `internal/tmux` | tmux session management (create, attach, kill, list sessions) |
| `internal/ui/` | TUI subcomponents (project list, file picker) |

### How it works

vibemux is a **project launcher** that delegates terminal emulation to tmux:

```
┌─ vibemux (main app)
│  ├─ Project list with session status
│  └─ Key handler
│     ├─ "enter" → tmux attach (hands terminal control to tmux)
│     ├─ "x" → tmux kill-session
│     └─ "d" → config.RemoveProject()
│
└─ tmux (subprocess, full terminal control)
   ├─ Session: vibemux-<uuid>
   │  └─ Shell with full history and process state
   └─ Detach (Ctrl+b d) → returns control to vibemux
```

When you open a project, vibemux creates a tmux session (if needed) and uses `tea.ExecProcess()` to hand terminal control to tmux. The session persists in the tmux server even after you detach or quit vibemux. Session names follow the pattern `vibemux-<uuid-without-dashes>`.

### Session persistence

Sessions are managed by the tmux server, not vibemux. This means:
- Sessions persist across vibemux restarts
- You can list them with `tmux list-sessions`
- You can manually attach with `tmux attach-session -t vibemux-<uuid>`
- Sessions can accumulate if not cleaned up (use `x` or `d` in vibemux to kill them)

## Dependencies

### Direct

| Package | Why |
|---------|-----|
| `charm.land/bubbletea/v2` | TUI event loop — state machine, input handling, render cycle |
| `charm.land/bubbles/v2` | TUI components: `list` (project picker), `filepicker` (add project) |
| `charm.land/lipgloss/v2` | Styling for UI elements (ANSI colors, layouts) |
| `github.com/google/uuid` | Project ID generation (stable, collision-free) |

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
