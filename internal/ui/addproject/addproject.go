package addproject

import (
	"os"
	"path/filepath"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type Model struct {
	currentDir   string
	entries      []string
	cursor       int
	selectedPath string
	width        int
	height       int
}

func New() Model {
	home, _ := os.UserHomeDir()
	m := Model{currentDir: home}
	m.loadEntries()
	return m
}

func (m *Model) loadEntries() {
	entries, err := os.ReadDir(m.currentDir)
	m.entries = nil
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				m.entries = append(m.entries, e.Name())
			}
		}
		sort.Strings(m.entries)
	}
	m.cursor = 0
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "right", "space":
			if len(m.entries) > 0 {
				m.currentDir = filepath.Join(m.currentDir, m.entries[m.cursor])
				m.loadEntries()
			}
		case "left":
			parent := filepath.Dir(m.currentDir)
			if parent != m.currentDir {
				m.currentDir = parent
				m.loadEntries()
			}
		case "enter":
			m.selectedPath = m.currentDir
		}
	}
	return m, nil
}

func (m Model) View() string {
	visibleRows := m.height - 8
	if visibleRows < 1 {
		visibleRows = 10
	}

	// Determine scroll window
	start := m.cursor - visibleRows/2
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(m.entries) {
		end = len(m.entries)
		start = end - visibleRows
		if start < 0 {
			start = 0
		}
	}

	var rows string
	for i := start; i < end; i++ {
		line := "  " + m.entries[i]
		if i == m.cursor {
			line = cursorStyle.Render("> " + m.entries[i])
		} else {
			line = "  " + m.entries[i]
		}
		rows += line + "\n"
	}
	if len(m.entries) == 0 {
		rows = dimStyle.Render("  (no subdirectories)") + "\n"
	}

	path := dimStyle.Render("  " + m.currentDir)
	help := helpStyle.Render("  ↑↓ move  space/→ enter  ← back  enter select  ctrl+c cancel")

	return "\n  Pick a project directory:\n" + path + "\n\n" + rows + "\n" + help + "\n"
}

func (m Model) SelectedPath() string {
	return m.selectedPath
}

func (m *Model) ClearSelection() {
	m.selectedPath = ""
}
