# vibemux

A project-based terminal session manager: vibemux launches and tracks persistent tmux sessions for registered project directories, and surfaces at-a-glance status for each project: its session, its git working tree, and the state of the Claude agent running inside it.

## Language

### Projects and sessions

**Project**:
A registered directory vibemux can open and track. Has a stable identity independent of its path or name.
_Avoid_: repo, folder (a Project need not be a git repository, and "folder" hides that it is a tracked entity, not just a directory).

**Session**:
The persistent tmux session bound to a Project, owned by the tmux server and surviving vibemux restarts.
_Avoid_: terminal, window, pane (those are tmux substructures that live inside a Session).

**Active**:
A Project whose Session currently exists in the tmux server. This is the only status vibemux surfaces today.
_Avoid_: open, running (running collides with the Agent's Working state).

### Agent state

**Agent**:
A coding agent running inside a Project's Session, tracked indirectly through the state it reports rather than observed directly. Claude Code is the first (currently only) supported agent, but the model is agent-agnostic. A Project may host several Agents at once (separate sessions or panes), each carrying its own Agent state.
_Avoid_: Claude, AI, assistant (use Agent for the tracked entity; name a specific product like "Claude Code" only when the distinction matters).

**Agent state**:
The current situation of a single Agent: one of Working, Done, Blocked, or Stale. Working, Done, and Blocked are reported by the Agent itself; Stale is inferred by vibemux. A Project surfaces the state of every Agent it hosts; when it hosts none, it has no Agent state.

**Working**:
The Agent is mid-turn: producing output or running tools. The ball is in Claude's court; leave it alone.
_Avoid_: busy, running, thinking.

**Done**:
The Agent's turn has ended and the ball is in your court, with nothing specifically pending. May mean the task is finished, or that Claude asked a question and stopped. These are indistinguishable to vibemux.
_Avoid_: idle, finished, complete (each overclaims; Claude may simply be awaiting an answer).

**Blocked**:
The Agent has stopped on a gate that needs your decision before it can continue: a tool-permission prompt. The most urgent state to act on.
_Avoid_: paused, stuck.

**Stale**:
An Agent that was Working but has gone quiet: no update for longer than expected while its Session is still Active. Most likely crashed or wedged, but possibly just deep in a long-running step. Means "reattach and check," not a confirmed failure.
_Avoid_: Crashed, dead, hung (each asserts a cause vibemux cannot actually confirm).

## Flagged ambiguities

- **"Waiting" is banned as a state name.** Both Done and Blocked are "waiting for you." Done = waiting for your next instruction (nothing pending). Blocked = waiting for your decision on a specific pending action. Always name the specific state.
- **"Active" is not "Working."** Active describes the Session existing; Working describes the Agent producing output. A Project can be Active with no Agent, or Active and Done.
- **Done can hide a question.** When Claude asks something in text and ends its turn, vibemux sees only that the turn ended, so it reads as Done rather than as needing you. Accepted limitation.

## Example dialogue

Dev: "This project's icon is green - is Claude finished?"
Expert: "Green is Done: its turn ended, so it's your turn now. It either finished the task or asked you something and stopped. We can't tell which."
Dev: "And the red one?"
Expert: "Blocked. Claude hit a permission prompt and can't move until you approve. That's the one to jump on first."
Dev: "One project is Active but has no colored icon."
Expert: "Then there's no Agent running in it. The Session is alive, there's just no Claude process reporting any state."
Dev: "Can a project have more than one Agent at once?"
Expert: "Yes - Claude running in two panes of the same Session is two Agents, each with its own state. The project surfaces them together so you can check each one."
