package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// lastSentenceFromText tests

func TestLastSentenceFromText_MultiSentence(t *testing.T) {
	got := lastSentenceFromText("I refactored it. All tests pass. Want me to commit?")
	want := "Want me to commit?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentenceFromText_TrailingPeriod(t *testing.T) {
	got := lastSentenceFromText("All tests pass.")
	want := "All tests pass."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentenceFromText_NewlineCollapse(t *testing.T) {
	got := lastSentenceFromText("Done.\nWant me to commit?")
	want := "Want me to commit?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentenceFromText_SingleNoperiod(t *testing.T) {
	got := lastSentenceFromText("All done")
	want := "All done"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentenceFromText_MarkdownStripped(t *testing.T) {
	text := "# Heading\n\n- bullet one\n* bullet two\nRegular sentence. Final line."
	got := lastSentenceFromText(text)
	want := "Final line."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentenceFromText_FencedCodeBlock(t *testing.T) {
	text := "Here is the code:\n```go\nfunc main() {}\n```\nAll done."
	got := lastSentenceFromText(text)
	want := "All done."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// LastSentence (file-based) tests

func TestLastSentence_NormalTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	lines := `{"type":"system","content":"startup"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I refactored it. All tests pass. Want me to commit?"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LastSentence(path)
	want := "Want me to commit?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentence_FallbackToAiTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	// Last assistant record has only a code block (no prose text after strip), so falls back to aiTitle
	lines := `{"type":"ai-title","aiTitle":"Refactor auth module","sessionId":"abc123"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + "```go\npackage main\n\nfunc veryLongFunctionNameThatHasNoSpaces() {}\n```" + `"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LastSentence(path)
	want := "Refactor auth module"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastSentence_MissingFile(t *testing.T) {
	got := LastSentence("/nonexistent/path/transcript.jsonl")
	if got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}
}
