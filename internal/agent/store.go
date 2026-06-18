package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AgentsDir returns the directory used to store per-session agent status files.
// Resolution order:
//  1. $XDG_RUNTIME_DIR/vibemux/agents
//  2. $XDG_STATE_HOME/vibemux/agents
//  3. $HOME/.local/state/vibemux/agents
//
// The directory is created (with MkdirAll 0o755) on first access.
func AgentsDir() string {
	var base string
	switch {
	case os.Getenv("XDG_RUNTIME_DIR") != "":
		base = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "vibemux", "agents")
	case os.Getenv("XDG_STATE_HOME") != "":
		base = filepath.Join(os.Getenv("XDG_STATE_HOME"), "vibemux", "agents")
	default:
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state", "vibemux", "agents")
	}
	_ = os.MkdirAll(base, 0o755)
	return base
}

func filePath(sessionID string) string {
	return filepath.Join(AgentsDir(), sessionID+".json")
}

// Write atomically persists s to <AgentsDir>/<sessionID>.json.
// It writes to a .tmp file first, then renames to ensure atomic replacement.
func Write(s Status) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	dst := filePath(s.SessionID)
	tmp := dst + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Delete removes the status file for the given sessionID.
// Returns nil if the file does not exist.
func Delete(sessionID string) error {
	err := os.Remove(filePath(sessionID))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadAll reads all *.json status files from AgentsDir.
// Malformed files are silently skipped so a single corrupt entry never
// blocks the rest of the list.
func LoadAll() ([]Status, error) {
	dir := AgentsDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var statuses []Status
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			// Skip unreadable files.
			continue
		}

		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
			// Skip malformed files - do not propagate error.
			continue
		}
		statuses = append(statuses, s)
	}

	return statuses, nil
}
