# Zellij as a tmux replacement: evaluation

Date: 2026-06-12
Branch: `feature/zellij-dashboard`
Tested against: zellij 0.44.3 (latest, May 2026), tmux 3.4
Spike code: `internal/zellij/` (same API surface as `internal/tmux/`, all tests pass)

## Verdict

**Viable, but degraded and with more code, not less.** Zellij covers every vibemux
capability, but the dashboard cannot use the tmux approach (nested attach is broken
upstream with no workaround). A working alternative was found and proven: each
dashboard pane runs a plain subprocess that mirrors the target session via
`zellij action dump-screen --ansi` polling. That yields a live side-by-side view,
read-only, with ~1s refresh. Everything else (create detached, list, kill, attach)
maps cleanly, but the zellij backend is 212 lines vs 141 for tmux, and the
dashboard is a monitoring view rather than tmux's fully interactive nested panes.

## The dashboard blocker (empirically verified)

The tmux dashboard works by running `TMUX= exec tmux attach-session -t '=name'`
in each pane: nested clients are explicitly supported once the env var is cleared.
Zellij has no equivalent escape hatch.

Tested on this machine with isolated `ZELLIJ_SOCKET_DIR`/`XDG_CACHE_HOME`, two
background sessions printing distinguishable output, and a generated KDL layout:

