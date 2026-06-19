package app

import (
	"os"
	"path/filepath"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/hookinstall"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/onboarding"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

type ViewState int

const (
	ViewProjectList ViewState = iota
	ViewAddProject
	ViewOnboarding
	ViewConsent
)

// defaultWidth and defaultHeight seed the model before bubbletea delivers the
// first WindowSizeMsg, which immediately overrides them with the real size.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

type AppModel struct {
	state       ViewState
	projectList projectlist.Model
	addProject  addproject.Model
	onboarding  onboarding.Model
	mux         mux.Multiplexer
	projects    []model.Project
	settings    config.Settings
	scopeDir    string
	width       int
	height      int
}

// NewAppModel builds the root model. When active is nil (no validly-saved
// multiplexer) it starts in onboarding seeded with the installed set;
// otherwise it starts in the project list with active wired in. Settings are
// loaded and applied to the project list so status icons render correctly.
// scopeDir, when non-empty, is the folder the session is scoped to and is used
// to seed the add-project picker.
func NewAppModel(projects []model.Project, active mux.Multiplexer, installed []mux.Kind, scopeDir string) AppModel {
	settings, _ := config.LoadSettings()
	pl := projectlist.New(projects, defaultWidth, defaultHeight)
	pl.SetSettings(settings)

	m := AppModel{
		projectList: pl,
		projects:    projects,
		mux:         active,
		settings:    settings,
		scopeDir:    scopeDir,
		width:       defaultWidth,
		height:      defaultHeight,
	}
	if active == nil {
		m.state = ViewOnboarding
		m.onboarding = onboarding.New(installed)
	} else {
		m.state = ViewProjectList
	}
	return m
}

// WithConsentPrompt returns a copy of m with state set to ViewConsent. main
// calls this after NewAppModel when no onboarding is needed but the hook
// consent prompt should still be shown.
func (m AppModel) WithConsentPrompt() AppModel {
	m.state = ViewConsent
	return m
}

// needsConsent reports whether the hook-consent prompt should be shown: hooks
// are not installed and the user has not previously declined.
func needsConsent() bool {
	if HooksDeclined() {
		return false
	}
	installed, err := hookinstall.IsInstalled()
	if err != nil {
		return false
	}
	return !installed
}

// hooksDeclinedFile returns the path of the "user declined hooks" marker file.
func hooksDeclinedFile() string {
	return filepath.Join(config.Dir(), "hooks-declined")
}

// HooksDeclined reports whether the user has previously declined hook installation.
func HooksDeclined() bool {
	_, err := os.Stat(hooksDeclinedFile())
	return err == nil
}

// setHooksDeclined writes the declined marker file so vibemux will not ask again.
func setHooksDeclined() error {
	if err := config.EnsureDir(); err != nil {
		return err
	}
	return os.WriteFile(hooksDeclinedFile(), []byte("declined\n"), 0o644)
}
