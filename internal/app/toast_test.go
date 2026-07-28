package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
// way toast.Model.Update does on its own. The real ordering guarantee, that a
// keypress which raises a toast must not be the same keypress that dismisses
// it, is exercised by TestKillSessionThroughAppUpdateToastSurvivesOwnKeystroke
// below, which drives an actual production handler through this same
// top-level Update.
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
// still alive on the multiplexer server, and the user may need to reach
// whatever is inside it. Blocking that would prevent recovery without
// preventing any failure, since attaching does not touch the missing path.
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

// TestMultiplexerReturnedErrorToasts verifies the error that used to be
// dropped on the floor now surfaces.
func TestMultiplexerReturnedErrorToasts(t *testing.T) {
	tempXDGDir(t)
	appModel := NewAppModel(nil, newFakeMux(), nil, "")

	updated, _ := appModel.Update(MultiplexerReturnedMsg{Err: errors.New("session died")})

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no toast raised for a non-nil MultiplexerReturnedMsg.Err")
	}
	if !strings.Contains(result.toast.Message(), "session died") {
		t.Errorf("toast = %q, want it to carry the underlying error", result.toast.Message())
	}
}

// TestMultiplexerReturnedCleanDetachIsSilent verifies a normal detach, which
// exits zero, does not raise anything.
func TestMultiplexerReturnedCleanDetachIsSilent(t *testing.T) {
	tempXDGDir(t)
	appModel := NewAppModel(nil, newFakeMux(), nil, "")

	updated, _ := appModel.Update(MultiplexerReturnedMsg{Err: nil})

	if updated.(AppModel).toast.Visible() {
		t.Error("a clean detach raised a toast")
	}
}

// TestKillSessionOnInactiveProjectIsSilent guards a regression this change
// would otherwise introduce. tmux kill-session on a nonexistent session exits
// 1, so attaching a toast to an unconditional KillSession would fire a useless
// "exit status 1" every time ctrl+x is pressed on an inactive Project, where
// today it is a silent no-op.
func TestKillSessionOnInactiveProjectIsSilent(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	fake.killSessionErr = errors.New("no server running")
	project := model.Project{ID: "p1", Name: "proj", Path: t.TempDir()}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 0 {
		t.Errorf("KillSession called %d times for an inactive Project, want 0", len(fake.killSessionCalls))
	}
	if result.toast.Visible() && strings.Contains(result.toast.Message(), "no server running") {
		t.Errorf("raised a useless backend error toast: %q", result.toast.Message())
	}
}

// TestKillSessionOnActiveProjectConfirms verifies the confirmation path.
func TestKillSessionOnActiveProjectConfirms(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project := model.Project{ID: "p1", Name: "proj", Path: dir}
	fake.sessions[fake.SessionName(dir)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 1 {
		t.Fatalf("KillSession called %d times, want 1", len(fake.killSessionCalls))
	}
	if !result.toast.Visible() {
		t.Error("no confirmation toast after killing a live Session")
	}
}

// TestKillSessionThroughAppUpdateToastSurvivesOwnKeystroke drives the full
// AppModel.Update (not updateProjectList) with the ctrl+x keystroke that both
// raises the confirmation toast and, if forwarding ran after dispatch instead
// of before, would immediately dismiss it. Every other handler test in this
// file calls updateProjectList directly, which bypasses the top-level Update
// where the forward-then-dispatch ordering actually lives (update.go:34-40),
// so this is the test that exercises that ordering guarantee end to end.
func TestKillSessionThroughAppUpdateToastSurvivesOwnKeystroke(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project := model.Project{ID: "p1", Name: "proj", Path: dir}
	fake.sessions[fake.SessionName(dir)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")

	updated, _ := appModel.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("confirmation toast not visible after ctrl+x through the full AppModel.Update; the keystroke that raised it dismissed it")
	}
}

// TestRemoveProjectOnInactiveProjectSkipsKill mirrors
// TestKillSessionOnInactiveProjectIsSilent for ctrl+d: an inactive Project has
// no Session to kill, so KillSession must never be called, but the Project is
// still removed and a confirmation toast names it.
func TestRemoveProjectOnInactiveProjectSkipsKill(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project, err := config.AddProject(dir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 0 {
		t.Errorf("KillSession called %d times for an inactive Project, want 0", len(fake.killSessionCalls))
	}
	if !result.toast.Visible() {
		t.Fatal("no confirmation toast after removing an inactive Project")
	}
	if !strings.Contains(result.toast.Message(), project.Name) {
		t.Errorf("toast = %q, want it to name the removed Project %q", result.toast.Message(), project.Name)
	}
	remaining, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("LoadProjects() = %v, want the Project removed from disk", remaining)
	}
}

// TestRemoveProjectOnActiveProjectKillsSession mirrors
// TestKillSessionOnActiveProjectConfirms for ctrl+d: an active Project's
// Session must be killed before the Project is removed.
func TestRemoveProjectOnActiveProjectKillsSession(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project, err := config.AddProject(dir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	fake.sessions[fake.SessionName(dir)] = true

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if len(fake.killSessionCalls) != 1 {
		t.Fatalf("KillSession called %d times, want 1", len(fake.killSessionCalls))
	}
	if !result.toast.Visible() {
		t.Fatal("no confirmation toast after removing an active Project")
	}
	if !strings.Contains(result.toast.Message(), project.Name) {
		t.Errorf("toast = %q, want it to name the removed Project %q", result.toast.Message(), project.Name)
	}
	remaining, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("LoadProjects() = %v, want the Project removed from disk", remaining)
	}
}

// TestRemoveProjectKillFailureBlocksRemoval verifies the fix for the brief's
// bug: a KillSession failure must raise an error toast and must NOT proceed
// to RemoveProject. Without the short-circuit, a failed kill would leave an
// orphaned vmx-<dir> session with no Project left to surface it anywhere.
func TestRemoveProjectKillFailureBlocksRemoval(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project, err := config.AddProject(dir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	fake.sessions[fake.SessionName(dir)] = true
	fake.killSessionErr = errors.New("boom")

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no error toast after a failed KillSession")
	}
	if !strings.Contains(result.toast.Message(), "boom") {
		t.Errorf("toast = %q, want it to carry the underlying kill error", result.toast.Message())
	}
	if len(result.projects) != 1 {
		t.Errorf("in-memory projects = %v, want the Project left in place after a failed kill", result.projects)
	}
	remaining, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("LoadProjects() = %v, want the Project still on disk after a failed kill", remaining)
	}
}

