package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/tmux"
)

func tempXDGDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	return dir
}

// TestComputeStatusReturnsMsg verifies that calling the tea.Cmd returned by
// computeStatus produces a StatusComputedMsg (even in an empty environment).
func TestComputeStatusReturnsMsg(t *testing.T) {
	tempXDGDir(t)
	projects := []model.Project{
		{ID: "p1", Name: "proj1", Path: t.TempDir()},
	}

	cmd := computeStatus(projects, tmux.Backend{})
	if cmd == nil {
		t.Fatal("computeStatus returned nil cmd")
	}

	msg := cmd()
	scm, ok := msg.(StatusComputedMsg)
	if !ok {
		t.Fatalf("expected StatusComputedMsg, got %T", msg)
	}
	// Active map must be non-nil (no sessions in test env = empty map is fine).
	if scm.Active == nil {
		t.Error("StatusComputedMsg.Active is nil, want non-nil map")
	}
	// Git map must be non-nil.
	if scm.Git == nil {
		t.Error("StatusComputedMsg.Git is nil, want non-nil map")
	}
	// Agents map must be non-nil.
	if scm.Agents == nil {
		t.Error("StatusComputedMsg.Agents is nil, want non-nil map")
	}
}

// TestUpdateStatusComputedMsg feeds a StatusComputedMsg through Update and
// verifies no panic and that the returned model is an AppModel.
func TestUpdateStatusComputedMsg(t *testing.T) {
	tempXDGDir(t)
	projects := []model.Project{
		{ID: "p1", Name: "proj1", Path: t.TempDir()},
	}

	m := NewAppModel(projects, tmux.Backend{}, nil, "")

	active := map[string]bool{"p1": true}
	agents := map[string][]agent.Status{
		"p1": {
			{
				Cwd:       projects[0].Path,
				SessionID: "s1",
				State:     agent.Working,
				Message:   "doing stuff",
				UpdatedAt: time.Now(),
			},
		},
	}
	git := map[string]gitstatus.Status{
		"p1": {IsRepo: true, Clean: true},
	}

	msg := StatusComputedMsg{Active: active, Agents: agents, Git: git}

	result, cmd := m.Update(msg)
	if result == nil {
		t.Fatal("Update returned nil model")
	}
	if _, ok := result.(AppModel); !ok {
		t.Fatalf("Update returned %T, want AppModel", result)
	}
	// No command expected on StatusComputedMsg.
	if cmd != nil {
		t.Errorf("expected nil cmd from StatusComputedMsg, got non-nil")
	}
}

// TestUpdateTickMsgReturnsBatch verifies that TickMsg through Update returns a
// non-nil command (the batch of computeStatus + tick perpetuation).
func TestUpdateTickMsgReturnsBatch(t *testing.T) {
	tempXDGDir(t)
	projects := []model.Project{}
	m := NewAppModel(projects, tmux.Backend{}, nil, "")

	result, cmd := m.Update(TickMsg{})
	if result == nil {
		t.Fatal("Update returned nil model")
	}
	if cmd == nil {
		t.Fatal("Update on TickMsg returned nil cmd, want batch cmd")
	}
}

// TestMultiplexerReturnedReappliesScope verifies that detaching from a session
// (MultiplexerReturnedMsg) reloads projects but keeps the scope filter applied,
// rather than showing every registered project.
func TestMultiplexerReturnedReappliesScope(t *testing.T) {
	tempXDGDir(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	scopeDir := filepath.Join(root, "scope")
	inside := model.Project{ID: "in", Name: "inside", Path: filepath.Join(scopeDir, "proj-a")}
	outside := model.Project{ID: "out", Name: "outside", Path: filepath.Join(root, "other", "proj-b")}

	if err := config.SaveProjects([]model.Project{inside, outside}); err != nil {
		t.Fatalf("SaveProjects: %v", err)
	}

	scoped := model.ProjectsUnder([]model.Project{inside, outside}, scopeDir)
	m := NewAppModel(scoped, tmux.Backend{}, nil, scopeDir)

	result, _ := m.Update(MultiplexerReturnedMsg{})
	app, ok := result.(AppModel)
	if !ok {
		t.Fatalf("expected AppModel, got %T", result)
	}

	if len(app.projects) != 1 || app.projects[0].ID != "in" {
		t.Fatalf("after detach, want only scoped project [in], got %+v", app.projects)
	}
}
