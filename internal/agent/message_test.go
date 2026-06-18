package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// scanTranscript / LastSentenceFinal tests

// A turn that ends with a text block (after earlier tool_use) is complete.
func TestScanTranscript_EndsWithText_TextAfterToolUse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Now update the call site."}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Build passes. Want me to launch it?"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scanTranscript(path)
	if !s.endsWithText {
		t.Errorf("endsWithText = false, want true")
	}
	if got := sentenceFrom(s); got != "Want me to launch it?" {
		t.Errorf("sentenceFrom = %q, want %q", got, "Want me to launch it?")
	}
}

// A transcript where the last assistant activity is a tool_use (the final text
// block not yet flushed) is incomplete: endsWithText must be false so the Stop
// handler waits instead of capturing the mid-turn preamble.
func TestScanTranscript_EndsWithText_ToolUseAfterText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Now update the call site."}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := scanTranscript(path); s.endsWithText {
		t.Errorf("endsWithText = true, want false (turn still mid tool_use)")
	}
}

// On an incomplete transcript LastSentenceFinal should give up after the
// timeout and return the best-available sentence rather than blocking forever.
func TestLastSentenceFinal_TimesOutAndFallsBack(t *testing.T) {
	prevTimeout, prevStep := transcriptSettleTimeout, transcriptSettleStep
	transcriptSettleTimeout, transcriptSettleStep = 60*time.Millisecond, 15*time.Millisecond
	defer func() { transcriptSettleTimeout, transcriptSettleStep = prevTimeout, prevStep }()

	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Working on it."}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LastSentenceFinal(path); got != "Working on it." {
		t.Errorf("LastSentenceFinal = %q, want %q", got, "Working on it.")
	}
}

// When the turn is already complete, LastSentenceFinal returns immediately.
func TestLastSentenceFinal_CompleteReturnsFinal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	lines := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Preamble before tools."}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done. Ship it?"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LastSentenceFinal(path); got != "Ship it?" {
		t.Errorf("LastSentenceFinal = %q, want %q", got, "Ship it?")
	}
}

func TestLastSentence_FallbackWhenSegmentLooksLikeCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	// Last assistant message has a long token with no spaces, which looksLikeCode will detect.
	// The fallback should return the aiTitle instead of this code-like segment.
	lines := `{"type":"ai-title","aiTitle":"Fallback Title Here","sessionId":"s1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a_very_long_identifier_that_has_no_spaces_at_all_here_xx"}]}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LastSentence(path)
	want := "Fallback Title Here"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
