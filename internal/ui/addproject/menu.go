package addproject

import (
	tea "charm.land/bubbletea/v2"
)

type Mode int

const (
	ModePickExisting Mode = iota
	ModeCreateEmpty
	ModeClone
)

type menuItem struct {
	mode  Mode
	label string
}

var menuItems = []menuItem{
	{ModePickExisting, "Pick existing folder"},
	{ModeCreateEmpty, "Create empty folder"},
	{ModeClone, "Clone GitHub repo"},
}

type menuModel struct {
	cursor   int
	chosen   bool
	choice   Mode
	canceled bool
}

func newMenu() menuModel {
	return menuModel{}
}

func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen = true
		m.choice = menuItems[m.cursor].mode
	case "esc":
		m.canceled = true
	}
	return m, nil
}

func (m menuModel) View() string {
	out := "\n  Add a project:\n\n"
	for i, it := range menuItems {
		if i == m.cursor {
			out += cursorStyle.Render("> "+it.label) + "\n"
		} else {
			out += "  " + it.label + "\n"
		}
	}
	out += "\n" + helpStyle.Render("  ↑↓ move  enter select  esc back")
	return out + "\n"
}
