# vibemux

A project-based terminal multiplexer for your terminal.

## What it does

`vibemux` lets you register project directories and open interactive shell sessions inside them. Switch between projects without killing their shells — each session persists in the background and resumes exactly where you left off. Uses a `Ctrl+a` prefix key for commands, leaving `Ctrl+b` for your inner tmux sessions.

## Usage

```bash
# Launch vibemux
./vibemux

# Project list controls
Enter    Open selected project in a terminal session
a        Add a new project (directory picker)
d        Delete selected project
q        Quit

# Inside a terminal session (after pressing Ctrl+a)
Ctrl+a p   Go back to the project list (session stays alive)
Ctrl+a x   Close the active pane
Ctrl+a d   Quit vibemux entirely (tmux sessions persist)
Ctrl+a Ctrl+a   Send a literal Ctrl+a to the shell
```

## How it works

1. Projects are stored in `~/.config/vibemux/projects.json`
2. Selecting a project spawns a `tmux` session in that directory (if it doesn't already exist).
3. `vibemux` captures the tmux pane content and renders it in its TUI.
4. Switching projects with `Ctrl+a p` suspends the view but leaves the `tmux` session running.
5. Returning to a project resumes the same `tmux` session — history, running processes, and all.
6. Even after quitting `vibemux`, your `tmux` sessions persist. You can attach to them directly via `tmux attach -t vibemux-<project-name>`.

## Build

```bash
go build -o vibemux .
```

## Dependencies

### Direct

| Package | Why |
|---|---|
| `charm.land/bubbletea/v2` | TUI event loop — drives the state machine, handles input, manages the render cycle |
| `charm.land/bubbles/v2` | Ready-made TUI components: `list` for the project picker and `filepicker` for adding new projects |
| `charm.land/lipgloss/v2` | Styles the status bar and other UI elements with ANSI colors and layout primitives |
| `github.com/google/uuid` | Generates stable IDs for each registered project so sessions can be keyed and persisted reliably |
| `tmux` (CLI) | Backend engine for terminal emulation and session persistence |
