package app

import "vibemux/internal/model"

type SwitchToProjectListMsg struct{}

type OpenProjectMsg struct {
	Project model.Project
}

type ProjectAddedMsg struct {
	Project model.Project
}

type ProjectDeletedMsg struct {
	ID string
}
