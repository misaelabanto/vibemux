package toast

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		if lipgloss.Width(line) > maxBoxWidth {
			t.Fatalf("Render(2) produced a %d-column line, want the clamp to hold it at or under %d", lipgloss.Width(line), maxBoxWidth)
		}
	}
}

// TestRenderWidthIsTotalNotInterior pins the lipgloss contract this file
// depends on: Style.Width sets the total rendered width, border and padding
// included. If a future lipgloss made Width mean interior width instead, the
// box would render wider than its budget and overflow the canvas.
//
// wantWidth is written out as the literal 60, not maxBoxWidth, on purpose: if
// this asserted against the constant it mirrors, changing maxBoxWidth would
// change the expectation right along with the bug, and the test could never
// fail no matter what the cap became. The point is to notice when the cap
// value drifts, so the check has to live outside the value being checked.
func TestRenderWidthIsTotalNotInterior(t *testing.T) {
	model := New()
	model.Show(KindError, "a short message")

	const terminalWidth = 80
	const wantWidth = 60
	got := lipgloss.Width(model.Render(terminalWidth))
	if got != wantWidth {
		t.Errorf("Render(%d) rendered %d columns, want %d", terminalWidth, got, wantWidth)
	}
}

// TestRenderWidthAtMidRangeAppliesMargin covers the plain maxWidth-screenMargin
// arithmetic on its own, at a width where neither the floor nor the cap binds.
//
// wantWidth is written out as the literal 36, not terminalWidth-screenMargin,
// on purpose: deriving it from screenMargin would make this test restate the
// production code's own arithmetic instead of checking it, so it would still
// pass even if screenMargin silently changed to the wrong value.
func TestRenderWidthAtMidRangeAppliesMargin(t *testing.T) {
	model := New()
	model.Show(KindError, "a short message")

	const terminalWidth = 40
	const wantWidth = 36
	got := lipgloss.Width(model.Render(terminalWidth))
	if got != wantWidth {
		t.Errorf("Render(%d) rendered %d columns, want %d", terminalWidth, got, wantWidth)
	}
}

func TestRenderNotVisibleIsEmpty(t *testing.T) {
	model := New()
	if out := model.Render(80); out != "" {
		t.Errorf("Render on an invisible toast = %q, want empty", out)
	}
}
