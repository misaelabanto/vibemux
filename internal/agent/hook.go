package agent

import (
	"encoding/json"
	"io"
	"time"
)

// hookPayload is the JSON structure sent by Claude Code on lifecycle events.
type hookPayload struct {
	HookEventName  string `json:"hook_event_name"`
	Cwd            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// RunHook reads a JSON hook payload from r, determines the event type, and
// writes or deletes the per-session agent status file accordingly.
//
// It returns nil for malformed JSON, missing session IDs, and unknown events
// so that the calling agent process is never blocked by a non-zero exit code.
func RunHook(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		// Read error: be lenient, do not propagate.
		return nil
	}

	var p hookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		// Malformed JSON: no-op.
		return nil
	}

	if p.SessionID == "" {
		// Missing session ID: no-op.
		return nil
	}

	now := time.Now().UTC()

	switch p.HookEventName {
	case "UserPromptSubmit", "PreToolUse", "PostToolUse":
		return Write(Status{
			Cwd:       p.Cwd,
			SessionID: p.SessionID,
			State:     Working,
			Message:   "",
			UpdatedAt: now,
		})

	case "Stop":
		return Write(Status{
			Cwd:       p.Cwd,
			SessionID: p.SessionID,
			State:     Done,
			Message:   LastSentence(p.TranscriptPath),
			UpdatedAt: now,
		})

	case "Notification":
		return Write(Status{
			Cwd:       p.Cwd,
			SessionID: p.SessionID,
			State:     Blocked,
			Message:   LastSentence(p.TranscriptPath),
			UpdatedAt: now,
		})

	case "SessionEnd":
		return Delete(p.SessionID)

	default:
		// Unknown event: no-op.
		return nil
	}
}
