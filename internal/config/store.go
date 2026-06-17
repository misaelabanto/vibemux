package config

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/misaelabanto/vibemux/internal/model"
)

func LoadProjects() ([]model.Project, error) {
	return readJSON[[]model.Project](ProjectsFile())
}

func SaveProjects(projects []model.Project) error {
	return writeJSON(ProjectsFile(), projects)
}

func AddProject(path string) (model.Project, error) {
	projects, err := LoadProjects()
	if err != nil {
		return model.Project{}, err
	}

	p := model.Project{
		ID:       uuid.New().String(),
		Name:     filepath.Base(path),
		Path:     path,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}

	projects = append(projects, p)
	return p, SaveProjects(projects)
}

func RemoveProject(id string) error {
	projects, err := LoadProjects()
	if err != nil {
		return err
	}

	filtered := projects[:0]
	for _, p := range projects {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return SaveProjects(filtered)
}

func TouchProject(id string) error {
	projects, err := LoadProjects()
	if err != nil {
		return err
	}

	for i := range projects {
		if projects[i].ID == id {
			projects[i].LastUsed = time.Now()
			break
		}
	}
	return SaveProjects(projects)
}
