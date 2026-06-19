package projectlist

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
	"github.com/misaelabanto/vibemux/internal/model"
	"github.com/misaelabanto/vibemux/internal/ui/styles"
)

var helpStyle = styles.Muted

var bannerStyle = styles.Accent

const banner = "" +
	" __  __      __                                         \n" +
	"/\\ \\/\\ \\  __/\\ \\                                        \n" +
	"\\ \\ \\ \\ \\/\\_\\ \\ \\____    __    ___ ___   __  __  __  _  \n" +
	" \\ \\ \\ \\ \\/\\ \\ \\ '__`\\ /'__`\\/' __` __`\\/\\ \\/\\ \\/\\ \\/'\\  \n" +
	"  \\ \\ \\_/ \\ \\ \\ \\ \\L\\ /\\  __//\\ \\/\\ \\/\\ \\ \\ \\_\\ \\/>  </  \n" +
	"   \\ `\\___/\\ \\_\\ \\_,__\\ \\____\\ \\_\\ \\_\\ \\_\\ \\____//\\_/\\_\\\n" +
	"    `\\/__/  \\/_/\\/___/ \\/____/\\/_/\\/_/\\/_/\\/___/ \\//\\/_/"

// focusedInfo holds precomputed data for the currently focused agent of a project.
type focusedInfo struct {
	status     agent.Status
	derived    agent.State
	otherCount int
}

// projectItem wraps a model.Project to add agent status, git status, and
// per-row display state for the list delegate.
type projectItem struct {
	model.Project
	active   bool
	index    int
	focused  focusedInfo
	git      gitstatus.Status
	settings config.Settings
	// message is the focused agent's message; path is the project path.
	// The delegate selects between them based on whether the row is selected.
	message string
}

func (p projectItem) Title() string {
	return fmt.Sprintf("%d. %s", p.index+1, p.Project.Name)
}

func (p projectItem) Description() string { return p.message }

// FilterValue feeds the list's fuzzy filter. It includes both the project name
// and its full path so a query can match a custom name, the leaf directory, or
// any parent directory in the path.
func (p projectItem) FilterValue() string { return p.Project.Name + " " + p.Project.Path }

// titleWithStatus builds the full title line with the status cluster placed
// inline, right after the project name. listWidth is the inner width of the
// list widget; it is accepted for signature stability but no longer used for
// right-alignment padding.
func (p projectItem) titleWithStatus(listWidth int, s *list.DefaultDelegate) string {
	left := p.Title()
	right := StatusLine(p.focused.status, p.focused.derived, p.focused.otherCount, p.git, p.active, p.settings)
	if right == "" {
		return left
	}
	return left + "  " + right
}

type Model struct {
	list           list.Model
	projects       []model.Project // unfiltered slice, source of truth for buildItems
	activeSessions map[string]bool // project ID -> has active multiplexer session
	agentsByProj   map[string][]agent.Status
	gitByProj      map[string]gitstatus.Status
	focusedAgent   map[string]int // project ID -> focused agent index
	settings       config.Settings
	showActiveOnly bool
	width          int
	height         int
	numberBuffer   string
}

