package addproject

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyPress(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	return tea.KeyPressMsg{Text: s}
}

// On the parent picker, "Create empty folder" should treat the focused
// subdirectory as the parent rather than the opened directory.
func TestCreateEmptyUsesFocusedFolderAsParent(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	model := New(root)
	model.mode = ModeCreateEmpty
	model.step = stepPickParent
	model.loadEntries()

	// Focus the second entry ("beta") and press enter.
	model, _ = model.Update(keyPress("down"))
	model, _ = model.Update(keyPress("enter"))

	if model.step != stepEnterName {
		t.Fatalf("expected stepEnterName, got %v", model.step)
	}
	want := filepath.Join(root, "beta")
	if model.parentDir != want {
		t.Errorf("parentDir = %q, want %q", model.parentDir, want)
	}
	if model.nameInput.parent != want {
		t.Errorf("nameInput.parent = %q, want %q", model.nameInput.parent, want)
	}
}

// With no subdirectories to focus, enter falls back to the opened directory.
func TestCreateEmptyFallsBackToCurrentDirWhenEmpty(t *testing.T) {
	root := t.TempDir()

	model := New(root)
	model.mode = ModeCreateEmpty
	model.step = stepPickParent
	model.loadEntries()

	model, _ = model.Update(keyPress("enter"))

	if model.parentDir != root {
		t.Errorf("parentDir = %q, want %q", model.parentDir, root)
	}
}

// The parent picker must size its viewport from the terminal height it is
// given, so a tall terminal shows every subdirectory instead of the ten-row
// fallback used when no size has been received.
func TestPickerViewportFollowsTerminalHeight(t *testing.T) {
	root := t.TempDir()
	const dirCount = 20
	for i := 0; i < dirCount; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("proj%02d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	model := New(root)
	model.mode = ModePickExisting
	model.step = stepPickParent
	model.SetSize(120, 50)

	view := model.View()
	for i := 0; i < dirCount; i++ {
		name := fmt.Sprintf("proj%02d", i)
		if !strings.Contains(view, name) {
			t.Errorf("%s missing from picker view", name)
		}
	}
}

func TestNewPrefersRealCasingOfHomeCodeDir(t *testing.T) {
	// os.Stat succeeds for any casing on a case-insensitive filesystem, so
	// probing "Code" before "code" used to report a lowercase ~/code as ~/Code
	// and store new projects under that miscased path.
	home := t.TempDir()
	realDir := filepath.Join(home, "code")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	if got := New("").currentDir; got != realDir {
		t.Errorf("expected picker to start at %q, got %q", realDir, got)
	}
}
