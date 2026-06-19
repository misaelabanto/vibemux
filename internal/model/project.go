package model

import (
	"path/filepath"
	"strings"
	"time"
)

type Project struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Created  time.Time `json:"created_at"`
	LastUsed time.Time `json:"last_used"`
}

// Implement list.DefaultItem interface
func (p Project) Title() string       { return p.Name }
func (p Project) Description() string { return p.Path }
// FilterValue includes both name and path so list filtering can match a custom
// name, the leaf directory, or any parent directory in the full path.
func (p Project) FilterValue() string { return p.Name + " " + p.Path }

// ProjectsUnder returns the projects whose Path is dir itself or nested anywhere
// beneath it. dir is expected to be absolute and cleaned. When dir is "", all
// projects are returned unchanged so the unscoped case is a no-op.
func ProjectsUnder(projects []Project, dir string) []Project {
	if dir == "" {
		return projects
	}
	prefix := ".." + string(filepath.Separator)
	filtered := make([]Project, 0, len(projects))
	for _, p := range projects {
		rel, err := filepath.Rel(dir, p.Path)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, prefix)) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
