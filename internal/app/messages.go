package app

import (
	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

// MultiplexerReturnedMsg is sent when tea.ExecProcess returns after the user
// detaches from (or the session ends in) the attached multiplexer session.
type MultiplexerReturnedMsg struct {
	Err error
}

// TickMsg is sent by the periodic tick timer to trigger a local status refresh.
type TickMsg struct{}

// StatusComputedMsg carries the full computed local status: active multiplexer
// sessions, agent statuses grouped by project, and git status per project.
type StatusComputedMsg struct {
	Active map[string]bool
	Agents map[string][]agent.Status
	Git    map[string]gitstatus.Status
}

// FetchDoneMsg is sent when a background git fetch for a project completes.
// It triggers a one-shot status refresh to pick up the fresh remote tracking
// info without starting a new tick loop.
type FetchDoneMsg struct {
	ProjectID string
}
