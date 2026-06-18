# vibemux

vibemux is a terminal UI for opening and managing per-project terminal sessions. It runs each project inside a multiplexer session so work survives detaching, and it can drive more than one multiplexer.

## Language

**Multiplexer**:
The terminal multiplexer that hosts project sessions. tmux and zellij are the two supported multiplexers. One is active per user, chosen during onboarding.
_Avoid_: backend, engine, driver, terminal tool

**Session**:
One multiplexer session bound to one project, named deterministically as `vmx-<dir>`. Created detached, attached on open, killed on demand.
_Avoid_: window, pane, tab (those are structures *inside* a session)

**Project**:
A directory the user has registered with vibemux. Persisted in `projects.json`. Opening a project creates or attaches its session.

**Active multiplexer**:
The single multiplexer vibemux currently uses for every session. Resolved at startup and persisted; never more than one at a time.

**Onboarding**:
The flow that resolves the active multiplexer when none is validly saved: at first run, or when the saved multiplexer is no longer installed (self-heal). It detects which multiplexers are installed, then either guides installation (none), auto-uses the only one (one), or asks the user to choose (two).
_Avoid_: setup wizard, install flow (onboarding may not install anything)
