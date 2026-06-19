package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/tmux"
)

func TestWithConsentPrompt_SetsStateViewConsent(t *testing.T) {
	m := NewAppModel([]model.Project{}, tmux.Backend{}, nil, "")
	if m.state != ViewProjectList {
		t.Fatalf("expected ViewProjectList after NewAppModel, got %v", m.state)
	}
	mc := m.WithConsentPrompt()
	if mc.state != ViewConsent {
		t.Fatalf("expected ViewConsent after WithConsentPrompt, got %v", mc.state)
	}
	// Original should be unchanged.
	if m.state != ViewProjectList {
		t.Fatalf("WithConsentPrompt mutated the original model")
	}
}

func TestUpdateConsent_NKey_WritesDeclinedMarker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := NewAppModel([]model.Project{}, tmux.Backend{}, nil, "").WithConsentPrompt()

	if HooksDeclined() {
		t.Fatal("expected HooksDeclined() to be false before 'n' keypress")
	}

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	result, _ := m.Update(msg)
	am := result.(AppModel)

	if am.state != ViewProjectList {
		t.Fatalf("expected ViewProjectList after 'n', got %v", am.state)
	}
	if !HooksDeclined() {
		t.Fatal("expected HooksDeclined() to be true after 'n' keypress")
	}
}

func TestUpdateConsent_OtherKey_DismissesWithoutMarker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := NewAppModel([]model.Project{}, tmux.Backend{}, nil, "").WithConsentPrompt()

	msg := tea.KeyPressMsg{Code: 'q', Text: "q"}
	result, _ := m.Update(msg)
	am := result.(AppModel)

	if am.state != ViewProjectList {
		t.Fatalf("expected ViewProjectList after 'q', got %v", am.state)
	}
	if HooksDeclined() {
		t.Fatal("expected HooksDeclined() to be false after a non-n keypress")
	}
}
