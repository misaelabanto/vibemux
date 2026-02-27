package app

import tea "charm.land/bubbletea/v2"

func (m AppModel) View() tea.View {
	var content string

	switch m.state {
	case ViewProjectList:
		content = m.projectList.View()
	case ViewAddProject:
		content = m.addProject.View()
	case ViewTerminal:
		content = m.terminal.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
