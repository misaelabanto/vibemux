package agent_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/misaelabanto/vibemux/internal/agent"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	// Clear any other XDG vars so resolution is deterministic.
	t.Setenv("XDG_STATE_HOME", "")
}

func TestWriteThenLoadAll(t *testing.T) {
	setup(t)

	s := agent.Status{
		Cwd:       "/home/user/project",
		SessionID: "abc123",
		State:     agent.Working,
		Message:   "running tests",
		UpdatedAt: time.Now(),
	}

	if err := agent.Write(s); err != nil {
		t.Fatalf("Write: %v", err)
	}

	all, err := agent.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 status, got %d", len(all))
	}
	got := all[0]
	if got.SessionID != s.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, s.SessionID)
	}
	if got.State != s.State {
		t.Errorf("State: got %q, want %q", got.State, s.State)
	}
	if got.Cwd != s.Cwd {
		t.Errorf("Cwd: got %q, want %q", got.Cwd, s.Cwd)
	}
	if got.Message != s.Message {
		t.Errorf("Message: got %q, want %q", got.Message, s.Message)
	}
}

func TestWriteTwiceSameSessionOverwrites(t *testing.T) {
	setup(t)

	s := agent.Status{
		Cwd:       "/home/user/project",
		SessionID: "sess1",
		State:     agent.Working,
		Message:   "first",
		UpdatedAt: time.Now(),
	}
	if err := agent.Write(s); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	s.State = agent.Done
	s.Message = "second"
	if err := agent.Write(s); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	all, err := agent.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 status after overwrite, got %d", len(all))
	}
	if all[0].Message != "second" {
		t.Errorf("expected overwritten message %q, got %q", "second", all[0].Message)
	}
}

func TestDelete(t *testing.T) {
	setup(t)

	s := agent.Status{
		Cwd:       "/home/user/project",
		SessionID: "to-delete",
		State:     agent.Blocked,
		UpdatedAt: time.Now(),
	}
	if err := agent.Write(s); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := agent.Delete(s.SessionID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := agent.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 statuses after delete, got %d", len(all))
	}
}

func TestLoadAllEmptyDir(t *testing.T) {
	setup(t)

	all, err := agent.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on empty dir: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(all))
	}
}

func TestLoadAllSkipsMalformedFile(t *testing.T) {
	setup(t)

	// Write a valid entry so we can confirm it still comes back.
	good := agent.Status{
		Cwd:       "/home/user/good",
		SessionID: "good-session",
		State:     agent.Done,
		UpdatedAt: time.Now(),
	}
	if err := agent.Write(good); err != nil {
		t.Fatalf("Write good: %v", err)
	}

	// Inject a malformed file directly into the agents dir.
	dir := agent.AgentsDir()
	badPath := filepath.Join(dir, "bad-session.json")
	if err := os.WriteFile(badPath, []byte("not json {{{{"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	all, err := agent.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll should not error on malformed file: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 valid status (malformed skipped), got %d", len(all))
	}
	if all[0].SessionID != good.SessionID {
		t.Errorf("expected session %q, got %q", good.SessionID, all[0].SessionID)
	}
}
