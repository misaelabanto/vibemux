# Project Status Indicators Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show per-project git working-tree status and live coding-agent state (Claude Code first) as icons in the vibemux project list.

**Architecture:** A coding agent reports state through lifecycle hooks that invoke a new `vibemux hook` subcommand, which writes one JSON status file per agent session into the runtime dir. The TUI reads those files (grouping each under its closest-ancestor project), reads local git status per project on a ~3s clock, fetches a repo in the background only when its session is opened, and renders a right-aligned icon column. Icons and tunables come from a new global `config.json`.

**Tech Stack:** Go 1.24, bubbletea/v2, bubbles/v2, lipgloss/v2, git CLI, tmux CLI.

## Global Constraints

- Go module `github.com/misaelabanto/vibemux`, Go 1.24.2.
- No new third-party dependencies unless unavoidable (stdlib `encoding/json`, `os/exec`, `crypto/sha256`, `context` preferred).
- Never use em dashes anywhere (code comments, commit messages, docs, UI copy). Use hyphen, colon, comma, or two sentences.
- All new code has Go tests; `go build ./...` and `go test ./...` must pass before each commit.
- Commits via `/commita` (subagent standing authorization for this approved plan).
- Agent-agnostic by design; Claude Code is the only wired agent for now (see `docs/adr/0001-agent-state-via-hooks.md`).

## File Structure

- `internal/config/settings.go` (new): `Settings` struct, defaults, `config.json` load/merge, `ConfigFile()`.
- `internal/agent/status.go` (new): `Status` struct, state constants.
- `internal/agent/store.go` (new): runtime dir resolution, atomic write, delete, load-all.
- `internal/agent/message.go` (new): last-sentence extraction from a transcript + aiTitle fallback.
- `internal/agent/match.go` (new): closest-ancestor project resolution, group-by-project, focus ordering.
- `internal/agent/hook.go` (new): hook stdin parsing + event routing.
- `internal/hookinstall/install.go` (new): merge/remove `vibemux hook` entries in `~/.claude/settings.json`.
- `internal/gitstatus/status.go` (new): compute git working-tree + upstream status.
- `internal/gitstatus/fetch.go` (new): safe background fetch.
- `internal/ui/projectlist/render.go` (new): icon/badge mapping + status-column composition (pure funcs).
- `internal/ui/projectlist/projectlist.go` (modify): hold agents+git per item, carousel keys, render.
- `internal/app/*.go` (modify): refresh ticker, fetch-on-enter, consent prompt, wiring.
- `main.go` (modify): subcommand dispatch (`hook`, `install-hooks`, `uninstall-hooks`, `icons`, default TUI).

---

### Task 1: Settings and config.json

**Files:**
- Create: `internal/config/settings.go`
- Test: `internal/config/settings_test.go`

**Interfaces:**
- Produces: `type Settings struct { Icons map[string]string; LocalRefreshMS int; StaleThresholdSec int; FetchOnEnter bool }`; `func DefaultSettings() Settings`; `func LoadSettings() Settings`; `func ConfigFile() string`.
- Icon keys: `working,done,blocked,stale,active,no_git`. Defaults: working `🦾`, done `✅`, blocked `‼️`, stale `🫠`, active `⚪`, no_git `⊘`. `LocalRefreshMS=3000`, `StaleThresholdSec=600`, `FetchOnEnter=true`.

- [ ] Step 1: Write failing tests: `DefaultSettings()` has the six icon keys and the numeric defaults; `LoadSettings()` with no file returns defaults; `LoadSettings()` merges a partial `config.json` (only `icons.working` set) over defaults keeping other defaults.
- [ ] Step 2: Run `go test ./internal/config/...`, expect FAIL (undefined).
- [ ] Step 3: Implement `Settings`, `DefaultSettings`, `ConfigFile()` (`filepath.Join(Dir(),"config.json")`), `LoadSettings()` (read file, json.Unmarshal into a copy of defaults so missing fields keep defaults; merge `Icons` key-by-key). Be tolerant: any read/parse error returns defaults.
- [ ] Step 4: Run `go test ./internal/config/...`, expect PASS.
- [ ] Step 5: `go build ./...`, then commit via `/commita`.

---

### Task 2: Agent status type and file store

**Files:**
- Create: `internal/agent/status.go`, `internal/agent/store.go`
- Test: `internal/agent/store_test.go`

