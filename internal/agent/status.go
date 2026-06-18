package agent

import "time"

// State represents the current activity state of a coding agent.
type State string

const (
	// Working means the agent is actively running.
	Working State = "working"
	// Done means the agent has finished successfully.
	Done State = "done"
	// Blocked means the agent is waiting for user input.
	Blocked State = "blocked"
)

// Status holds the per-session agent status persisted to disk.
type Status struct {
	Cwd       string    `json:"cwd"`
	SessionID string    `json:"session_id"`
	State     State     `json:"state"`
	Message   string    `json:"message"`
	UpdatedAt time.Time `json:"updated_at"`
}
