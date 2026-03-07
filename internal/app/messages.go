package app

// TmuxReturnedMsg is sent when tea.ExecProcess returns after the user detaches
// from (or the tmux session ends in) the attached tmux session.
type TmuxReturnedMsg struct {
	Err error
}

// SessionStatusMsg carries the set of currently active vibemux tmux session
// names so the project list can show indicators.
type SessionStatusMsg struct {
	ActiveSessions map[string]bool // session name → exists
}
