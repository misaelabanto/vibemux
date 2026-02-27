package projectlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"vibemux/internal/model"
)

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

type Model struct {
	list   list.Model
	width  int
	height int
}

func New(projects []model.Project, width, height int) Model {
	items := projectsToItems(projects)
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, width, height-3)
	l.Title = "Vibemux - Projects"
	l.SetShowHelp(true)
	return Model{list: l, width: width, height: height}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	help := helpStyle.Render("enter open  a add  d delete  q quit")
	return m.list.View() + "\n" + help
}

func (m Model) SelectedProject() (model.Project, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return model.Project{}, false
	}
	p, ok := item.(model.Project)
	return p, ok
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetSize(w, h-3)
}

func (m *Model) SetProjects(projects []model.Project) tea.Cmd {
	return m.list.SetItems(projectsToItems(projects))
}

func projectsToItems(projects []model.Project) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = p
	}
	return items
}
