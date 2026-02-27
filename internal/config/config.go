package config

import (
	"os"
	"path/filepath"
)

func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vibemux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vibemux")
}

func ProjectsFile() string {
	return filepath.Join(Dir(), "projects.json")
}

func EnsureDir() error {
	return os.MkdirAll(Dir(), 0o755)
}
