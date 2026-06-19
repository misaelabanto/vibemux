package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/app"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/hookinstall"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
)

func printIcons() {
	settings, _ := config.LoadSettings()

	fmt.Println("Agent states:")
	agentKeys := []string{"working", "done", "blocked", "stale", "active", "no_git"}
	agentLabels := map[string]string{
		"working": "working",
		"done":    "done",
		"blocked": "blocked",
		"stale":   "stale",
		"active":  "active",
		"no_git":  "no-git",
	}
	for _, key := range agentKeys {
		icon := settings.Icons[key]
		label := agentLabels[key]
		fmt.Printf("  %-10s %s\n", label, icon)
	}

	fmt.Println("Git glyphs:")
	type glyphEntry struct {
		label string
		glyph string
	}
	glyphs := []glyphEntry{
		{"clean", "✔"},
		{"modified", "✚"},
		{"staged", "●"},
		{"untracked", "…"},
		{"stashed", "⚑"},
		{"conflict", "✖"},
		{"ahead", "↑"},
		{"behind", "↓"},
		{"diverged", "<>"},
		{"in-sync", "="},
	}
	for _, g := range glyphs {
		fmt.Printf("  %-10s %s\n", g.label, g.glyph)
	}
}

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

	if len(os.Args) > 1 && os.Args[1] == "icons" {
		printIcons()
		return
	}

	// A non-subcommand first argument scopes the TUI to registered projects at
	// or under that folder. The list opens empty when nothing matches (and even
	// when the folder does not exist); scopeDir is "" when no argument is given.
	var scopeDir string
	if len(os.Args) > 1 {
		if abs, absErr := filepath.Abs(os.Args[1]); absErr == nil {
			scopeDir = filepath.Clean(abs)
		} else {
			scopeDir = filepath.Clean(os.Args[1])
		}
	}

	projects, err := config.LoadProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading projects: %v\n", err)
		os.Exit(1)
	}
	projects = model.ProjectsUnder(projects, scopeDir)

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

	m := app.NewAppModel(projects, active, installed, scopeDir)
	// Show the hook-consent prompt only when no onboarding is needed (a
	// multiplexer is already active) and the user has neither installed nor
	// declined the hooks.
	if active != nil {
		if hooked, _ := hookinstall.IsInstalled(); !hooked && !app.HooksDeclined() {
			m = m.WithConsentPrompt()
		}
	}
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
