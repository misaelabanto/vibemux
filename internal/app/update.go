package app

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/tmux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(computeStatus(m.projects), tick(m.settings))
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TmuxReturnedMsg:
		// User detached or the session ended. Return to project list.
		projects, _ := config.LoadProjects()
		m.projects = projects
		prevActiveOnly := m.projectList.ShowActiveOnly()
		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetSettings(m.settings)
		m.projectList.SetShowActiveOnly(prevActiveOnly)
		m.projectList.SetActiveSessions(m.projectList.ActiveSessions())
		m.state = ViewProjectList
		return m, computeStatus(m.projects)

	case StatusComputedMsg:
		m.projectList.SetActiveSessions(msg.Active)
		m.projectList.SetAgents(msg.Agents)
		m.projectList.SetGitStatus(msg.Git)
		return m, nil

	case TickMsg:
		return m, tea.Batch(computeStatus(m.projects), tick(m.settings))

	case FetchDoneMsg:
		return m, computeStatus(m.projects)

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
					return m, tea.Batch(cmd, computeStatus(m.projects))
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					tmux.KillSession(tmux.SessionName(p.Path))
					return m, computeStatus(m.projects)
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

	execCmd := tea.ExecProcess(tmux.AttachCommand(name), func(err error) tea.Msg {
		return TmuxReturnedMsg{Err: err}
	})

	if m.settings.FetchOnEnter {
		fetchCmd := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), gitstatus.FetchTimeout())
			defer cancel()
			_ = gitstatus.Fetch(ctx, p.Path)
			return FetchDoneMsg{ProjectID: p.ID}
		}
		return m, tea.Batch(execCmd, fetchCmd)
	}

	return m, execCmd
}

// tick returns a tea.Cmd that fires TickMsg after LocalRefreshMS milliseconds.
// Defaults to 3000 ms if LocalRefreshMS is not positive.
func tick(s config.Settings) tea.Cmd {
	ms := s.LocalRefreshMS
	if ms <= 0 {
		ms = 3000
	}
	return tea.Tick(time.Duration(ms)*time.Millisecond, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

// computeStatus is a tea.Cmd that, off the UI goroutine, computes the full
// local status: active tmux sessions, agent statuses grouped per project (only
// for active projects), and git status per project.
func computeStatus(projects []model.Project) tea.Cmd {
	return func() tea.Msg {
		// Collect active tmux sessions.
		sessions, _ := tmux.ListVibemuxSessions()
		active := mapSessionsToProjects(sessions, projects)

		// Load all agent statuses and group them by project.
		statuses, _ := agent.LoadAll()
		allAgents := agent.GroupByProject(statuses, projects)

		// Gate agents on active: an agent cannot be live if its session is gone.
		agentsByActive := make(map[string][]agent.Status, len(allAgents))
		for id, ss := range allAgents {
			if active[id] {
				agentsByActive[id] = ss
			}
		}

		// Compute git status for each project.
		gitByProj := make(map[string]gitstatus.Status, len(projects))
		for _, p := range projects {
			gitByProj[p.ID] = gitstatus.Compute(p.Path)
		}

		return StatusComputedMsg{
			Active: active,
			Agents: agentsByActive,
			Git:    gitByProj,
		}
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
