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

func printIcons() {
	settings := config.LoadSettings()

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

	projects, err := config.LoadProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading projects: %v\n", err)
		os.Exit(1)
	}

	m := app.NewAppModel(projects)
	if installed, _ := hookinstall.IsInstalled(); !installed && !app.HooksDeclined() {
		m = m.WithConsentPrompt()
	}
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
