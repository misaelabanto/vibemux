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
func (p Project) FilterValue() string { return p.Name }
