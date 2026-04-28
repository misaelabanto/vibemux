package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/tmux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

func (m AppModel) Init() tea.Cmd {
	return refreshSessionStatus()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TmuxReturnedMsg:
		// User detached or session ended — return to project list.
		projects, _ := config.LoadProjects()
		m.projects = projects
		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetActiveSessions(m.projectList.ActiveSessions())
		m.state = ViewProjectList
		return m, refreshSessionStatus()

	case SessionStatusMsg:
		active := mapSessionsToProjects(msg.ActiveSessions, m.projects)
		m.projectList.SetActiveSessions(active)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.projectList.SetSize(msg.Width, msg.Height)
		return m, nil
	}

	switch m.state {
	case ViewProjectList:
		return m.updateProjectList(msg)
	case ViewAddProject:
		return m.updateAddProject(msg)
	}

	return m, nil
}

func (m AppModel) updateProjectList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		s := key.String()
		if s == "ctrl+c" {
			return m, tea.Quit
		}
		if s == "enter" && m.projectList.IsFiltering() {
			var cmd tea.Cmd
			m.projectList, cmd = m.projectList.Update(msg)
			if p, ok := m.projectList.SelectedProject(); ok {
				newModel, openCmd := m.openProject(p)
				return newModel, tea.Batch(cmd, openCmd)
			}
			return m, cmd
		}
		if !m.projectList.IsFiltering() {
			switch s {
			case "enter":
				if p, ok := m.projectList.SelectedProject(); ok {
					return m.openProject(p)
				}
			case "ctrl+n":
				m.state = ViewAddProject
				m.addProject = addproject.New()
				return m, m.addProject.Init()
			case "ctrl+d":
				if p, ok := m.projectList.SelectedProject(); ok {
					tmux.KillSession(tmux.SessionName(p.Path))
					config.RemoveProject(p.ID)
					projects, _ := config.LoadProjects()
					m.projects = projects
					cmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(cmd, refreshSessionStatus())
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					tmux.KillSession(tmux.SessionName(p.Path))
					return m, refreshSessionStatus()
				}
			}
		}
	}

	var cmd tea.Cmd
	m.projectList, cmd = m.projectList.Update(msg)
	return m, cmd
}

func (m AppModel) updateAddProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "ctrl+c" && !m.addProject.IsRunning() {
			m.addProject.Cancel()
			m.state = ViewProjectList
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.addProject, cmd = m.addProject.Update(msg)

	if m.addProject.Canceled() {
		m.state = ViewProjectList
		return m, nil
	}

	if path := m.addProject.SelectedPath(); path != "" {
		m.addProject.ClearSelection()
		p, err := config.AddProject(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding project: %v\n", err)
			m.state = ViewProjectList
			return m, nil
		}
		m.projects = append(m.projects, p)
		m.state = ViewProjectList
		setCmd := m.projectList.SetProjects(m.projects)
		return m, setCmd
	}

	return m, cmd
}

func (m AppModel) openProject(p model.Project) (tea.Model, tea.Cmd) {
	config.TouchProject(p.ID)

	if !tmux.IsInstalled() {
		fmt.Fprintf(os.Stderr, "tmux is not installed\n")
		return m, nil
	}

	name := tmux.SessionName(p.Path)

	// Create a new tmux session if one doesn't already exist.
	if !tmux.HasSession(name) {
		if err := tmux.NewSession(name, p.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
			return m, nil
		}
	}

	cmd := tea.ExecProcess(tmux.AttachCommand(name), func(err error) tea.Msg {
		return TmuxReturnedMsg{Err: err}
	})
	return m, cmd
}

// refreshSessionStatus returns a Cmd that queries tmux for active vibemux
// sessions and sends a SessionStatusMsg.
func refreshSessionStatus() tea.Cmd {
	return func() tea.Msg {
		sessions, _ := tmux.ListVibemuxSessions()
		return SessionStatusMsg{ActiveSessions: sessions}
	}
}

// mapSessionsToProjects maps tmux session names back to project IDs.
func mapSessionsToProjects(sessions map[string]bool, projects []model.Project) map[string]bool {
	active := map[string]bool{}
	for _, p := range projects {
		name := tmux.SessionName(p.Path)
		if sessions[name] {
			active[p.ID] = true
		}
	}
	return active
}
