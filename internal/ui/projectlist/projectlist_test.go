package projectlist

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
	"github.com/misaelabanto/vibemux/internal/model"
)

// makeProjects builds a simple list with one project.
func makeProjects() []model.Project {
	return []model.Project{
		{ID: "proj1", Name: "Alpha", Path: "/alpha"},
	}
}

// makeProjectsMulti builds two projects so selection tests can target one.
func makeProjectsMulti() []model.Project {
	return []model.Project{
		{ID: "proj1", Name: "Alpha", Path: "/alpha"},
		{ID: "proj2", Name: "Beta", Path: "/beta"},
	}
}

// TestSetAgents_FocusedIsHighestUrgency verifies that after SetAgents with 3
// agents (one Blocked, one Working, one Done) the focused agent is the Blocked one.
func TestSetAgents_FocusedIsHighestUrgency(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Working, UpdatedAt: now},
			{SessionID: "s2", State: agent.Done, UpdatedAt: now},
			{SessionID: "s3", State: agent.Blocked, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	focused, ok := m.FocusedAgentForSelected()
	if !ok {
		t.Fatal("FocusedAgentForSelected: expected ok=true")
	}
	if focused.State != agent.Blocked {
		t.Errorf("focused agent state = %v, want Blocked", focused.State)
	}
}

// TestFocusNextAgent_AdvancesAndWraps checks that FocusNextAgent cycles through
// agents and wraps back to 0.
func TestFocusNextAgent_AdvancesAndWraps(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
			{SessionID: "s3", State: agent.Done, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	// index 0 after SetAgents (highest urgency = Blocked)
	f0, _ := m.FocusedAgentForSelected()
	if f0.State != agent.Blocked {
		t.Fatalf("expected Blocked at index 0, got %v", f0.State)
	}

	m.FocusNextAgent()
	f1, _ := m.FocusedAgentForSelected()
	if f1.State != agent.Working {
		t.Errorf("after 1st FocusNextAgent: got %v, want Working", f1.State)
	}

	m.FocusNextAgent()
	f2, _ := m.FocusedAgentForSelected()
	if f2.State != agent.Done {
		t.Errorf("after 2nd FocusNextAgent: got %v, want Done", f2.State)
	}

	// wrap
	m.FocusNextAgent()
	f3, _ := m.FocusedAgentForSelected()
	if f3.State != agent.Blocked {
		t.Errorf("after wrap FocusNextAgent: got %v, want Blocked", f3.State)
	}
}

// TestFocusPrevAgent_Wraps checks that FocusPrevAgent wraps around.
func TestFocusPrevAgent_Wraps(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	// at index 0 (Blocked), FocusPrevAgent should wrap to index 1 (Working)
	m.FocusPrevAgent()
	f, _ := m.FocusedAgentForSelected()
	if f.State != agent.Working {
		t.Errorf("FocusPrevAgent wrap: got %v, want Working", f.State)
	}
}

// TestFocusNextAgent_NoOp_LessThanTwoAgents checks that FocusNextAgent is a
// no-op when there is only one agent.
func TestFocusNextAgent_NoOp_LessThanTwoAgents(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	m.FocusNextAgent()
	f, _ := m.FocusedAgentForSelected()
	if f.State != agent.Blocked {
		t.Errorf("FocusNextAgent with 1 agent should be no-op; got %v", f.State)
	}
}

// TestUpdateLeft_ChangesFocus checks that a left key message triggers FocusPrevAgent.
func TestUpdateLeft_ChangesFocus(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	// Send left key via Update
	leftMsg := tea.KeyPressMsg{Code: tea.KeyLeft}
	m2, _ := m.Update(leftMsg)

	f, _ := m2.FocusedAgentForSelected()
	if f.State != agent.Working {
		t.Errorf("left key: got %v, want Working", f.State)
	}
}

// TestUpdateRight_ChangesFocus checks that a right key message triggers FocusNextAgent.
func TestUpdateRight_ChangesFocus(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	// Send right key via Update
	rightMsg := tea.KeyPressMsg{Code: tea.KeyRight}
	m2, _ := m.Update(rightMsg)

	f, _ := m2.FocusedAgentForSelected()
	if f.State != agent.Working {
		t.Errorf("right key: got %v, want Working", f.State)
	}
}

// TestSetGitStatus_StoredAndReflected checks that SetGitStatus stores the data
// and it is retrievable from the selected project item.
func TestSetGitStatus_StoredAndReflected(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	g := gitstatus.Status{IsRepo: true, Clean: true}
	m.SetGitStatus(map[string]gitstatus.Status{"proj1": g})

	got, ok := m.GitStatusForSelected()
	if !ok {
		t.Fatal("GitStatusForSelected: expected ok=true")
	}
	if got.IsRepo != g.IsRepo || got.Clean != g.Clean {
		t.Errorf("git status mismatch: got %+v, want %+v", got, g)
	}
}

// TestSetSettings_StoredAndReflected checks that SetSettings stores the settings.
func TestSetSettings_StoredAndReflected(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	s := config.DefaultSettings()
	s.StaleThresholdSec = 999
	m.SetSettings(s)

	got := m.Settings()
	if got.StaleThresholdSec != 999 {
		t.Errorf("Settings().StaleThresholdSec = %d, want 999", got.StaleThresholdSec)
	}
}

// TestUpdateLeftRight_IgnoredWhileFiltering checks that left/right keys do NOT
// change the focused agent index while the list is in filtering mode.
func TestUpdateLeftRight_IgnoredWhileFiltering(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)

	// Confirm starting focus is index 0 (Blocked, highest urgency).
	f0, _ := m.FocusedAgentForSelected()
	if f0.State != agent.Blocked {
		t.Fatalf("pre-condition failed: expected Blocked at index 0, got %v", f0.State)
	}

	// Enter filter mode by sending a printable character that isTypingChar accepts.
	// Update dispatches a slash followed by the char, which puts the list into Filtering.
	typingMsg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	m2, _ := m.Update(typingMsg)

	if !m2.IsFiltering() {
		t.Fatal("pre-condition failed: model is not in Filtering state after typing 'a'")
	}

	// Now send left and right - focus must NOT change.
	m3, _ := m2.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m4, _ := m3.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	f, _ := m4.FocusedAgentForSelected()
	if f.State != agent.Blocked {
		t.Errorf("focus changed while filtering: got %v, want Blocked (index 0)", f.State)
	}
}

// TestSetAgents_ResetsFocusOnUpdate checks that when agents are updated for a
// project the focused index resets to 0 (most urgent).
func TestSetAgents_ResetsFocusOnUpdate(t *testing.T) {
	m := New(makeProjects(), 80, 24)

	now := time.Now()
	agents := map[string][]agent.Status{
		"proj1": {
			{SessionID: "s1", State: agent.Blocked, UpdatedAt: now},
			{SessionID: "s2", State: agent.Working, UpdatedAt: now},
		},
	}
	m.SetAgents(agents)
	m.FocusNextAgent() // move to index 1

	// Re-set agents - should reset focus to 0
	m.SetAgents(agents)
	f, _ := m.FocusedAgentForSelected()
	if f.State != agent.Blocked {
		t.Errorf("after re-SetAgents focus should reset to 0 (Blocked), got %v", f.State)
	}
}
