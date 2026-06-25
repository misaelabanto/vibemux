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
	"github.com/misaelabanto/vibemux/internal/hookinstall"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/addproject"
	"github.com/misaelabanto/vibemux/internal/ui/projectlist"
)

func (m AppModel) Init() tea.Cmd {
	// Dispatch on the authoritative view state. Onboarding has no multiplexer
	// yet, so it cannot compute status; every other state starts the periodic
	// status tick loop.
	switch m.state {
	case ViewOnboarding:
		return m.onboarding.Init()
	default:
		return tea.Batch(computeStatus(m.projects, m.mux), tick(m.settings))
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MultiplexerReturnedMsg:
		// User detached or the session ended: return to project list. Reapply the
		// scope filter so a scoped session does not reveal every project on reload.
		projects, _ := config.LoadProjects()
		projects = model.ProjectsUnder(projects, m.scopeDir)
		m.projects = projects
		prevActiveOnly := m.projectList.ShowActiveOnly()
		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetSettings(m.settings)
		m.projectList.SetShowActiveOnly(prevActiveOnly)
		m.state = ViewProjectList
		return m, computeStatus(m.projects, m.mux)

	case StatusComputedMsg:
		// Each setter rebuilds the list items; when a filter is active, SetItems
		// returns a cmd that recomputes the filtered view. These cmds must be
		// run or the list gets stuck on "no results matched" after a refresh.
		return m, tea.Batch(
			m.projectList.SetActiveSessions(msg.Active),
			m.projectList.SetAgents(msg.Agents),
			m.projectList.SetGitStatus(msg.Git),
		)

	case TickMsg:
		return m, tea.Batch(computeStatus(m.projects, m.mux), tick(m.settings))

	case FetchDoneMsg:
		return m, computeStatus(m.projects, m.mux)

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
	case ViewConsent:
		return m.updateConsent(msg)
	}

	return m, nil
}

// updateOnboarding routes input to the onboarding sub-model and, once the
// user has chosen a multiplexer, persists it, builds the backend, and enters
// the project list (or the hook-consent prompt on first run). Quitting
// onboarding exits vibemux (it cannot run without a multiplexer).
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
		// Load, modify, save so the status-display settings are preserved.
		s, _ := config.LoadSettings()
		s.Multiplexer = string(k)
		_ = config.SaveSettings(s)

		active, err := mux.New(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing %s: %v\n", k, err)
			return m, tea.Quit
		}
		m.mux = active
		if needsConsent() {
			m.state = ViewConsent
		} else {
			m.state = ViewProjectList
		}
		m.projectList.SetSize(m.width, m.height)
		return m, tea.Batch(cmd, computeStatus(m.projects, m.mux), tick(m.settings))
	}

	return m, cmd
}

// updateConsent handles key events in the hook-consent prompt state.
func (m AppModel) updateConsent(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		_ = hookinstall.Install("vibemux")
		m.state = ViewProjectList
		return m, computeStatus(m.projects, m.mux)
	case "n", "N":
		_ = setHooksDeclined()
		m.state = ViewProjectList
		return m, nil
	default:
		m.state = ViewProjectList
		return m, nil
	}
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
				m.addProject = addproject.New(m.scopeDir)
				return m, m.addProject.Init()
			case "ctrl+d":
				if p, ok := m.projectList.SelectedProject(); ok {
					m.mux.KillSession(m.mux.SessionName(p.Path))
					config.RemoveProject(p.ID)
					projects, _ := config.LoadProjects()
					projects = model.ProjectsUnder(projects, m.scopeDir)
					m.projects = projects
					cmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(cmd, computeStatus(m.projects, m.mux))
				}
			case "ctrl+x":
				if p, ok := m.projectList.SelectedProject(); ok {
					m.mux.KillSession(m.mux.SessionName(p.Path))
					return m, computeStatus(m.projects, m.mux)
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

	execCmd := tea.ExecProcess(m.mux.AttachCommand(name), func(err error) tea.Msg {
		return MultiplexerReturnedMsg{Err: err}
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
// local status: active multiplexer sessions, agent statuses grouped per
// project (only for active projects), and git status per project.
func computeStatus(projects []model.Project, mx mux.Multiplexer) tea.Cmd {
	return func() tea.Msg {
		if mx == nil {
			return StatusComputedMsg{}
		}

		// Collect active multiplexer sessions.
		sessions, _ := mx.ListVibemuxSessions()
		active := mapSessionsToProjects(sessions, projects, mx)

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
