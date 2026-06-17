package app

import (
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
)

type AppModel struct {
	state       ViewState
	projectList projectlist.Model
	addProject  addproject.Model
	onboarding  onboarding.Model
	mux         mux.Multiplexer
	projects    []model.Project
	width       int
	height      int
}

// NewAppModel builds the root model. When active is nil (no validly-saved
// multiplexer) it starts in onboarding seeded with the installed set;
// otherwise it starts in the project list with active wired in.
func NewAppModel(projects []model.Project, active mux.Multiplexer, installed []mux.Kind) AppModel {
	m := AppModel{
		projectList: projectlist.New(projects, 80, 24),
		projects:    projects,
		mux:         active,
		width:       80,
		height:      24,
	}
	if active == nil {
		m.state = ViewOnboarding
		m.onboarding = onboarding.New(installed)
	} else {
		m.state = ViewProjectList
	}
	return m
}
