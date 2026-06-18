package hookinstall_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/misaelabanto/vibemux/internal/hookinstall"
)

// requiredEvents are the six events vibemux registers hooks for.
var requiredEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Stop",
	"Notification",
	"SessionEnd",
}

// readSettings reads and parses the settings.json in the given dir.
func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("readSettings unmarshal: %v", err)
	}
	return m
}

// commandsForEvent collects all command strings registered under eventName.
func commandsForEvent(t *testing.T, settings map[string]any, eventName string) []string {
	t.Helper()
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := hooks[eventName]
	if !ok {
		return nil
	}
	groups, ok := raw.([]any)
	if !ok {
		return nil
	}
	var cmds []string
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
			if cmd, ok := entry["command"].(string); ok {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

// containsCmd returns true if cmds contains target.
func containsCmd(cmds []string, target string) bool {
	for _, c := range cmds {
		if c == target {
			return true
		}
	}
	return false
}

// TestInstallIntoMissingFile verifies that installing into a non-existent
// settings.json creates the file and registers "vibemux hook" in all six events.
func TestInstallIntoMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	settings := readSettings(t, dir)
	for _, event := range requiredEvents {
		cmds := commandsForEvent(t, settings, event)
		if !containsCmd(cmds, "vibemux hook") {
			t.Errorf("event %s: missing 'vibemux hook'; got %v", event, cmds)
		}
	}
}

// TestInstallPreservesExistingHook verifies that a pre-existing hook under Stop
// is kept intact, and "vibemux hook" is added alongside it.
func TestInstallPreservesExistingHook(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write a settings.json with an existing notify-send hook under Stop.
	initial := map[string]any{
		"permissions": []any{"allow:Bash"},
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "notify-send 'Claude stopped'",
						},
					},
				},
			},
		},
	}
	clauDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(clauDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(filepath.Join(clauDir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	settings := readSettings(t, dir)

	// The existing notify-send hook must still be present.
	stopCmds := commandsForEvent(t, settings, "Stop")
	if !containsCmd(stopCmds, "notify-send 'Claude stopped'") {
		t.Errorf("Stop: existing notify-send hook was removed; got %v", stopCmds)
	}

	// Our hook must also be present.
	if !containsCmd(stopCmds, "vibemux hook") {
		t.Errorf("Stop: 'vibemux hook' not added; got %v", stopCmds)
	}

	// All other required events must also have our hook.
	for _, event := range requiredEvents {
		cmds := commandsForEvent(t, settings, event)
		if !containsCmd(cmds, "vibemux hook") {
			t.Errorf("event %s: missing 'vibemux hook'; got %v", event, cmds)
		}
	}

	// The unrelated top-level key must survive.
	if _, ok := settings["permissions"]; !ok {
		t.Error("top-level 'permissions' key was removed")
	}
}

// TestInstallIdempotent verifies that calling Install twice does not duplicate
// the "vibemux hook" entry under any event.
func TestInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	settings := readSettings(t, dir)
	for _, event := range requiredEvents {
		cmds := commandsForEvent(t, settings, event)
		count := 0
		for _, c := range cmds {
			if c == "vibemux hook" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("event %s: expected exactly 1 'vibemux hook', got %d", event, count)
		}
	}
}

// TestUninstallRemovesOnlyOurs verifies that Uninstall removes "vibemux hook"
// entries but leaves unrelated hooks (e.g., notify-send) in place.
func TestUninstallRemovesOnlyOurs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write a settings.json with both a notify-send hook and our hook under Stop.
	initial := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "notify-send 'Claude stopped'",
						},
					},
				},
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "vibemux hook",
						},
					},
				},
			},
		},
	}
	clauDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(clauDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(filepath.Join(clauDir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := hookinstall.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	settings := readSettings(t, dir)
	stopCmds := commandsForEvent(t, settings, "Stop")

	// notify-send must remain.
	if !containsCmd(stopCmds, "notify-send 'Claude stopped'") {
		t.Errorf("Stop: notify-send was incorrectly removed; got %v", stopCmds)
	}

	// Our hook must be gone.
	if containsCmd(stopCmds, "vibemux hook") {
		t.Errorf("Stop: 'vibemux hook' was NOT removed; got %v", stopCmds)
	}
}

// TestIsInstalledReflectsState verifies that IsInstalled returns the correct
// value before and after Install/Uninstall.
func TestIsInstalledReflectsState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Before install: missing file -> not installed.
	installed, err := hookinstall.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled (before install): %v", err)
	}
	if installed {
		t.Error("expected not installed before Install()")
	}

	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// After install: should be installed.
	installed, err = hookinstall.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled (after install): %v", err)
	}
	if !installed {
		t.Error("expected installed after Install()")
	}

	if err := hookinstall.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// After uninstall: should not be installed.
	installed, err = hookinstall.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled (after uninstall): %v", err)
	}
	if installed {
		t.Error("expected not installed after Uninstall()")
	}
}

// TestUninstallMissingFile verifies that Uninstall on a non-existent
// settings.json is a no-op and returns no error.
func TestUninstallMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := hookinstall.Uninstall(); err != nil {
		t.Fatalf("Uninstall on missing file: %v", err)
	}
}

// TestInstallCreatesBackup verifies that installing over an existing
// settings.json creates a settings.json.vibemux-bak file.
func TestInstallCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	clauDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(clauDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"someKey": "someValue"}`)
	if err := os.WriteFile(filepath.Join(clauDir, "settings.json"), original, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := hookinstall.Install("vibemux"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	bakPath := filepath.Join(clauDir, "settings.json.vibemux-bak")
	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(bak) != string(original) {
		t.Errorf("backup content mismatch: got %s, want %s", bak, original)
	}
}
