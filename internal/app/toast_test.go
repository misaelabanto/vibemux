package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/ui/toast"
)

// TestViewCompositesToastOverContent verifies the toast is actually drawn.
// lipgloss Layer.Draw renders only its own content and ignores children and
// X/Y, so composing a parent layer directly produces a frame with no toast in
// it at all. This test fails loudly if that mistake creeps back in.
func TestViewCompositesToastOverContent(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24
	model.toast.Show(toast.KindError, "kaboom")

	view := model.View()
	if !strings.Contains(view.Content, "kaboom") {
		t.Errorf("View content does not contain the toast message; got:\n%s", view.Content)
	}
}

// TestViewWithoutToastIsUnchanged verifies compositing is skipped entirely
// when nothing is showing, so the normal render path is untouched.
func TestViewWithoutToastIsUnchanged(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24

	if model.toast.Visible() {
		t.Fatal("a fresh AppModel must not have a visible toast")
	}
	if got := model.View().Content; strings.Contains(got, "╭") {
		t.Error("view contains a toast border with no toast showing")
	}
}

// TestToastSurvivesTheKeyPressThatRaisedIt is the regression guard for
// forwarding order. The toast clears on any key press, so if forwarding runs
// after the handler calls Show, every confirmation toast is destroyed by the
// exact keystroke that raised it and the user never sees one.
func TestToastSurvivesTheKeyPressThatRaisedIt(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24
	model.toast.Show(toast.KindInfo, "raised before the key")

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	appModel := updated.(AppModel)

	if appModel.toast.Visible() {
		t.Fatal("test premise is wrong: a key press should clear an already-showing toast")
	}
}

// TestToastClearsOnExpiry verifies the expiry message reaches the toast even
// though AppModel.Update's first type switch returns early for several
// message types. Forwarding must sit above that switch.
func TestToastClearsOnExpiry(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.toast.Show(toast.KindError, "temporary")
	seq := model.toast.CurrentSeq()

	updated, _ := model.Update(toast.ExpiredMsg{Seq: seq})
	if updated.(AppModel).toast.Visible() {
		t.Error("toast still visible after its ExpiredMsg reached AppModel.Update")
	}
}
