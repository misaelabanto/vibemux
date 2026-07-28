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
	"github.com/misaelabanto/vibemux/internal/ui/toast"
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
	// Forwarded before anything else, for two reasons. The switch below
	// returns early for several message types, so forwarding placed after it
	// would never deliver ExpiredMsg. And the toast clears on any key press:
	// if the keystroke reached the toast after a handler called Show, every
	// confirmation toast would be dismissed by the key that raised it.
	m.toast = m.toast.Update(msg)

	switch msg := msg.(type) {
	case MultiplexerReturnedMsg:
		// User detached or the session ended: return to project list. Reapply the
		// scope filter so a scoped session does not reveal every project on reload.
		projects, _ := config.LoadProjects()
		projects = model.ProjectsUnder(projects, m.scopeDir)
		m.projects = projects

		// The rebuilt list starts out knowing no status at all, so the status the
		// previous sweep computed is carried across. Detaching does not change any
		// of it: the session that was just left is still running, and the git
		// worktrees are as they were. Without this the dashboard renders from an
		// empty status until the next sweep lands, which with the active-only
		// filter on means rendering nothing at all, since every project is judged
		// inactive and filtered away.
		prevActiveOnly := m.projectList.ShowActiveOnly()
		prevActive := m.projectList.ActiveSessions()
		prevAgents := m.projectList.Agents()
		prevGit := m.projectList.GitStatus()

		m.projectList = projectlist.New(projects, m.width, m.height)
		m.projectList.SetSettings(m.settings)
		m.projectList.SetActiveSessions(prevActive)
		m.projectList.SetAgents(prevAgents)
		m.projectList.SetGitStatus(prevGit)
		m.projectList.SetShowActiveOnly(prevActiveOnly)
		m.state = ViewProjectList

		// Worded neutrally on purpose. A clean detach exits zero, but a Session
		// killed out from under an attached client, or a client closed after
		// another vibemux instance deleted the Session, surfaces a non-zero exit
		// on what the user experienced as an ordinary exit.
		var toastCmd tea.Cmd
		if msg.Err != nil {
			toastCmd = m.toast.Show(toast.KindError, fmt.Sprintf("Session exited: %v", msg.Err))
		}
		return m, tea.Batch(computeStatus(m.projects, m.mux), toastCmd)

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
		active, err := mux.New(k)
		if err != nil {
			// ClearChoice matters: hasChosen is sticky, so without resetting it
			// every later message would replay this same failing path.
			m.onboarding.ClearChoice()
			return m, tea.Batch(cmd, m.toast.Show(toast.KindError, fmt.Sprintf("Could not initialize %s: %v", k, err)))
		}

		// Load, modify, save so the status-display settings are preserved.
		s, _ := config.LoadSettings()
		s.Multiplexer = string(k)
		_ = config.SaveSettings(s)

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
		m.state = ViewProjectList
		if err := hookinstall.Install("vibemux"); err != nil {
			return m, tea.Batch(
				computeStatus(m.projects, m.mux),
				m.toast.Show(toast.KindError, fmt.Sprintf("Could not install hooks: %v", err)),
			)
		}
		return m, tea.Batch(
			computeStatus(m.projects, m.mux),
			m.toast.Show(toast.KindInfo, "Agent status tracking enabled"),
		)
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
				if selected, ok := m.projectList.SelectedProject(); ok {
					// Gated on HasSession: kill-session against a session that is
					// not there exits non-zero on both backends, and reporting
					// that as a failure would be noise, not information.
					name := m.mux.SessionName(selected.Path)
					if m.mux.HasSession(name) {
						if err := m.mux.KillSession(name); err != nil {
							// Do not proceed to remove the Project: a failed kill
							// would otherwise leave an orphaned session with no
							// Project left to surface it in the dashboard.
							return m, tea.Batch(
								computeStatus(m.projects, m.mux),
								m.toast.Show(toast.KindError, fmt.Sprintf("Could not kill session: %v", err)),
							)
						}
					}
					if err := config.RemoveProject(selected.ID); err != nil {
						return m, tea.Batch(
							computeStatus(m.projects, m.mux),
							m.toast.Show(toast.KindError, fmt.Sprintf("Could not remove Project: %v", err)),
						)
					}
					projects, _ := config.LoadProjects()
					projects = model.ProjectsUnder(projects, m.scopeDir)
					m.projects = projects
					setCmd := m.projectList.SetProjects(projects)
					return m, tea.Batch(
						setCmd,
						computeStatus(m.projects, m.mux),
						m.toast.Show(toast.KindInfo, fmt.Sprintf("Removed %s", selected.Name)),
					)
				}
			case "ctrl+x":
				if selected, ok := m.projectList.SelectedProject(); ok {
					name := m.mux.SessionName(selected.Path)
					if !m.mux.HasSession(name) {
						return m, nil
					}
					if err := m.mux.KillSession(name); err != nil {
						return m, tea.Batch(
							computeStatus(m.projects, m.mux),
							m.toast.Show(toast.KindError, fmt.Sprintf("Could not kill session: %v", err)),
						)
					}
					return m, tea.Batch(
						computeStatus(m.projects, m.mux),
						m.toast.Show(toast.KindInfo, fmt.Sprintf("Killed session %s", name)),
					)
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
		m.state = ViewProjectList
		added, err := config.AddProject(path)
		if err != nil {
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not add Project: %v", err))
		}
		m.projects = append(m.projects, added)
		setCmd := m.projectList.SetProjects(m.projects)
		return m, tea.Batch(setCmd, m.toast.Show(toast.KindInfo, fmt.Sprintf("Added %s", added.Name)))
	}

	return m, cmd
}

