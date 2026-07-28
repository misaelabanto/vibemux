package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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

	v := tea.NewView(m.withToast(content))
	v.AltScreen = true
	return v
}

// withToast composites the toast over content at the bottom right, leaving
// content itself untouched so nothing in the layout shifts.
//
// The compositing MUST go through lipgloss.NewCompositor. Layer.Draw renders
// only its own content string and ignores both its children and its own X/Y,
// so handing a parent layer straight to Canvas.Compose draws the base and
// silently discards the toast.
func (m AppModel) withToast(content string) string {
	if !m.toast.Visible() {
		return content
	}

	rendered := m.toast.Render(m.width)
	toastX := m.width - lipgloss.Width(rendered) - 2
	toastY := m.height - lipgloss.Height(rendered) - 2
	if toastX < 0 {
		toastX = 0
	}
	if toastY < 0 {
		toastY = 0
	}

	base := lipgloss.NewLayer(content)
	toastBox := lipgloss.NewLayer(rendered).X(toastX).Y(toastY).Z(1)

	return lipgloss.NewCanvas(m.width, m.height).
		Compose(lipgloss.NewCompositor(base, toastBox)).
		Render()
}