**Interfaces:**
- Produces: `type State string` with consts `Working="working"`, `Done="done"`, `Blocked="blocked"` (Stale is derived later, not stored); `type Status struct { Cwd string; SessionID string; State State; Message string; UpdatedAt time.Time }` (json tags `cwd,session_id,state,message,updated_at`).
- `func AgentsDir() string`; `func Write(s Status) error`; `func Delete(sessionID string) error`; `func LoadAll() ([]Status, error)`.
- `AgentsDir()`: `$XDG_RUNTIME_DIR/vibemux/agents` if set, else `$XDG_STATE_HOME/vibemux/agents` if set, else `$HOME/.local/state/vibemux/agents`. Create with `MkdirAll` 0o755.
- Filename: `<sessionID>.json`. `Write` is atomic (write `<sessionID>.json.tmp` then `os.Rename`).

- [ ] Step 1: Write failing tests (set `XDG_RUNTIME_DIR` to `t.TempDir()`): write then LoadAll returns it; Write twice same session overwrites (LoadAll len 1); Delete removes; LoadAll on empty dir returns empty; malformed file is skipped, not fatal.
- [ ] Step 2: Run `go test ./internal/agent/...`, expect FAIL.
- [ ] Step 3: Implement status.go + store.go.
- [ ] Step 4: Run tests, expect PASS.
- [ ] Step 5: `go build ./...`, commit via `/commita`.

---

### Task 3: Last-sentence extraction

**Files:**
- Create: `internal/agent/message.go`
- Test: `internal/agent/message_test.go`

