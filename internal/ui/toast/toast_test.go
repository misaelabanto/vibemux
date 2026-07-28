package toast

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShowMakesVisible(t *testing.T) {
	model := New()
	if model.Visible() {
		t.Fatal("a new toast model must not be visible")
	}
	cmd := model.Show(KindError, "boom")
	if !model.Visible() {
		t.Error("Visible() = false after Show, want true")
	}
	if model.Message() != "boom" {
		t.Errorf("Message() = %q, want %q", model.Message(), "boom")
	}
	if cmd == nil {
		t.Error("Show returned a nil cmd, want the auto-dismiss tick")
	}
}

// TestMatchingExpiryClears verifies the happy path of the auto-dismiss timer.
func TestMatchingExpiryClears(t *testing.T) {
	model := New()
	model.Show(KindError, "boom")

	model, _ = model.Update(ExpiredMsg{Seq: model.seq})
	if model.Visible() {
		t.Error("Visible() = true after a matching ExpiredMsg, want false")
	}
}

// TestStaleExpiryLeavesNewerToast is the regression guard for the sequence
// counter. Without it, a toast raised just before an older toast's timer
// fires would be wiped by that older timer.
func TestStaleExpiryLeavesNewerToast(t *testing.T) {
	model := New()
	model.Show(KindError, "first")
	staleSeq := model.seq
	model.Show(KindInfo, "second")

	model, _ = model.Update(ExpiredMsg{Seq: staleSeq})
	if !model.Visible() {
		t.Fatal("Visible() = false, the stale timer wiped the newer toast")
	}
	if model.Message() != "second" {
		t.Errorf("Message() = %q, want %q", model.Message(), "second")
	}
}

func TestKeyPressClears(t *testing.T) {
	model := New()
	model.Show(KindInfo, "hello")

	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if model.Visible() {
		t.Error("Visible() = true after a key press, want false")
	}
}

func TestRenderIncludesMessage(t *testing.T) {
	model := New()
	model.Show(KindError, "boom")

	out := model.Render(80)
	if !strings.Contains(out, "boom") {
		t.Errorf("Render(80) = %q, want it to contain %q", out, "boom")
	}
}

// TestRenderTinyWidthDoesNotAutosize guards the clamp lower bound. A plain
// min(60, maxWidth-4) goes non-positive below width 5, and lipgloss treats
// Width(0) as "autosize to content", so the cap would silently vanish.
func TestRenderTinyWidthDoesNotAutosize(t *testing.T) {
	model := New()
	model.Show(KindError, strings.Repeat("wide ", 40))

	out := model.Render(2)
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 40 {
			t.Fatalf("Render(2) produced a %d-rune line, want the clamp to hold it near the 20-column floor", len([]rune(line)))
		}
	}
}

func TestRenderNotVisibleIsEmpty(t *testing.T) {
	model := New()
	if out := model.Render(80); out != "" {
		t.Errorf("Render on an invisible toast = %q, want empty", out)
	}
}
