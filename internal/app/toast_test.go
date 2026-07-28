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
// when nothing is showing, so the normal render path is untouched. The
// assertion compares View().Content byte-for-byte against the
// un-composited projectList.View(), rather than just probing for a border
// character: a border check would pass even if withToast dropped its
// visibility guard and started running content through the canvas anyway,
// since canvas padding or reflow does not necessarily add a "╭".
func TestViewWithoutToastIsUnchanged(t *testing.T) {
	tempXDGDir(t)
	model := NewAppModel(nil, newFakeMux(), nil, "")
	model.width, model.height = 80, 24

	if model.toast.Visible() {
		t.Fatal("a fresh AppModel must not have a visible toast")
	}

	want := model.projectList.View()
	if got := model.View().Content; got != want {
		t.Errorf("View().Content differs from the un-composited content with no toast showing:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestKeyPressReachesToastThroughAppUpdate verifies that AppModel.Update
// forwards key presses to the toast at all: showing a toast and then
// delivering a key press through the top-level Update clears it, the same
// way toast.Model.Update does on its own. The real ordering guarantee this
// task cares about, that a keypress which raises a toast must not be the
// same keypress that dismisses it, cannot be exercised yet because no
// production code calls Show; that gets covered once a later task wires
// Show into an actual handler.
func TestKeyPressReachesToastThroughAppUpdate(t *testing.T) {
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
