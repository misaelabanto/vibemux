package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
	"github.com/misaelabanto/vibemux/internal/zellij"
)

func (m AppModel) Init() tea.Cmd {
	return refreshSessionStatus()
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
					zellij.KillSession(zellij.SessionName(p.Path))
					config.RemoveProject(p.ID)
					projects, _ := config.LoadProjects()
					m.projects = projects
					cmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(cmd, refreshSessionStatus())
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					zellij.KillSession(zellij.SessionName(p.Path))
					return m, refreshSessionStatus()
				}
			case "ctrl+a":
				cmd := m.projectList.ToggleActiveOnly()
				return m, cmd
			case "ctrl+o":
				return m.openDashboard()
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

	if !zellij.IsInstalled() {
		fmt.Fprintf(os.Stderr, "zellij is not installed\n")
		return m, nil
	}

	name := zellij.SessionName(p.Path)

	// Create a new zellij session if one doesn't already exist.
	if !zellij.HasSession(name) {
		if err := zellij.NewSession(name, p.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating zellij session: %v\n", err)
			return m, nil
		}
	}

	cmd := tea.ExecProcess(zellij.AttachCommand(name), func(err error) tea.Msg {
		return MultiplexerReturnedMsg{Err: err}
	})
	return m, cmd
}

// openDashboard opens the web dashboard: it makes sure the zellij web server
// is running and opens one browser window per active vibemux session. The TUI
// stays in the project list the whole time.
func (m AppModel) openDashboard() (tea.Model, tea.Cmd) {
	if !zellij.IsInstalled() {
		fmt.Fprintf(os.Stderr, "zellij is not installed\n")
		return m, nil
	}

	sessions, _ := zellij.ListVibemuxSessions()
	names := zellij.DashboardSessions(sessions)
	if len(names) == 0 {
		return m, m.projectList.StatusMessage("no active sessions")
	}

	if err := zellij.StartWebServer(zellij.DefaultWebPort); err != nil {
		return m, m.projectList.StatusMessage(fmt.Sprintf("dashboard error: %v", err))
	}

	token, created, err := zellij.EnsureWebToken()
	if err != nil {
		return m, m.projectList.StatusMessage(fmt.Sprintf("dashboard error: %v", err))
	}

	var openErr error
	for _, name := range names {
		if err := zellij.OpenInBrowser(zellij.SessionURL(zellij.DefaultWebPort, name)); err != nil {
			openErr = err
		}
	}
	if openErr != nil {
		return m, m.projectList.StatusMessage(fmt.Sprintf("dashboard error: %v", openErr))
	}

	status := fmt.Sprintf("dashboard opened in browser (%d sessions)", len(names))
	if created {
		// The plaintext token exists only at creation time (zellij stores
		// hashes), and the status message is transient, so the token is also
		// printed to stderr the way other errors are surfaced.
		status = fmt.Sprintf("dashboard opened (%d sessions). zellij web token (shown once): %s", len(names), token)
		fmt.Fprintf(os.Stderr, "zellij web token (shown once): %s\n", token)
	}
	return m, m.projectList.StatusMessage(status)
}

// refreshSessionStatus returns a Cmd that queries zellij for active vibemux
// sessions and sends a SessionStatusMsg.
func refreshSessionStatus() tea.Cmd {
	return func() tea.Msg {
		sessions, _ := zellij.ListVibemuxSessions()
		return SessionStatusMsg{ActiveSessions: sessions}
	}
}

// mapSessionsToProjects maps zellij session names back to project IDs.
func mapSessionsToProjects(sessions map[string]bool, projects []model.Project) map[string]bool {
	active := map[string]bool{}
	for _, p := range projects {
		name := zellij.SessionName(p.Path)
		if sessions[name] {
			active[p.ID] = true
		}
	}
	return active
}
