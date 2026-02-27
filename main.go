package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"vibemux/internal/app"
	"vibemux/internal/config"
)

func main() {
	projects, err := config.LoadProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading projects: %v\n", err)
		os.Exit(1)
	}

	m := app.NewAppModel(projects)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
