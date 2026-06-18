package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/app"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/hookinstall"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		if err := agent.RunHook(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "install-hooks" {
		if err := hookinstall.Install("vibemux"); err != nil {
			fmt.Fprintln(os.Stderr, "install-hooks failed:", err)
			os.Exit(1)
		}
		fmt.Println("vibemux hooks installed into", hookinstall.SettingsPath())
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "uninstall-hooks" {
		if err := hookinstall.Uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall-hooks failed:", err)
			os.Exit(1)
		}
		fmt.Println("vibemux hooks removed from", hookinstall.SettingsPath())
		return
	}

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
