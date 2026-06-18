package agent

import (
	"strings"
	"testing"
)

func TestRunHook_UserPromptSubmit(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"UserPromptSubmit","cwd":"/tmp/proj","session_id":"sess-1","transcript_path":""}`
	err := RunHook(strings.NewReader(input))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	statuses, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.State != Working {
		t.Errorf("expected Working, got %s", s.State)
	}
	if s.SessionID != "sess-1" {
		t.Errorf("expected sess-1, got %s", s.SessionID)
	}
	if s.Cwd != "/tmp/proj" {
		t.Errorf("expected /tmp/proj, got %s", s.Cwd)
	}
	if s.Message != "" {
		t.Errorf("expected empty message for Working, got %q", s.Message)
	}
}

func TestRunHook_PreToolUse(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"PreToolUse","cwd":"/tmp/proj","session_id":"sess-2","transcript_path":""}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 1 || statuses[0].State != Working {
		t.Errorf("expected 1 Working status, got %+v", statuses)
	}
}

func TestRunHook_PostToolUse(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"PostToolUse","cwd":"/tmp/proj","session_id":"sess-3","transcript_path":""}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 1 || statuses[0].State != Working {
		t.Errorf("expected 1 Working status, got %+v", statuses)
	}
}

func TestRunHook_Stop(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"Stop","cwd":"/tmp/proj","session_id":"sess-4","transcript_path":"/nonexistent"}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].State != Done {
		t.Errorf("expected Done, got %s", statuses[0].State)
	}
}

func TestRunHook_Notification(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"Notification","cwd":"/tmp/proj","session_id":"sess-5","transcript_path":"/nonexistent"}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].State != Blocked {
		t.Errorf("expected Blocked, got %s", statuses[0].State)
	}
}

func TestRunHook_SessionEnd(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Write a status first.
	writeInput := `{"hook_event_name":"UserPromptSubmit","cwd":"/tmp/proj","session_id":"sess-6","transcript_path":""}`
	if err := RunHook(strings.NewReader(writeInput)); err != nil {
		t.Fatalf("unexpected error on write: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status before delete, got %d", len(statuses))
	}

	// Now delete via SessionEnd.
	deleteInput := `{"hook_event_name":"SessionEnd","cwd":"/tmp/proj","session_id":"sess-6","transcript_path":""}`
	if err := RunHook(strings.NewReader(deleteInput)); err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}

	statuses, _ = LoadAll()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses after SessionEnd, got %d", len(statuses))
	}
}

func TestRunHook_UnknownEvent(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"SomeFutureEvent","cwd":"/tmp/proj","session_id":"sess-7","transcript_path":""}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses for unknown event, got %d", len(statuses))
	}
}

func TestRunHook_MissingSessionID(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `{"hook_event_name":"UserPromptSubmit","cwd":"/tmp/proj","session_id":"","transcript_path":""}`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("expected nil for missing session_id, got %v", err)
	}

	statuses, _ := LoadAll()
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestRunHook_MalformedJSON(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	input := `not valid json {{{`
	if err := RunHook(strings.NewReader(input)); err != nil {
		t.Fatalf("expected nil for malformed JSON, got %v", err)
	}
}
