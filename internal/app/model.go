package app

import (
	"os"
	"path/filepath"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

type ViewState int

const (
	ViewProjectList ViewState = iota
	ViewAddProject
	ViewConsent
)

type AppModel struct {
	state       ViewState
	projectList projectlist.Model
	addProject  addproject.Model
	projects    []model.Project
	settings    config.Settings
	width       int
	height      int
}

// WithConsentPrompt returns a copy of m with state set to ViewConsent.
// Call this after NewAppModel when the consent prompt should be shown.
func (m AppModel) WithConsentPrompt() AppModel {
	m.state = ViewConsent
	return m
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

func NewAppModel(projects []model.Project) AppModel {
	settings := config.LoadSettings()
	pl := projectlist.New(projects, 80, 24)
	pl.SetSettings(settings)
	return AppModel{
		state:       ViewProjectList,
		projectList: pl,
		projects:    projects,
		settings:    settings,
	}
}
