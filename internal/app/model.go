package app

import (
	"vibemux/internal/model"
	"vibemux/internal/ui/addproject"
	"vibemux/internal/ui/projectlist"
	"vibemux/internal/ui/terminal"
)

type ViewState int

const (
	ViewProjectList ViewState = iota
	ViewAddProject
	ViewTerminal
)

type AppModel struct {
	state         ViewState
	projectList   projectlist.Model
	addProject    addproject.Model
	terminal      terminal.Model
	projects      []model.Project
	activeProject *model.Project
	width         int
	height        int
}

func NewAppModel(projects []model.Project) AppModel {
	return AppModel{
		state:       ViewProjectList,
		projectList: projectlist.New(projects, 80, 24),
		projects:    projects,
	}
}