// TestRemoveProjectSaveFailureLeavesProjectInPlace verifies that when
// config.RemoveProject itself fails (the write to projects.json errors), the
// handler raises an error toast and does not touch in-memory state, so the
// dashboard and projects.json stay in agreement.
func TestRemoveProjectSaveFailureLeavesProjectInPlace(t *testing.T) {
	tempXDGDir(t)
	fake := newFakeMux()
	dir := t.TempDir()
	project, err := config.AddProject(dir)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// Make the subsequent write in RemoveProject fail: projects.json exists
	// (AddProject just wrote it), so stripping write permission from the file
	// itself makes the os.WriteFile inside config.SaveProjects error out.
	projectsFile := config.ProjectsFile()
	if err := os.Chmod(projectsFile, 0o400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(projectsFile, 0o644) })

	appModel := NewAppModel([]model.Project{project}, fake, nil, "")
	appModel.state = ViewProjectList

	updated, _ := appModel.updateProjectList(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	result := updated.(AppModel)
	if !result.toast.Visible() {
		t.Fatal("no error toast after a failed RemoveProject")
	}
	// Must be the failure toast, not the "Removed <name>" confirmation: without
	// the short-circuit, the handler falls through to the success path and
	// claims a removal that never happened.
	if !strings.Contains(result.toast.Message(), "Could not remove Project") {
		t.Errorf("toast = %q, want the failure message, not a false confirmation", result.toast.Message())
	}
	if len(result.projects) != 1 {
		t.Errorf("in-memory projects = %v, want the Project left in place after a failed removal", result.projects)
	}

	if err := os.Chmod(projectsFile, 0o644); err != nil {
		t.Fatalf("Chmod restore: %v", err)
	}
	remaining, err := config.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("LoadProjects() = %v, want the Project still on disk after a failed removal", remaining)
	}
}

// TestViewNeverOverflowsTerminal guards the invariant the original bug
// actually violated: an error was written straight to os.Stderr while the
// alternate screen was active, and stderr does not know or care how wide or
// tall the terminal is, so a long message overflowed the frame and tore the
// rendered output. A frame line wider than the terminal, or a frame with more
// lines than the terminal is tall, is exactly what that corruption looked
// like. Now that errors are drawn through withToast's Canvas/Compositor
// instead, this test asserts the composited frame can never reproduce that
// shape.
//
// The table sweeps down to absurdly small terminals (1x1) on purpose: those
// degenerate sizes are where the toastX/toastY clamps in withToast and the
// clipping lipgloss performs while composing the Canvas are most likely to
// break, so they are the sizes most worth guarding.
func TestViewNeverOverflowsTerminal(t *testing.T) {
	tempXDGDir(t)

	longMessage := "error: " + strings.Repeat("overflow ", 40)

	sizes := []struct {
		width  int
		height int
	}{
		{80, 24},
		{40, 12},
		{20, 8},
		{10, 4},
		{5, 3},
		{1, 1},
	}

	for _, size := range sizes {
		model := NewAppModel(nil, newFakeMux(), nil, "")
		model.width, model.height = size.width, size.height
		model.projectList.SetSize(size.width, size.height)
		model.toast.Show(toast.KindError, longMessage)

		frame := model.View().Content
		lines := strings.Split(frame, "\n")

		if len(lines) > size.height {
			t.Errorf("size %dx%d: frame has %d lines, want at most %d", size.width, size.height, len(lines), size.height)
		}
		for lineIndex, line := range lines {
			if width := lipgloss.Width(line); width > size.width {
				t.Errorf("size %dx%d: line %d has width %d, want at most %d", size.width, size.height, lineIndex, width, size.width)
			}
		}
	}
}