**Interfaces:**
- Produces: `func LastSentence(transcriptPath string) string`. Reads the transcript JSONL; tracks the last record with `type=="assistant"` and a text content block, and the last `aiTitle` (record with `aiTitle` field). From the last assistant text: strip basic markdown (drop fenced ``` blocks, leading `#`/`-`/`*` markers, backticks), collapse all whitespace/newlines to single spaces, trim. Split on `". "`, take the last non-empty trimmed segment. If that is empty or looks like code (contains no spaces and length>40, or starts with `{`/`}`), fall back to `aiTitle`. If still empty, return "".
- Helper (exported for tests): `func lastSentenceFromText(text string) string`.

- [ ] Step 1: Write failing tests for `lastSentenceFromText`: `"I refactored it. All tests pass. Want me to commit?"` -> `"Want me to commit?"`; `"All tests pass."` -> `"All tests pass."`; `"Done.\nWant me to commit?"` -> `"Want me to commit?"` (newline collapse); single sentence no period -> whole string; markdown bullets/`#` stripped. Plus a `LastSentence` test writing a small JSONL fixture to a temp file (assistant text last) and one where last assistant is a code block so it falls back to aiTitle.
- [ ] Step 2: `go test ./internal/agent/...`, expect FAIL.
- [ ] Step 3: Implement.
- [ ] Step 4: Tests PASS.
- [ ] Step 5: `go build ./...`, commit via `/commita`.

---

### Task 4: Project mapping and focus ordering

**Files:**
- Create: `internal/agent/match.go`
- Test: `internal/agent/match_test.go`

**Interfaces:**
- Consumes: `model.Project`, `agent.Status`.
- Produces: `func OwningProjectID(cwd string, projects []model.Project) (string, bool)` (longest path that equals cwd or is an ancestor of cwd, after `filepath.Clean`; compare with separator boundary so `/a/bc` is not an ancestor of `/a/b`); `func GroupByProject(statuses []Status, projects []model.Project) map[string][]Status`; `func SortByUrgency(ss []Status, staleThreshold time.Duration, now time.Time) []Status` ordering Blocked > Working > Done, and within equal state by most-recent `UpdatedAt`; the derived `Stale` (Working older than threshold) sorts just below Blocked. Provide `func DerivedState(s Status, staleThreshold time.Duration, now time.Time) State` returning `"stale"` when `State==Working && now.Sub(UpdatedAt) > staleThreshold` else `s.State`.

- [ ] Step 1: Write failing tests: exact match; nested projects `/code/mono` and `/code/mono/api`, cwd `/code/mono/api/src` -> `/code/mono/api`; sibling-prefix false positive guard; no match returns false; GroupByProject buckets multiple; SortByUrgency orders blocked first then stale then working then done, recency tiebreak; DerivedState flips Working->stale past threshold.
- [ ] Step 2: `go test ./internal/agent/...`, FAIL.
- [ ] Step 3: Implement.
- [ ] Step 4: PASS.
- [ ] Step 5: `go build ./...`, commit via `/commita`.

---

### Task 5: hook subcommand handler

**Files:**
- Create: `internal/agent/hook.go`
- Test: `internal/agent/hook_test.go`
- Modify: `main.go` (dispatch)

**Interfaces:**
- Consumes: `Write`, `Delete`, `LastSentence`.
- Produces: `func RunHook(r io.Reader) error`. Parses JSON with fields `hook_event_name,cwd,session_id,transcript_path`. Routing: `UserPromptSubmit|PreToolUse|PostToolUse` -> Write `{State:Working, Message: existing-or-empty, UpdatedAt: now}` (do not call LastSentence for working; keep it cheap; Message may be ""); `Stop` -> Write `{State:Done, Message: LastSentence(transcript_path), UpdatedAt: now}`; `Notification` -> Write `{State:Blocked, Message: LastSentence(transcript_path), UpdatedAt: now}`; `SessionEnd` -> `Delete(session_id)`. Unknown event -> no-op, return nil. Missing session_id -> return nil (never error the agent's run).
- `main.go`: `if len(os.Args)>1 && os.Args[1]=="hook" { if err:=agent.RunHook(os.Stdin); err!=nil { fmt.Fprintln(os.Stderr,err) }; return }` placed before TUI launch. Hook must never exit non-zero in a way that blocks Claude: always exit 0.

- [ ] Step 1: Write failing tests (temp XDG_RUNTIME_DIR): feed JSON for each event, assert resulting Status via LoadAll; SessionEnd after a Write deletes; unknown event no-op; malformed JSON returns nil.
- [ ] Step 2: `go test ./internal/agent/...`, FAIL.
- [ ] Step 3: Implement hook.go and main.go dispatch.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 6: hook install/uninstall

**Files:**
- Create: `internal/hookinstall/install.go`
- Test: `internal/hookinstall/install_test.go`
- Modify: `main.go` (dispatch)

**Interfaces:**
- Produces: `func SettingsPath() string` (`$HOME/.claude/settings.json`); `func Install(binPath string) error`; `func Uninstall() error`; `func IsInstalled() (bool, error)`. The hook command string is exactly `binPath + " hook"` (default `binPath="vibemux"`). Events to register: `UserPromptSubmit,PreToolUse,PostToolUse,Stop,Notification,SessionEnd`. Structure each as `{"hooks":[{"type":"command","command":"vibemux hook"}]}` appended to the event's array. Install is idempotent: skip an event that already contains a command equal to `vibemux hook`. Preserve all existing hooks/keys. Back up the existing file to `settings.json.vibemux-bak` before writing. Uninstall removes only entries whose command equals `vibemux hook` (and prunes now-empty matcher groups). Tolerate a missing file (treat as `{}`). Write with 2-space indent.
- `main.go`: dispatch `install-hooks` -> `hookinstall.Install("vibemux")`; `uninstall-hooks` -> `hookinstall.Uninstall()`; print a one-line result.

- [ ] Step 1: Write failing tests (set `HOME=t.TempDir()`): install into missing file creates all six events with `vibemux hook`; install preserving a pre-existing `Stop` notify-send hook keeps it and adds ours; running install twice does not duplicate; uninstall removes only ours and leaves the notify-send hook; IsInstalled reflects state.
- [ ] Step 2: `go test ./internal/hookinstall/...`, FAIL.
- [ ] Step 3: Implement (use `map[string]any` JSON handling to avoid clobbering unknown keys).
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 7: git status compute

**Files:**
- Create: `internal/gitstatus/status.go`
- Test: `internal/gitstatus/status_test.go`

**Interfaces:**
- Produces: `type Status struct { IsRepo bool; Clean bool; Modified bool; Staged bool; Untracked bool; Stashed bool; Conflict bool; Ahead int; Behind int; HasUpstream bool }`; `func Compute(path string) Status`.
- Implementation: run `git -C path rev-parse --is-inside-work-tree`; if it fails -> `{IsRepo:false}`. Else run `git -C path status --porcelain=v2 --branch`. Parse: lines starting `1 `/`2 ` are changed entries with XY field (col 2): staged if X in `MADRC`, modified if Y in `MD`; `u ` lines -> Conflict; `? ` -> Untracked. `# branch.ab +A -B` -> Ahead=A, Behind=B, HasUpstream=true. Stash: `git -C path rev-parse --verify --quiet refs/stash` exit 0 -> Stashed. Clean = repo with no Modified/Staged/Untracked/Conflict/Stashed.
- All git calls: 5s context timeout, `GIT_OPTIONAL_LOCKS=0` env, ignore stderr.

- [ ] Step 1: Write failing tests that build temp repos with `git init` + commits: non-repo dir -> IsRepo false; clean repo -> Clean true; create untracked file -> Untracked; `git add` it -> Staged; modify a committed file -> Modified; `git stash` -> Stashed. (Skip the test with `t.Skip` if `git` is not on PATH.)
- [ ] Step 2: `go test ./internal/gitstatus/...`, FAIL.
- [ ] Step 3: Implement.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 8: safe background fetch

**Files:**
- Create: `internal/gitstatus/fetch.go`
- Test: `internal/gitstatus/fetch_test.go`

**Interfaces:**
- Produces: `func HasRemote(path string) bool`; `func Fetch(ctx context.Context, path string) error`. `Fetch` returns nil immediately if `!HasRemote`. Command: `git -C path fetch --quiet` with env `GIT_TERMINAL_PROMPT=0`, `GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=10`, `GIT_OPTIONAL_LOCKS=0`; honor `ctx` (caller passes a 15s timeout). Errors are returned but callers ignore them (silent staleness).
- Provide `func FetchTimeout() time.Duration` returning 15s for callers.

- [ ] Step 1: Write failing tests: `HasRemote` false on a fresh `git init` temp repo (skip if no git); add a fake remote with `git remote add origin /nonexistent` -> HasRemote true; `Fetch` on no-remote repo returns nil fast.
- [ ] Step 2: `go test ./internal/gitstatus/...`, FAIL.
- [ ] Step 3: Implement.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 9: icon and badge rendering (pure functions)

**Files:**
- Create: `internal/ui/projectlist/render.go`
- Test: `internal/ui/projectlist/render_test.go`

**Interfaces:**
- Consumes: `config.Settings`, `agent.State`, `gitstatus.Status`.
- Produces: `func AgentIcon(state agent.State, s config.Settings) string` (maps working/done/blocked/stale via Icons; unknown -> ""); `func GitBadge(g gitstatus.Status, s config.Settings) string`. GitBadge: if `!g.IsRepo` -> Icons["no_git"]; else compose, space-joined in this order, only present flags: staged `●`, modified `✚`, untracked `…`, stashed `⚑`, conflict `✖`, then upstream: if HasUpstream and Ahead>0 `↑`, Behind>0 `↓`, both `<>`, equal and clean `=`; if fully clean and HasUpstream and Ahead==0 and Behind==0 show `✔`. (Git glyphs are literal Unicode constants in this file, not from Settings.)
- `func StatusLine(focused agent.Status, derived agent.State, otherCount int, g gitstatus.Status, active bool, s config.Settings) string`: `gitBadge + "  " + agentIconOrActiveDot + plusN`. When no agents and active -> Icons["active"]. `+N` only when otherCount>0.

- [ ] Step 1: Write failing tests: AgentIcon mapping; GitBadge for no-repo, clean+insync (`✔ =`), staged+untracked+ahead; StatusLine with 0 agents active shows active dot, with 3 agents shows focused icon + `+2`.
- [ ] Step 2: `go test ./internal/ui/projectlist/...`, FAIL.
- [ ] Step 3: Implement.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 10: projectlist model integration and carousel

**Files:**
- Modify: `internal/ui/projectlist/projectlist.go`
- Test: `internal/ui/projectlist/projectlist_test.go`

**Interfaces:**
- Consumes: render.go funcs, `agent.Status`, `gitstatus.Status`, `config.Settings`.
- Produces on `Model`: `SetSettings(config.Settings)`; `SetAgents(map[string][]agent.Status)`; `SetGitStatus(map[string]gitstatus.Status)`; `FocusNextAgent()`, `FocusPrevAgent()` (operate on the selected project; clamp/wrap). `projectItem` gains `agents []agent.Status`, `git gitstatus.Status`, `focused int`, `active bool`, `settings config.Settings`. `Title()` becomes `index. name` + right-aligned `StatusLine(...)` using known list width; `Description()` returns the focused agent's `Message`, except the selected row returns the path (the delegate already special-cases selection -> instead expose both and let the delegate pick; simplest: store both and have `buildItems` set Description to message, and the existing custom delegate render path on the selected index). Keep right-alignment by padding within the title to `m.list.Width()`.
- Rebind: in `New`, set `l.KeyMap.PrevPage.SetKeys("pgup")` and `NextPage.SetKeys("pgdown")` (drop left/right). In `Update`, when not filtering, `left`->FocusPrevAgent, `right`->FocusNextAgent (no list propagation).
- Default focus: when agents set for a project, `focused` = index of `SortByUrgency(...)[0]`; store agents already urgency-sorted so index 0 is the default and left/right walk the sorted slice.

- [ ] Step 1: Write failing tests on pure-ish logic: after `SetAgents` with 3 agents (one blocked), focused agent is the blocked one; FocusNextAgent advances and wraps; left/right keys change focus via `Update`; SetGitStatus/SetSettings store and reflect in `buildItems`. (Construct `Model` via `New` with a fixed width/height.)
- [ ] Step 2: `go test ./internal/ui/projectlist/...`, FAIL.
- [ ] Step 3: Implement; keep existing filter/quick-select behavior intact.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 11: app wiring (refresh ticker, fetch-on-enter, status compute)

**Files:**
- Modify: `internal/app/model.go`, `internal/app/update.go`, `internal/app/messages.go`
- Test: `internal/app/update_test.go`

**Interfaces:**
- Consumes: agent (LoadAll, GroupByProject, DerivedState, SortByUrgency), gitstatus (Compute, Fetch, HasRemote, FetchTimeout), config (LoadSettings).
- New messages: `type TickMsg struct{}`; `type StatusComputedMsg struct { Active map[string]bool; Agents map[string][]agent.Status; Git map[string]gitstatus.Status }`; `type FetchDoneMsg struct{ ProjectID string }`.
- `AppModel` holds `settings config.Settings`. `Init()` returns `tea.Batch(loadSettings->apply, computeStatus(), tick())`.
- `tick()` returns `tea.Tick(settings.LocalRefreshMS ms, func(_) TickMsg{})`. On `TickMsg` -> `tea.Batch(computeStatus(), tick())`.
- `computeStatus()`: a `tea.Cmd` that, off the UI goroutine, lists tmux sessions, `agent.LoadAll()` grouped per project with urgency sort + derived stale, and `gitstatus.Compute` per project, returning `StatusComputedMsg`. Cap git computes by iterating projects sequentially (acceptable for now; note: future optimization to visible rows).
- On `StatusComputedMsg`: push active/agents/git into the projectList via its setters.
- `openProject`: before returning the attach `ExecProcess` cmd, also kick a background fetch. Use `tea.Batch(execCmd, fetchCmd(p))` where `fetchCmd` runs `gitstatus.Fetch(ctx, p.Path)` (ctx timeout FetchTimeout) and returns `FetchDoneMsg`. On `TmuxReturnedMsg` (already handled) also trigger `computeStatus()` (already returns refresh; extend to full status).
- On `FetchDoneMsg` -> `computeStatus()` for freshness.

- [ ] Step 1: Write failing tests: `StatusComputedMsg` handling updates projectList setters (assert via projectList getters or that no panic and active map applied); `computeStatus()` returns a `StatusComputedMsg` when run (call the cmd func directly with a temp env). Keep tests light and deterministic (no real tmux/git assertions beyond "produces a message").
- [ ] Step 2: `go test ./internal/app/...`, FAIL.
- [ ] Step 3: Implement; preserve existing add/delete/kill/toggle behavior.
- [ ] Step 4: PASS; `go build ./...`.
- [ ] Step 5: Commit via `/commita`.

---

### Task 12: install/icons subcommands, consent prompt, README

**Files:**
- Modify: `main.go` (icons subcommand), `internal/app/*` (consent prompt), `README.md`
- Test: covered by build + manual; add `internal/app` consent-state unit test if feasible.

**Interfaces:**
- `main.go`: `icons` subcommand prints every state icon and git glyph with labels (uses `config.LoadSettings()`), so the user can eyeball nerd-font/emoji rendering. Dispatch order in main: `hook`, `install-hooks`, `uninstall-hooks`, `icons`, else TUI.
- Consent prompt: on TUI start, if `!hookinstall.IsInstalled()` and a `~/.config/vibemux/hooks-declined` marker file does not exist, show a small overlay/first-line prompt: "Enable agent status tracking? Adds hooks to Claude Code settings. [y/N]". `y` -> `hookinstall.Install("vibemux")`; `n` -> write the declined marker so it never asks again. Implement as a `ViewConsent` state shown before `ViewProjectList` only when needed; any key other than y/n dismisses for this run without marker.
- README: add a "Project status indicators" section documenting the icons, git glyphs, `install-hooks`/`uninstall-hooks`/`icons` commands, config.json, and that it is agent-agnostic (Claude first).

- [ ] Step 1: Implement `icons` subcommand; verify `go run . icons` prints glyphs.
- [ ] Step 2: Implement consent prompt state + marker handling.
- [ ] Step 3: Update README.
- [ ] Step 4: `go build ./...` and `go test ./...` all PASS.
- [ ] Step 5: Commit via `/commita`.

---

## Final verification

- [ ] `go build ./...` passes.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` clean.
- [ ] `go run . icons` renders all glyphs.
- [ ] `go run . install-hooks` is idempotent and preserves existing hooks (verified against a temp HOME in tests).
