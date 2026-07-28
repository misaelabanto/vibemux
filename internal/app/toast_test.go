package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
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

// TestOpenMissingDirectoryToastsAndStops verifies the pre-check names the
// cause and never lets the multiplexer try to chdir into a directory that is
// not there. That attempt is what produced the bare "exit status 1" written
// straight to a live alternate screen.
func TestOpenMissingDirectoryToastsAndStops(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	missing := filepath.Join(t.TempDir(), "deleted-project")
	project := model.Project{ID: "p1", Name: "deleted", Path: missing}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, _ := appModel.openProject(project)
	result := updated.(AppModel)

	if !result.toast.Visible() {
		t.Fatal("no toast raised for a missing Project directory")
	}
	if !strings.Contains(result.toast.Message(), missing) {
		t.Errorf("toast = %q, want it to name the missing path %q", result.toast.Message(), missing)
	}
	if len(fake.newSessionCalls) != 0 {
		t.Errorf("NewSession was called %v times, want 0: the pre-check must return first", len(fake.newSessionCalls))
	}
}

// TestOpenMissingDirectoryDoesNotTouchProject verifies a failed open does not
// bump the Project's last-used timestamp, which would reorder the dashboard
// to reward a Project that could not be opened.
func TestOpenMissingDirectoryDoesNotTouchProject(t *testing.T) {
	tempXDGDir(t)
	existing := t.TempDir()
	project, err := config.AddProject(existing)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if err := os.RemoveAll(existing); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	before, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}

	appModel := NewAppModel([]model.Project{project}, newFakeMux(), nil, "")
	appModel.openProject(project)

	after, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("expected exactly one Project before and after, got %d and %d", len(before), len(after))
	}
	if !before[0].LastUsed.Equal(after[0].LastUsed) {
		t.Errorf("LastUsed changed from %v to %v after a failed open", before[0].LastUsed, after[0].LastUsed)
	}
}

// TestOpenMissingDirectoryWithLiveSessionStillAttaches verifies the pre-check
// gates Session creation only. When a Session is already alive its process is
// running on the multiplexer server, and the user may need to reach whatever
// is inside it. Blocking that would prevent recovery without preventing any
// failure, since attaching does not touch the missing path.
func TestOpenMissingDirectoryWithLiveSessionStillAttaches(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	missing := filepath.Join(t.TempDir(), "deleted-project")
	project := model.Project{ID: "p1", Name: "deleted", Path: missing}
	fake.sessions[fake.SessionName(missing)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, cmd := appModel.openProject(project)

	if updated.(AppModel).toast.Visible() {
		t.Errorf("toast raised for a live Session: %q", updated.(AppModel).toast.Message())
	}
	if cmd == nil {
		t.Error("openProject returned a nil cmd, want the attach command")
	}
}

// TestOpenNotInstalledToasts covers the multiplexer-not-installed path that
// used to write to stderr.
func TestOpenNotInstalledToasts(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	fake.installed = false
	project := model.Project{ID: "p1", Name: "proj", Path: t.TempDir()}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	updated, _ := appModel.openProject(project)

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no toast raised when the multiplexer is not installed")
	}
	if !strings.Contains(result.toast.Message(), "not installed") {
		t.Errorf("toast = %q, want it to mention the multiplexer is not installed", result.toast.Message())
	}
}
