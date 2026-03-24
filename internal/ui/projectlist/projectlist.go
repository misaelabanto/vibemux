package projectlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/misaelabanto/vibemux/internal/model"
)

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// projectItem wraps a model.Project to add an active session indicator.
type projectItem struct {
	model.Project
	active bool
}

func (p projectItem) Title() string {
	if p.active {
		return "● " + p.Project.Name
	}
	return p.Project.Name
}

func (p projectItem) Description() string { return p.Project.Path }
func (p projectItem) FilterValue() string { return p.Project.Name }

type Model struct {
	list           list.Model
	activeSessions map[string]bool // project ID → has active tmux session
	width          int
	height         int
}

func New(projects []model.Project, width, height int) Model {
	m := Model{width: width, height: height, activeSessions: map[string]bool{}}
	items := m.projectsToItems(projects)
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, width, height-3)
	l.Title = "Vibemux - Projects"
	l.SetShowHelp(true)
	m.list = l
	return m
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
	help := helpStyle.Render("enter open  a add  d delete  x kill session  q quit")
	return m.list.View() + "\n" + help
}

func (m Model) SelectedProject() (model.Project, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return model.Project{}, false
	}
	if pi, ok := item.(projectItem); ok {
		return pi.Project, true
	}
	if p, ok := item.(model.Project); ok {
		return p, true
	}
	return model.Project{}, false
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetSize(w, h-3)
}

func (m *Model) SetProjects(projects []model.Project) tea.Cmd {
	return m.list.SetItems(m.projectsToItems(projects))
}

// SetActiveSessions updates which projects have running tmux sessions and
// rebuilds the list items with indicators.
func (m *Model) SetActiveSessions(active map[string]bool) {
	m.activeSessions = active
	// Rebuild items with current active state.
	var projects []model.Project
	for _, item := range m.list.Items() {
		switch v := item.(type) {
		case projectItem:
			projects = append(projects, v.Project)
		case model.Project:
			projects = append(projects, v)
		}
	}
	m.list.SetItems(m.projectsToItems(projects))
}

// ActiveSessions returns the current active sessions map.
func (m Model) ActiveSessions() map[string]bool {
	return m.activeSessions
}

func (m Model) projectsToItems(projects []model.Project) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{
			Project: p,
			active:  m.activeSessions[p.ID],
		}
	}
	return items
}
