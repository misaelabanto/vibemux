package model

import "time"

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
