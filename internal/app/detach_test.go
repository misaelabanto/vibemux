package app

import (
	"testing"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/tmux"
)

// detachModel builds an app model showing two projects, one of which has a live
// session, with the status a completed sweep would have produced already applied.
func detachModel(t *testing.T) AppModel {
	t.Helper()
	tempXDGDir(t)
	// Redirect the config dir too. Without this, SaveProjects below writes to the
	// real projects.json and destroys the user's project list.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	projects := []model.Project{
		{ID: "a", Name: "alpha", Path: t.TempDir()},
		{ID: "b", Name: "beta", Path: t.TempDir()},
	}
	if err := config.SaveProjects(projects); err != nil {
		t.Fatalf("SaveProjects: %v", err)
	}

	m := NewAppModel(projects, tmux.Backend{}, nil, "")
	m.projectList.SetActiveSessions(map[string]bool{"a": true})
	m.projectList.SetAgents(map[string][]agent.Status{"a": {{Cwd: projects[0].Path, State: agent.Working}}})
	m.projectList.SetGitStatus(map[string]gitstatus.Status{
		"a": {IsRepo: true, Clean: true},
		"b": {IsRepo: true, Modified: true},
	})
	return m
}

// TestDetachKeepsProjectsVisibleWithActiveFilter is the regression test for the
// blank dashboard after detaching with the active-only filter on.
//
// The handler rebuilds the project list, and a fresh list knows about no active
// sessions at all. With the filter on, every project is therefore filtered out,
// so the dashboard sits empty until the next status sweep finishes: on a large
// project list that is a visible, seconds-long gap where the projects are gone.
func TestDetachKeepsProjectsVisibleWithActiveFilter(t *testing.T) {
	m := detachModel(t)
	m.projectList.SetShowActiveOnly(true)

	if got := len(m.projectList.VisibleItems()); got != 1 {
		t.Fatalf("before detach: %d visible items, want 1 (the active project)", got)
	}

	result, _ := m.Update(MultiplexerReturnedMsg{})
	after, ok := result.(AppModel)
	if !ok {
		t.Fatalf("expected AppModel, got %T", result)
	}

	if !after.projectList.ShowActiveOnly() {
		t.Fatal("after detach: active-only filter was lost")
	}
	if got := len(after.projectList.VisibleItems()); got != 1 {
		t.Errorf("after detach: %d visible items, want 1; the dashboard is blank until the next sweep", got)
	}
}

// TestDetachKeepsStatusUntilNextSweep covers the same rebuild for the unfiltered
// case: the previously computed status must survive so the dashboard does not
// flash rows stripped of their session, agent, and git indicators.
func TestDetachKeepsStatusUntilNextSweep(t *testing.T) {
	m := detachModel(t)

	result, _ := m.Update(MultiplexerReturnedMsg{})
	after, ok := result.(AppModel)
	if !ok {
		t.Fatalf("expected AppModel, got %T", result)
	}

	if !after.projectList.ActiveSessions()["a"] {
		t.Error("after detach: active session for project a was dropped")
	}
	if got, ok := after.projectList.GitStatusFor("b"); !ok || !got.Modified {
		t.Errorf("after detach: git status for project b = %+v (present=%v), want Modified", got, ok)
	}
	if got := after.projectList.AgentsFor("a"); len(got) != 1 {
		t.Errorf("after detach: %d agents for project a, want 1", len(got))
	}
}
