package app

// MultiplexerReturnedMsg is sent when tea.ExecProcess returns after the user
// detaches from (or the session ends in) the attached multiplexer session.
type MultiplexerReturnedMsg struct {
	Err error
}

// SessionStatusMsg carries the set of currently active vibemux multiplexer session
// names so the project list can show indicators.
type SessionStatusMsg struct {
	ActiveSessions map[string]bool // session name → exists
}
