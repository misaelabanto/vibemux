# Observe agent state via hooks writing a status file

## Context

vibemux shows per-project status in the list. Beyond "a tmux session is active", we want to surface what the coding agent running inside a project is doing (working, done and waiting on you, blocked on a permission prompt, or gone quiet) along with the agent's last message.

## Decision

A coding agent reports its own state through lifecycle hooks that invoke a `vibemux hook` subcommand. That subcommand writes a small status file, one JSON file per agent session, into the runtime directory. The vibemux TUI reads those files, maps each to the owning project by closest-ancestor path match, and renders the state.

Because vibemux both writes the file (via `vibemux hook`) and reads it (in the TUI), the file format is an internal implementation detail with no cross-language contract to keep in sync.

This mechanism is meant to be **agent-agnostic**: any coding agent that can run a command on lifecycle events can feed vibemux through the same `vibemux hook` contract and status-file format. We are **starting with Claude Code specifically because the project maintainer uses it**. Support for other agents can be added later by mapping their event systems onto the same subcommand, without changing the TUI or the file format.

## Alternatives considered

- **Parsing the agent's transcript files.** Gives the last message and a rough done/working signal, but cannot see a live permission prompt (the Blocked state) and is fragile against transcript-format changes.
- **Scraping the tmux pane** (`capture-pane`). Sees the live UI including permission prompts, but is brittle: it depends on the agent's TUI strings, the terminal width, and which pane the agent runs in.

Hooks push authoritative state transitions from the agent itself, which is more robust than inferring them, and they are the only source that cleanly exposes the "blocked on a permission prompt" state.
