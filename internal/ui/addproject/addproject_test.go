package addproject

import (
	"os"
	"path/filepath"
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
