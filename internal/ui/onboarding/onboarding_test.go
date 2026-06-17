package onboarding

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/mux"
)

func enter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func down() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func press(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestConfirmSingleChoosesOnlyInstalled(t *testing.T) {
	m := New([]mux.Kind{mux.Tmux})
	if _, ok := m.Chosen(); ok {
		t.Fatal("Chosen() true before any key press")
	}
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Tmux {
		t.Errorf("after enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Tmux)
	}
}

func TestSelectMovesAndChooses(t *testing.T) {
	m := New([]mux.Kind{mux.Tmux, mux.Zellij})
	m, _ = m.Update(down())
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Zellij {
		t.Errorf("after down+enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Zellij)
	}
}

func TestInstallQuit(t *testing.T) {
	m := New(nil)
	if m.Quit() {
		t.Fatal("Quit() true before any key press")
	}
	m, _ = m.Update(press('q'))
	if !m.Quit() {
		t.Error("after q on install screen: Quit() = false, want true")
	}
}

func TestInstallRecheckReturnsCmd(t *testing.T) {
	m := New(nil)
	_, cmd := m.Update(press('r'))
	if cmd == nil {
		t.Error("pressing r on install screen returned a nil cmd, want a re-detect cmd")
	}
}

func TestReDetectAdvancesPastInstall(t *testing.T) {
	m := New(nil) // starts on the install screen
	m, _ = m.Update(reDetectedMsg{installed: []mux.Kind{mux.Zellij}})
	// Now exactly one is installed: enter should choose it.
	m, _ = m.Update(enter())
	got, ok := m.Chosen()
	if !ok || got != mux.Zellij {
		t.Errorf("after re-detect+enter: Chosen() = (%q, %v), want (%q, true)", got, ok, mux.Zellij)
	}
}

func TestInstallHint(t *testing.T) {
	if got := installHint(mux.Zellij, "darwin"); !strings.Contains(got, "brew install zellij") {
		t.Errorf("installHint(zellij, darwin) = %q, want it to mention brew", got)
	}
	if got := installHint(mux.Tmux, "linux"); !strings.Contains(got, "tmux") {
		t.Errorf("installHint(tmux, linux) = %q, want it to mention tmux", got)
	}
}