- **Nested `zellij attach` panes: broken.** Both inner clients connect, but both
  render into the first pane (last writer wins). The pane that ran
  `attach vmx-alpha` displayed the vmx-beta UI; the second pane stayed blank.
  Clearing `ZELLIJ`/`ZELLIJ_SESSION_NAME` changes nothing. Matches upstream
  issues [#3411](https://github.com/zellij-org/zellij/issues/3411),
  [#3519](https://github.com/zellij-org/zellij/issues/3519),
  [#3847](https://github.com/zellij-org/zellij/issues/3847) (open, no fix planned).
- **`zellij watch` panes (read-only mirror, 0.44+): fails.** Watch processes stay
  alive but render nothing inside a pane, even with a real client attached to the
  watched session. Also read-only, so even working it would degrade the feature.
- **No grouped-sessions/linked-windows equivalent** to borrow from tmux's model.

The only first-party multi-session UX zellij offers is the session-manager plugin
and `zellij action switch-session`: navigation between sessions, not simultaneous
side-by-side viewing. That is a different feature, not the dashboard.

## The workaround that does work: subprocess mirror (empirically verified)

A dashboard pane does not need to be a nested multiplexer client. It can run a
plain subprocess that fetches the target session's screen and repaints:

```bash
while true; do
  out=$(zellij --session vmx-alpha action dump-screen --pane-id terminal_1 --ansi)
  printf '\033[H\033[2J%s' "$out"
  sleep 1
done
```

Verified on 0.44.3: both dashboard panes showed their own session's live output
side by side with advancing timestamps, and an attached client rendered both
simultaneously. Key enabling facts, all tested:

- `zellij --session X action ...` works from inside a pane of a different zellij
  session; explicit `--session` overrides the ZELLIJ env vars (issue #3637
  hijacking did not occur).
- `dump-screen` prints to stdout and supports `--ansi`, `--full`, `--pane-id`.
  Always pass `--pane-id` explicitly: without it, a fresh background session may
  have a floating plugin focused and the dump comes back empty.
- An event-driven variant also works (`subscribe --format json --ansi` piped
  through jq, full viewport frames ~1/sec), but adds a jq runtime dependency;
  the spike uses polling.
- Input forwarding is feasible: `action write-chars --pane-id` plus `action write
  13` (Enter) typed into a source session and took effect. Caveat: keystrokes
  sent before the pane's shell has rendered a prompt are silently dropped.

Limitations vs tmux nested attach: read-only (an input forwarder would be a
separate raw-mode shim), mirrors a single pane (terminal_0) rather than the whole
session with its splits and status bar, viewport only (no scrollback), no resize
reflow or cursor mirroring, ~1s refresh with full clear+repaint flicker, and a
dead source session leaves a frozen mirror pane instead of auto-closing.

## Capability mapping (everything else works)

| vibemux need | tmux | zellij 0.44.3 | Notes |
|---|---|---|---|
| Create detached session at dir | `new-session -d -s n -c dir` | `attach --create-background n options --default-cwd dir` | Needs 0.40+. No `--cwd` flag; use process cwd or options |
| Session exists (exact) | `has-session -t =n` | none; parse `list-sessions -n` | Must exclude `EXITED` lines or dead sessions count as alive |
| List sessions | `list-sessions -F ...` | `list-sessions -n` | No JSON/format strings; merges live and EXITED entries |
| Kill session | `kill-session` | `kill-session` + `delete-session --force` | Plain kill leaves a resurrectable EXITED corpse (serialization is on by default, ~60s interval) |
| Attach (terminal handoff) | `attach-session` | `attach n` | Works with tea.ExecProcess |
| Drive from outside | `send-keys` | `zellij --session n action ...` | 0.44+ is actually nicer: pane IDs on stdout, `--block-until-exit`, `list-panes --json` |
| Tiled N-pane layout | `split-window` + `select-layout tiled` | generate KDL grid file | No auto-tile command; spike generates rows/columns breadth-first |
| Nested attach (dashboard) | `TMUX= exec tmux attach` | broken; use subprocess mirror | Mirror is read-only, ~1s refresh (see section above) |
| Inside-multiplexer detection | `$TMUX` | `$ZELLIJ` (= "0"), `$ZELLIJ_SESSION_NAME` | Session name var goes stale after rename |

## Native zellij options for "all sessions at once" (investigated 2026-06-12)

Is there anything first-party, without per-pane subprocess loops? Short answer: no
first-class feature exists, but the web client comes close.

### WASM plugin (the session-manager mechanism): no content access across sessions

- Each zellij session is a separate server process. Cross-session data
  (`SessionUpdate` event) is metadata gossip via cache files: session names, tab
  and pane manifests, geometry. No pane content, and the protobuf API has no way
  to request it. Within its OWN session a 0.44 plugin can read pane content
  (`PaneRenderReport`, `get_pane_scrollback`), but not for other sessions.
- A plugin CAN re-implement the dump-screen mirror natively: `run_command()`
  against `zellij --session X action dump-screen --ansi` returns stdout in-event,
  plus timers and ANSI rendering. Same fidelity as the bash mirror, so it is a
  packaging upgrade only, and it costs a Rust toolchain (the Go plugin SDK,
  zelligo, predates the needed APIs).
- No community plugin does simultaneous multi-session viewing (awesome-zellij
  surveyed: all session plugins are switchers). Upstream has no roadmap signal
  for cross-session content; #3411/#3519/#3847 sit open with near-zero activity.

### Web client (0.43+, tested on 0.44.3): works, one browser window per session

Empirically verified with two live sessions: `zellij web --start -d --port 8899`
serves `http://127.0.0.1:8899/<session-name>`, fully interactive (typed input
executed, live resize reflow), both sessions streaming simultaneously in two
tabs, mirrorable with a terminal attach, and immune to the nesting bug since the
browser is not a zellij pane. Caveats:

- No built-in multi-session page, and `X-Frame-Options: DENY` blocks composing
  sessions into one page via iframes. Tiling means one browser window per
  session, arranged by the OS window manager (e.g. `chromium --app=URL`).
- Sessions must be created with `web_sharing "on"` (default off). Gotcha: an
  existing `~/.config/zellij` wins over `XDG_CONFIG_HOME`; pass
  `ZELLIJ_CONFIG_DIR` or `--config` explicitly.
- One-time token auth (`zellij web --create-token`, shown once, cookie persists
  per browser). Read-only tokens available. Never link to `/`: it spawns a junk
  randomly named session. No JSON session-list endpoint; discovery stays on the
  CLI. Localhost HTTP is fine; non-local binds require TLS.
- Cost: ~17 MB for the web server, plus one browser window per session.

## Other risks found

- **Version fragmentation:** the usable orchestration CLI (send-keys, watch,
  switch-session, list-panes --json) is 0.44.0+, released March 2026. Distro
  packages commonly ship 0.39 to 0.41. `attach --create-background` needs 0.40+.
  Client/server version mismatches historically broke session visibility (#3371).
- **Resurrection semantics:** killed sessions reappear as `EXITED` in listings and
  resurrected command panes wait behind a "Press ENTER to run" banner. Vibemux
  would either parse around this everywhere or require
  `session_serialization false` in user config.
- **Silent failures:** `write-chars --pane-id` against a nonexistent pane exits 0.

## Where zellij is genuinely better

For fairness, things that would be easier than tmux if the dashboard were not the
goal: declarative KDL layouts beat imperative split-window loops, `--block-until-exit`
has no clean tmux equivalent, `subscribe` streams pane output as NDJSON, pane IDs
are returned on creation, and session resurrection comes free (tmux needs
tmux-resurrect).

## Outcome (2026-06-12, same day)

The user accepted the web-based dashboard tradeoff, so this branch implements the
full replacement: `internal/app` now uses `internal/zellij` exclusively, sessions
are created web-shared, and ctrl+o starts the local zellij web server and opens
one browser window per active session (the TUI stays in the project list; no
`vmx-dashboard` session exists anymore). Implementation notes:

- zellij 0.44.3 silently ignores `options --web-sharing on` at session creation
  (flag parses, `web_clients_allowed` stays false). The working method is a
  generated config file passed via the global `--config` flag, with
  `web_sharing "on"` prepended to a copy of the user's effective config
  (duplicate KDL keys are first-wins). The file must outlive the creating
  client: the server re-reads it, so it persists in the temp dir.
- First run creates a one-time web login token, surfaced in a status message
  and on stderr; zellij stores only hashes, so it cannot be re-shown.
- `internal/tmux` remains in the tree, unused by the app, as the reference
  implementation; the in-terminal mirror `BuildDashboard` stays in
  `internal/zellij` as a documented fallback that the app does not call.

The section below predates that decision and is kept as the original analysis.

## Recommendation

Stay on tmux as the primary backend: it delivers the fully interactive dashboard
in less code with decades-stable semantics. The subprocess mirror makes a zellij
backend genuinely possible, and the spike in `internal/zellij/` implements it
end to end (BuildDashboard generates mirror panes; the test asserts both source
sessions render in the dashboard). Treat it as a read-only monitoring dashboard;
full interactivity needs an input-forwarding shim or the upstream nesting fix
(track issues #3411 and #3847).
