package app

import (
	"vibemux/internal/model"
	"vibemux/internal/ui/addproject"
	"vibemux/internal/ui/projectlist"
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
	width       int
	height      int
}

func NewAppModel(projects []model.Project) AppModel {
	return AppModel{
		state:       ViewProjectList,
		projectList: projectlist.New(projects, 80, 24),
		projects:    projects,
	}
}
