package app

import (
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

type ViewState int

const (
	ViewProjectList ViewState = iota
	ViewAddProject
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
