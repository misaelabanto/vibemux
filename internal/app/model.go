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
	width       int
	height      int
}

// NewAppModel builds the root model. When active is nil (no validly-saved
// multiplexer) it starts in onboarding seeded with the installed set;
// otherwise it starts in the project list with active wired in.
func NewAppModel(projects []model.Project, active mux.Multiplexer, installed []mux.Kind) AppModel {
	m := AppModel{
		projectList: projectlist.New(projects, defaultWidth, defaultHeight),
		projects:    projects,
		mux:         active,
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
