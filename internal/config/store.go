package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"vibemux/internal/model"
)

func LoadProjects() ([]model.Project, error) {
	if err := EnsureDir(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(ProjectsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []model.Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func SaveProjects(projects []model.Project) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ProjectsFile(), data, 0o644)
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
