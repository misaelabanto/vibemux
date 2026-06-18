package app

import tea "charm.land/bubbletea/v2"

const consentPrompt = `Enable agent status tracking?

This adds 'vibemux hook' to your Claude Code settings (~/.claude/settings.json),
so vibemux can show which projects have an agent working, done, or blocked.
Your existing hooks are preserved.

[y] enable   [n] no, do not ask again   [any other key] not now`

func (m AppModel) View() tea.View {
	var content string

	switch m.state {
	case ViewProjectList:
		content = m.projectList.View()
	case ViewAddProject:
		content = m.addProject.View()
	case ViewConsent:
		content = consentPrompt
	case ViewOnboarding:
		content = m.onboarding.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
