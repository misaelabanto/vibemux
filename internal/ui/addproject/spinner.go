package addproject

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type spinnerModel struct {
	s      spinner.Model
	status string
}

func newSpinner(status string) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return spinnerModel{s: s, status: status}
}

func (m spinnerModel) Init() tea.Cmd {
	return m.s.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (spinnerModel, tea.Cmd) {
	var cmd tea.Cmd
	m.s, cmd = m.s.Update(msg)
	return m, cmd
}

func (m spinnerModel) View() string {
	out := "\n  " + m.s.View() + "  " + m.status + "\n\n"
	out += helpStyle.Render("  ctrl+c cancel")
	return out + "\n"
}
