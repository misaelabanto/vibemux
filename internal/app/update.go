package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

func (m AppModel) Init() tea.Cmd {
	// Dispatch on the authoritative view state, like Update and View, rather
	// than re-deriving it from whether the mux is nil.
	switch m.state {
	case ViewOnboarding:
		return m.onboarding.Init()
	default:
		return refreshSessionStatus(m.mux)
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MultiplexerReturnedMsg:
		// User detached or session ended: return to project list.
		projects, _ := config.LoadProjects()
		m.projects = projects
		prevActiveOnly := m.projectList.ShowActiveOnly()
		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetShowActiveOnly(prevActiveOnly)
		m.projectList.SetActiveSessions(m.projectList.ActiveSessions())
		m.state = ViewProjectList
		return m, refreshSessionStatus(m.mux)

	case SessionStatusMsg:
		active := mapSessionsToProjects(msg.ActiveSessions, m.projects, m.mux)
		m.projectList.SetActiveSessions(active)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.projectList.SetSize(msg.Width, msg.Height)
		return m, nil
	}

	switch m.state {
	case ViewOnboarding:
		return m.updateOnboarding(msg)
	case ViewProjectList:
		return m.updateProjectList(msg)
	case ViewAddProject:
		return m.updateAddProject(msg)
	}

	return m, nil
}

// updateOnboarding routes input to the onboarding sub-model and, once the
// user has chosen a multiplexer, persists it, builds the backend, and enters
// the project list. Quitting onboarding exits vibemux (it cannot run without
// a multiplexer).
func (m AppModel) updateOnboarding(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.onboarding, cmd = m.onboarding.Update(msg)

	if m.onboarding.Quit() {
		return m, tea.Quit
	}

	if k, ok := m.onboarding.Chosen(); ok {
		_ = config.SaveSettings(config.Settings{Multiplexer: string(k)})
		active, err := mux.New(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing %s: %v\n", k, err)
			return m, tea.Quit
		}
		m.mux = active
		m.state = ViewProjectList
		m.projectList.SetSize(m.width, m.height)
		return m, tea.Batch(cmd, refreshSessionStatus(m.mux))
	}

	return m, cmd
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
					m.mux.KillSession(m.mux.SessionName(p.Path))
					config.RemoveProject(p.ID)
					projects, _ := config.LoadProjects()
					m.projects = projects
					cmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(cmd, refreshSessionStatus(m.mux))
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					m.mux.KillSession(m.mux.SessionName(p.Path))
					return m, refreshSessionStatus(m.mux)
				}
			case "ctrl+a":
				cmd := m.projectList.ToggleActiveOnly()
				return m, cmd
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

	if !m.mux.IsInstalled() {
		fmt.Fprintf(os.Stderr, "%s is not installed\n", m.mux.Name())
		return m, nil
	}

	name := m.mux.SessionName(p.Path)

	// Create a new session if one doesn't already exist.
	if !m.mux.HasSession(name) {
		if err := m.mux.NewSession(name, p.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s session: %v\n", m.mux.Name(), err)
			return m, nil
		}
	}

	cmd := tea.ExecProcess(m.mux.AttachCommand(name), func(err error) tea.Msg {
		return MultiplexerReturnedMsg{Err: err}
	})
	return m, cmd
}

// refreshSessionStatus returns a Cmd that queries the active multiplexer for
// active vibemux sessions and sends a SessionStatusMsg.
func refreshSessionStatus(mx mux.Multiplexer) tea.Cmd {
	return func() tea.Msg {
		sessions, _ := mx.ListVibemuxSessions()
		return SessionStatusMsg{ActiveSessions: sessions}
	}
}

// mapSessionsToProjects maps live multiplexer session names back to project
// IDs.
func mapSessionsToProjects(sessions map[string]bool, projects []model.Project, mx mux.Multiplexer) map[string]bool {
	active := map[string]bool{}
	for _, p := range projects {
		name := mx.SessionName(p.Path)
		if sessions[name] {
			active[p.ID] = true
		}
	}
	return active
}
