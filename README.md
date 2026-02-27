# vibemux

A project-based terminal multiplexer written in Go with a bubbletea TUI. Manage multiple shell sessions across projects — detach without killing, reattach to resume exactly where you left off.

## What it does

`vibemux` lets you:
- **Register project directories** in a persistent config
- **Open interactive shell sessions** inside them with a PTY (pseudo-terminal) emulator
- **Switch between projects** with sessions staying alive in the background
- **Resume with full state** — history, running processes, environment — when you return to a project
- **Use `Ctrl+a` prefix** for commands, leaving `Ctrl+b` free for inner tmux/screen sessions

Each session runs in a real PTY (not a wrapper), so shells behave like native terminals. Full VT100 emulation means colors, cursor movement, and full-screen TUIs (vim, htop, etc.) all work correctly.

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
- Go 1.21+
- A POSIX-compliant shell (`$SHELL` environment variable, defaults to `/bin/sh`)

## Usage

### Project list (startup screen)

```
Enter    Open selected project in a terminal session
a        Add a new project (directory picker)
d        Delete selected project
q        Quit (closes all background sessions)
```

### Inside a terminal session (prefix mode: `Ctrl+a` + command)

```
Ctrl+a d       Detach from session (keeps shell running in background)
Ctrl+a p       Same as detach
Ctrl+a x       Kill session (closes the shell)
Ctrl+a Ctrl+a  Send literal Ctrl+a to the shell
```

### Session behavior

- **Detach** (`Ctrl+a d` / `Ctrl+a p`): Suspends the view and returns to the project list. The shell process continues running in the background with full state preserved.
- **Reattach**: Select the same project again to reconnect to the background session. Everything you left running is still there.
- **Kill** (`Ctrl+a x`): Terminates the shell process. Reopening the project starts a fresh session.
- **Quit** (`q` from project list): Closes all background sessions cleanly before exiting.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/app` | Root TUI model, state machine, message routing |
| `internal/config` | XDG-compliant config store for projects (JSON) |
| `internal/model` | Project and Session data structures |
| `internal/pty` | PTY lifecycle (start, read, write, resize, close) |
| `internal/ui/` | TUI subcomponents (project list, terminal, status bar, file picker) |

### Session persistence

```
┌─ vibemux (main app)
├─ activeSessionID (currently displayed terminal)
├─ sessions map (background shells)
│  ├─ project-id-1 → Terminal + PTY (running, background)
│  └─ project-id-2 → Terminal + PTY (running, background)
└─ Terminal emulator (vt10x)
   └─ Renders the shell output with full ANSI SGR colors/attrs
```

When you detach, the `terminal.Model` and its PTY move into the `sessions` map. When you reattach, it moves back to `activeSessionID` — the PTY's file descriptor and shell process remain connected the entire time.

### Message flow for background sessions

PTY output (and exit signals) are tagged with `ProjectID` in the message so the router knows which background session to update:

```
PTY output → SessionOutputMsg{ProjectID, Data}
            ↓
       App.Update (routes to background session in sessions map)
            ↓
       Terminal.Update (writes to vterm)
            ↓
       vterm.String() renders on next frame (even if session is in background)
```

This allows multiple shells to produce output concurrently without interfering.

## Dependencies

### Direct

| Package | Why |
|---------|-----|
| `charm.land/bubbletea/v2` | TUI event loop — state machine, input handling, render cycle |
| `charm.land/bubbles/v2` | TUI components: `list` (project picker), `filepicker` (add project) |
| `charm.land/lipgloss/v2` | Styling for status bar and UI elements (ANSI colors, layouts) |
| `github.com/hinshun/vt10x` | VT100 terminal emulator — parses ANSI escape sequences, renders cell grid |
| `github.com/creack/pty` | PTY allocation and lifecycle (Unix ioctl wrappers) |
| `github.com/google/uuid` | Project ID generation (stable, collision-free) |

### Why not tmux/screen?

`vibemux` uses a direct PTY instead of spawning a `tmux` session inside it. This means:
- **Simpler** — no subprocess management complexity
- **Faster** — direct shell, no tmux overhead
- **Portable** — doesn't require tmux to be installed
- **Full control** — we handle PTY resizing, signal forwarding, and terminal state directly

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