func (m AppModel) openProject(p model.Project) (tea.Model, tea.Cmd) {
	if !m.mux.IsInstalled() {
		return m, m.toast.Show(toast.KindError, fmt.Sprintf("%s is not installed", m.mux.Name()))
	}

	name := m.mux.SessionName(p.Path)

	// The directory is only checked when a Session has to be created. An
	// existing Session is already running on the multiplexer server and
	// attaching to it does not touch the path, so a Project whose directory
	// was deleted can still be reached to recover whatever is inside it.
	if !m.mux.HasSession(name) {
		if _, err := os.Stat(p.Path); err != nil {
			if os.IsNotExist(err) {
				return m, m.toast.Show(toast.KindError, fmt.Sprintf("Project directory not found: %s", p.Path))
			}
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Cannot open %s: %v", p.Path, err))
		}
		if err := m.mux.NewSession(name, p.Path); err != nil {
			return m, m.toast.Show(toast.KindError, fmt.Sprintf("Could not create session: %v", err))
		}
	}

	config.TouchProject(p.ID)

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
//
// Sweeps are coalesced, so the burst of refresh triggers that arrives when a
// multiplexer session is left costs one sweep rather than one each.
func computeStatus(projects []model.Project, mx mux.Multiplexer) tea.Cmd {
	return func() tea.Msg {
		if mx == nil {
			return StatusComputedMsg{}
		}
		return statusSweeps.do(func() StatusComputedMsg {
			return sweepStatus(projects, mx)
		})
	}
}

// sweepStatus performs one full status computation.
func sweepStatus(projects []model.Project, mx mux.Multiplexer) StatusComputedMsg {
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

	// Compute git status for every project. The paths are swept together rather
	// than one after another: each costs a git spawn plus a worktree scan, so a
	// sequential sweep takes as long as the sum of every project while this
	// takes about as long as the slowest one.
	paths := make([]string, len(projects))
	for i, p := range projects {
		paths[i] = p.Path
	}
	byPath := gitstatus.ComputeAll(paths)
	gitByProj := make(map[string]gitstatus.Status, len(projects))
	for _, p := range projects {
		gitByProj[p.ID] = byPath[p.Path]
	}

	return StatusComputedMsg{
		Active: active,
		Agents: agentsByActive,
		Git:    gitByProj,
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
