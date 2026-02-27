package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"vibemux/internal/config"
	"vibemux/internal/model"
	"vibemux/internal/pty"
	"vibemux/internal/ui/addproject"
	"vibemux/internal/ui/projectlist"
	"vibemux/internal/ui/terminal"
)

func (m AppModel) Init() tea.Cmd {
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle tagged PTY messages at top level so background sessions receive them.
	switch msg := msg.(type) {
	case terminal.SessionOutputMsg:
		id := msg.ProjectID
		if m.state == ViewTerminal && m.activeSessionID == id {
			var cmd tea.Cmd
			m.terminal, cmd = m.terminal.Update(msg)
			return m, cmd
		}
		if sess, ok := m.sessions[id]; ok {
			var cmd tea.Cmd
			sess, cmd = sess.Update(msg)
			m.sessions[id] = sess
			return m, cmd
		}
		return m, nil

	case terminal.SessionExitedMsg:
		id := msg.ProjectID
		if m.state == ViewTerminal && m.activeSessionID == id {
			delete(m.sessions, id)
			m.state = ViewProjectList
			projects, _ := config.LoadProjects()
			m.projects = projects
			m.projectList = projectlist.New(projects, m.width, m.height)
			return m, nil
		}
		delete(m.sessions, id)
		return m, nil

	case terminal.DetachMsg:
		m.sessions[m.activeSessionID] = m.terminal
		m.state = ViewProjectList
		projects, _ := config.LoadProjects()
		m.projects = projects
		m.projectList = projectlist.New(projects, m.width, m.height)
		return m, nil

	case terminal.KillMsg:
		delete(m.sessions, m.activeSessionID)
		m.state = ViewProjectList
		projects, _ := config.LoadProjects()
		m.projects = projects
		m.projectList = projectlist.New(projects, m.width, m.height)
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.projectList.SetSize(msg.Width, msg.Height)
		if m.state == ViewTerminal {
			m.terminal.SetSize(msg.Width, msg.Height)
		}
		return m, nil
	}

	switch m.state {
	case ViewProjectList:
		return m.updateProjectList(msg)
	case ViewAddProject:
		return m.updateAddProject(msg)
	case ViewTerminal:
		return m.updateTerminal(msg)
	}

	return m, nil
}

func (m AppModel) updateProjectList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if p, ok := m.projectList.SelectedProject(); ok {
				return m.openProject(p)
			}
		case "a":
			m.state = ViewAddProject
			m.addProject = addproject.New()
			return m, m.addProject.Init()
		case "d":
			if p, ok := m.projectList.SelectedProject(); ok {
				config.RemoveProject(p.ID)
				projects, _ := config.LoadProjects()
				m.projects = projects
				cmd := m.projectList.SetProjects(projects)
				return m, cmd
			}
		case "q", "ctrl+c":
			for _, sess := range m.sessions {
				sess.Close()
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.projectList, cmd = m.projectList.Update(msg)
	return m, cmd
}

func (m AppModel) updateAddProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.state = ViewProjectList
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.addProject, cmd = m.addProject.Update(msg)

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

func (m AppModel) updateTerminal(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.terminal, cmd = m.terminal.Update(msg)
	return m, cmd
}

func (m AppModel) openProject(p model.Project) (tea.Model, tea.Cmd) {
	config.TouchProject(p.ID)

	// Reattach to existing background session if available.
	if sess, ok := m.sessions[p.ID]; ok {
		delete(m.sessions, p.ID)
		m.activeSessionID = p.ID
		m.terminal = sess
		m.terminal.SetSize(m.width, m.height)
		proj := p
		m.activeProject = &proj
		m.state = ViewTerminal
		return m, nil // read loop already running from background
	}

	// Start a new PTY session.
	termH := m.height - 1
	if termH < 1 {
		termH = 1
	}

	ptyInst, err := pty.Start(p.Path, m.width, termH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting PTY: %v\n", err)
		return m, nil
	}

	proj := p
	m.activeProject = &proj
	m.activeSessionID = p.ID
	m.terminal = terminal.New(ptyInst, p.ID, p.Name, m.width, m.height)
	m.state = ViewTerminal
	return m, m.terminal.Init()
}