func New(projects []model.Project, width, height int) Model {
	m := Model{
		width:          width,
		height:         height,
		activeSessions: map[string]bool{},
		agentsByProj:   map[string][]agent.Status{},
		gitByProj:      map[string]gitstatus.Status{},
		focusedAgent:   map[string]int{},
		settings:       config.DefaultSettings(),
		projects:       projects,
	}
	items := m.buildItems()
	delegate := highlightWhileFilteringDelegate{list.NewDefaultDelegate()}
	bannerHeight := lipgloss.Height(bannerStyle.Render(banner))
	l := list.New(items, delegate, width, height-bannerHeight-2)
	l.Title = "Projects"
	l.SetShowHelp(true)
	// Strip single-letter nav bindings so any typed character flows into filter.
	l.KeyMap.CursorUp.SetKeys("up")
	l.KeyMap.CursorDown.SetKeys("down")
	l.KeyMap.PrevPage.SetKeys("pgup")
	l.KeyMap.NextPage.SetKeys("pgdown")
	l.KeyMap.GoToStart.SetKeys("home")
	l.KeyMap.GoToEnd.SetKeys("end")
	l.KeyMap.ShowFullHelp.SetKeys("ctrl+h")
	l.KeyMap.CloseFullHelp.SetKeys("ctrl+h")
	m.list = l
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

// maxNumberBufferLen caps digit quick-select input; 999 items is well beyond UX.
const maxNumberBufferLen = 3

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.list.FilterState() != list.Filtering {
			s := key.String()
			count := len(m.list.Items())
			switch {
			case s == "left":
				m.FocusPrevAgent()
				return m, nil
			case s == "right":
				m.FocusNextAgent()
				return m, nil
			case len(s) == 1 && unicode.IsDigit(rune(s[0])):
				buf := m.numberBuffer + s
				if len(buf) > maxNumberBufferLen {
					buf = s
				}
				if n, _ := strconv.Atoi(buf); n >= 1 && n <= count {
					m.numberBuffer = buf
					m.list.Select(n - 1)
				} else if n, _ := strconv.Atoi(s); n >= 1 && n <= count {
					m.numberBuffer = s
					m.list.Select(n - 1)
				}
				return m, nil
			case s == "backspace" && m.numberBuffer != "":
				m.numberBuffer = m.numberBuffer[:len(m.numberBuffer)-1]
				if n, err := strconv.Atoi(m.numberBuffer); err == nil && n >= 1 && n <= count {
					m.list.Select(n - 1)
				}
				return m, nil
			case s == "/":
				m.numberBuffer = ""
			case isTypingChar(key):
				m.numberBuffer = ""
				slashKey := tea.KeyPressMsg{Code: '/', Text: "/"}
				var cmd1 tea.Cmd
				m.list, cmd1 = m.list.Update(slashKey)
				var cmd2 tea.Cmd
				m.list, cmd2 = m.list.Update(msg)
				return m, tea.Batch(cmd1, cmd2)
			default:
				m.numberBuffer = ""
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) IsFiltering() bool {
	return m.list.FilterState() == list.Filtering
}

func isTypingChar(key tea.KeyPressMsg) bool {
	if key.Mod != 0 {
		return false
	}
	if key.Text == "" {
		return false
	}
	for _, r := range key.Text {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func (m Model) View() string {
	b := bannerStyle.Render(banner)
	toggle := "ctrl+a active"
	if m.showActiveOnly {
		toggle = "ctrl+a all"
	}
	help := fmt.Sprintf("enter open  type filter  %s  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit", toggle)
	if m.numberBuffer != "" {
		help = fmt.Sprintf("-> %s    ", m.numberBuffer) + help
	}
	return lipgloss.JoinVertical(lipgloss.Left, b, m.list.View(), helpStyle.Render(help))
}

func (m Model) SelectedProject() (model.Project, bool) {
	if pi, ok := m.list.SelectedItem().(projectItem); ok {
		return pi.Project, true
	}
	return model.Project{}, false
}

// FocusedAgentForSelected returns the focused agent for the currently selected
// project. Returns false if no project is selected or it has no agents.
func (m Model) FocusedAgentForSelected() (agent.Status, bool) {
	pi, ok := m.list.SelectedItem().(projectItem)
	if !ok {
		return agent.Status{}, false
	}
	agents := m.agentsByProj[pi.ID]
	if len(agents) == 0 {
		return agent.Status{}, false
	}
	idx := m.focusedAgent[pi.ID]
	if idx >= len(agents) {
		idx = 0
	}
	return agents[idx], true
}

// GitStatusForSelected returns the git status for the currently selected project.
func (m Model) GitStatusForSelected() (gitstatus.Status, bool) {
	pi, ok := m.list.SelectedItem().(projectItem)
	if !ok {
		return gitstatus.Status{}, false
	}
	g, exists := m.gitByProj[pi.ID]
	return g, exists
}

// Settings returns the current settings stored on the model.
func (m Model) Settings() config.Settings {
	return m.settings
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	bannerHeight := lipgloss.Height(bannerStyle.Render(banner))
	m.list.SetSize(w, h-bannerHeight-2)
}

func (m *Model) SetProjects(projects []model.Project) tea.Cmd {
	m.projects = projects
	return m.list.SetItems(m.buildItems())
}

// SetActiveSessions updates which projects have running multiplexer sessions
// and rebuilds items so the active-only filter (if on) reflects the new set.
// The returned cmd MUST be run: when a filter is active, SetItems clears the
// filtered view and returns a cmd that recomputes it; dropping the cmd leaves
// the list stuck on "no results matched".
func (m *Model) SetActiveSessions(active map[string]bool) tea.Cmd {
	m.activeSessions = active
	return m.list.SetItems(m.buildItems())
}

// ActiveSessions returns the current active sessions map.
func (m Model) ActiveSessions() map[string]bool {
	return m.activeSessions
}

// SetAgents stores the per-project agent slices. The caller should pass
// urgency-sorted slices; SetAgents re-sorts them here as a safety measure.
// Per-project focused index is reset to 0 (most urgent) on each call.
// The returned cmd MUST be run so an active filter is recomputed (see
// SetActiveSessions).
func (m *Model) SetAgents(byProj map[string][]agent.Status) tea.Cmd {
	m.agentsByProj = make(map[string][]agent.Status, len(byProj))
	m.focusedAgent = make(map[string]int, len(byProj))
	threshold := time.Duration(m.settings.StaleThresholdSec) * time.Second
	now := time.Now()
	for id, ss := range byProj {
		m.agentsByProj[id] = agent.SortByUrgency(ss, threshold, now)
		// Reset focus to 0 so the most urgent is always default.
		m.focusedAgent[id] = 0
	}
	return m.list.SetItems(m.buildItems())
}

// SetGitStatus stores git status keyed by project ID and rebuilds items. The
// returned cmd MUST be run so an active filter is recomputed (see
// SetActiveSessions).
func (m *Model) SetGitStatus(byProj map[string]gitstatus.Status) tea.Cmd {
	m.gitByProj = byProj
	return m.list.SetItems(m.buildItems())
}

// SetSettings stores the settings and rebuilds items so icons/thresholds are fresh.
func (m *Model) SetSettings(s config.Settings) {
	m.settings = s
	m.list.SetItems(m.buildItems())
}

// FocusNextAgent advances the focused agent index for the selected project,
// wrapping around. No-op if the project has fewer than 2 agents.
func (m *Model) FocusNextAgent() {
	pi, ok := m.list.SelectedItem().(projectItem)
	if !ok {
		return
	}
	agents := m.agentsByProj[pi.ID]
	if len(agents) < 2 {
		return
	}
	cur := m.focusedAgent[pi.ID]
	m.focusedAgent[pi.ID] = (cur + 1) % len(agents)
	m.list.SetItems(m.buildItems())
}

// FocusPrevAgent moves the focused agent index back for the selected project,
// wrapping around. No-op if the project has fewer than 2 agents.
func (m *Model) FocusPrevAgent() {
	pi, ok := m.list.SelectedItem().(projectItem)
	if !ok {
		return
	}
	agents := m.agentsByProj[pi.ID]
	if len(agents) < 2 {
		return
	}
	cur := m.focusedAgent[pi.ID]
	m.focusedAgent[pi.ID] = (cur - 1 + len(agents)) % len(agents)
	m.list.SetItems(m.buildItems())
}

// ToggleActiveOnly flips the active-only filter and rebuilds items.
func (m *Model) ToggleActiveOnly() tea.Cmd {
	m.showActiveOnly = !m.showActiveOnly
	m.numberBuffer = ""
	m.syncTitle()
	return m.list.SetItems(m.buildItems())
}

// SetShowActiveOnly preserves the toggle across model rebuilds (e.g. after
// returning from a multiplexer session).
func (m *Model) SetShowActiveOnly(v bool) {
	if m.showActiveOnly == v {
		return
	}
	m.showActiveOnly = v
	m.syncTitle()
	m.list.SetItems(m.buildItems())
}

// ShowActiveOnly reports whether the active-only filter is on.
func (m Model) ShowActiveOnly() bool {
	return m.showActiveOnly
}

func (m *Model) syncTitle() {
	if m.showActiveOnly {
		m.list.Title = "Projects (active only)"
	} else {
		m.list.Title = "Projects"
	}
}

// highlightWhileFilteringDelegate is a DefaultDelegate that also applies the
// selected style while the user is still typing a filter, so the top match is
// visibly highlighted. It also knows to show the project path (instead of agent
// message) on the selected row.
type highlightWhileFilteringDelegate struct {
	list.DefaultDelegate
}

const ellipsis = "…"

func (d highlightWhileFilteringDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(projectItem)
	if !ok {
		return
	}

	width := m.Width()
	if width <= 0 {
		return
	}

	s := &d.Styles
	textwidth := width - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()

	isSelected := index == m.Index()

	// Title includes the right-aligned status cluster.
	title := pi.titleWithStatus(width, &d.DefaultDelegate)
	title = ansi.Truncate(title, textwidth, ellipsis)

	// Always show the agent's last message; fall back to the path when the row
	// has no message (idle, or working with a cleared message) so the line stays useful.
	desc := pi.message
	if desc == "" {
		desc = pi.Project.Path
	}

	if d.ShowDescription {
		var lines []string
		for i, line := range strings.Split(desc, "\n") {
			if i >= d.Height()-1 {
				break
			}
			lines = append(lines, ansi.Truncate(line, textwidth, ellipsis))
		}
		desc = strings.Join(lines, "\n")
	}

	filterState := m.FilterState()
	emptyFilter := filterState == list.Filtering && m.FilterValue() == ""
	isFiltered := filterState == list.Filtering || filterState == list.FilterApplied

	var matchedRunes []int
	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	// Active projects render in bold so they stand out from the full list.
	normalTitle := s.NormalTitle
	selectedTitle := s.SelectedTitle
	if pi.active {
		normalTitle = normalTitle.Bold(true)
		selectedTitle = selectedTitle.Bold(true)
	}

	switch {
	case emptyFilter:
		title = s.DimmedTitle.Render(title)
		desc = s.DimmedDesc.Render(desc)
	case isSelected:
		if isFiltered {
			unmatched := selectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = selectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	default:
		if isFiltered {
			unmatched := normalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = normalTitle.Render(title)
		desc = s.NormalDesc.Render(desc)
	}

	if d.ShowDescription {
		fmt.Fprintf(w, "%s\n%s", title, desc)
		return
	}
	fmt.Fprintf(w, "%s", title)
}

func (m Model) buildItems() []list.Item {
	sorted := make([]model.Project, len(m.projects))
	copy(sorted, m.projects)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	filtered := sorted
	if m.showActiveOnly {
		filtered = make([]model.Project, 0, len(sorted))
		for _, p := range sorted {
			if m.activeSessions[p.ID] {
				filtered = append(filtered, p)
			}
		}
	}

	threshold := time.Duration(m.settings.StaleThresholdSec) * time.Second
	now := time.Now()

	items := make([]list.Item, len(filtered))
	for i, p := range filtered {
		agents := m.agentsByProj[p.ID]
		focIdx := m.focusedAgent[p.ID]
		if focIdx >= len(agents) {
			focIdx = 0
		}

		var fi focusedInfo
		var msg string
		if len(agents) > 0 {
			foc := agents[focIdx]
			fi = focusedInfo{
				status:     foc,
				derived:    agent.DerivedState(foc, threshold, now),
				otherCount: len(agents) - 1,
			}
			msg = foc.Message
		}

		items[i] = projectItem{
			Project:  p,
			active:   m.activeSessions[p.ID],
			index:    i,
			focused:  fi,
			git:      m.gitByProj[p.ID],
			settings: m.settings,
			message:  msg,
		}
	}
	return items
}
