// Package hookinstall manages vibemux hook entries in ~/.claude/settings.json.
// It can install, uninstall, and check for the "vibemux hook" command under
// the six Claude Code lifecycle events.
package hookinstall

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// requiredEvents lists the Claude Code lifecycle events vibemux registers under.
var requiredEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Stop",
	"Notification",
	"SessionEnd",
}

// SettingsPath returns the absolute path to ~/.claude/settings.json.
func SettingsPath() string {
	return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
}

// loadSettings reads and unmarshals settings.json. If the file does not exist,
// it returns an empty map without error.
func loadSettings() (map[string]any, error) {
	path := SettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// saveSettings writes settings with 2-space indentation to settings.json,
// creating the directory if needed.
func saveSettings(m map[string]any) error {
	path := SettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// backupSettings copies the current settings.json to settings.json.vibemux-bak.
// If the file does not exist, it does nothing.
func backupSettings() error {
	path := SettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.WriteFile(path+".vibemux-bak", data, 0o644)
}

// hooksMap returns the top-level "hooks" map from settings, creating it
// and wiring it in if absent.
func hooksMap(settings map[string]any) map[string]any {
	raw, ok := settings["hooks"]
	if !ok {
		h := map[string]any{}
		settings["hooks"] = h
		return h
	}
	h, ok := raw.(map[string]any)
	if !ok {
		h = map[string]any{}
		settings["hooks"] = h
	}
	return h
}

// eventGroups returns the group slice for the given event, creating it if
// absent and wiring it into the hooks map.
func eventGroups(hooks map[string]any, event string) []any {
	raw, ok := hooks[event]
	if !ok {
		return nil
	}
	groups, ok := raw.([]any)
	if !ok {
		return nil
	}
	return groups
}

// eventContainsCmd returns true if any group under groups contains a hook
// whose "command" equals cmd.
func eventContainsCmd(groups []any, cmd string) bool {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		hooksArr, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hooksArr {
			entry, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if c, ok := entry["command"].(string); ok && c == cmd {
				return true
			}
		}
	}
	return false
}

// Install adds a "vibemux hook" command group under each required event in
// ~/.claude/settings.json. It is idempotent: events already containing
// "vibemux hook" are skipped. All existing hooks and top-level keys are
// preserved. A backup of the existing file is written to
// settings.json.vibemux-bak before any changes.
//
// binPath is the executable name (typically "vibemux"); the registered command
// will be binPath + " hook".
func Install(binPath string) error {
	if err := backupSettings(); err != nil {
		return err
	}

	settings, err := loadSettings()
	if err != nil {
		return err
	}

	cmd := binPath + " hook"
	hooks := hooksMap(settings)

	for _, event := range requiredEvents {
		groups := eventGroups(hooks, event)
		if eventContainsCmd(groups, cmd) {
			continue
		}
		newGroup := map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": cmd,
				},
			},
		}
		hooks[event] = append(groups, newGroup)
	}

	return saveSettings(settings)
}

// Uninstall removes all hook groups whose command equals "vibemux hook" from
// ~/.claude/settings.json. Groups that become empty after removal are dropped.
// Event keys that become empty are deleted from the hooks map.
// All other hooks and top-level keys are preserved.
// If the file does not exist, Uninstall is a no-op.
// A backup of the existing file is written to settings.json.vibemux-bak before
// any changes are made.
func Uninstall() error {
	if err := backupSettings(); err != nil {
		return err
	}

	settings, err := loadSettings()
	if err != nil {
		return err
	}
	if len(settings) == 0 {
		return nil
	}

	raw, ok := settings["hooks"]
	if !ok {
		return nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	const cmd = "vibemux hook"

	// Iterate over ALL keys present in the hooks map, not just requiredEvents,
	// so any event that contains a "vibemux hook" command is cleaned up.
	for event := range hooks {
		rawGroups, ok := hooks[event]
		if !ok {
			continue
		}
		groups, ok := rawGroups.([]any)
		if !ok {
			continue
		}

		var filtered []any
		for _, g := range groups {
			group, ok := g.(map[string]any)
			if !ok {
				filtered = append(filtered, g)
				continue
			}
			hooksArr, ok := group["hooks"].([]any)
			if !ok {
				filtered = append(filtered, g)
				continue
			}
			var keptHooks []any
			for _, h := range hooksArr {
				entry, ok := h.(map[string]any)
				if !ok {
					keptHooks = append(keptHooks, h)
					continue
				}
				if c, ok := entry["command"].(string); ok && c == cmd {
					continue
				}
				keptHooks = append(keptHooks, h)
			}
			if len(keptHooks) == 0 {
				// Drop this group entirely.
				continue
			}
			group["hooks"] = keptHooks
			filtered = append(filtered, group)
		}
		if len(filtered) == 0 {
			// Delete the event key rather than assigning nil/empty (nil marshals to null).
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}

	return saveSettings(settings)
}

// IsInstalled returns true if every required event in settings.json contains
// at least one hook group with command "vibemux hook". Returns false (no error)
// if the file is missing or any event lacks the command.
func IsInstalled() (bool, error) {
	settings, err := loadSettings()
	if err != nil {
		return false, err
	}
	if len(settings) == 0 {
		return false, nil
	}

	raw, ok := settings["hooks"]
	if !ok {
		return false, nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return false, nil
	}

	const cmd = "vibemux hook"
	for _, event := range requiredEvents {
		groups := eventGroups(hooks, event)
		if !eventContainsCmd(groups, cmd) {
			return false, nil
		}
	}
	return true, nil
}
