package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/app"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/mux"
)

func main() {
	projects, err := config.LoadProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading projects: %v\n", err)
		os.Exit(1)
	}

	settings, err := config.LoadSettings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading settings: %v\n", err)
		os.Exit(1)
	}

	// Non-interactive resolve: use the saved multiplexer when it is still
	// installed; otherwise leave active nil so the app onboards (first run or
	// self-heal).
	installed := mux.Installed()
	var active mux.Multiplexer
	if k, ok := mux.Active(settings.Multiplexer, installed); ok {
		active, err = mux.New(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing multiplexer: %v\n", err)
			os.Exit(1)
		}
	}

	m := app.NewAppModel(projects, active, installed)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
