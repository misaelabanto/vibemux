package projectlist

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/misaelabanto/vibemux/internal/model"
)

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

var bannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

const banner = "" +
	" __  __      __                                         \n" +
	"/\\ \\/\\ \\  __/\\ \\                                        \n" +
	"\\ \\ \\ \\ \\/\\_\\ \\ \\____    __    ___ ___   __  __  __  _  \n" +
	" \\ \\ \\ \\ \\/\\ \\ \\ '__`\\ /'__`\\/' __` __`\\/\\ \\/\\ \\/\\ \\/'\\  \n" +
	"  \\ \\ \\_/ \\ \\ \\ \\ \\L\\ /\\  __//\\ \\/\\ \\/\\ \\ \\ \\_\\ \\/>  </  \n" +
	"   \\ `\\___/\\ \\_\\ \\_,__\\ \\____\\ \\_\\ \\_\\ \\_\\ \\____//\\_/\\_\\\n" +
	"    `\\/__/  \\/_/\\/___/ \\/____/\\/_/\\/_/\\/_/\\/___/ \\//\\/_/"

// projectItem wraps a model.Project to add an active session indicator.
type projectItem struct {
	model.Project
	active bool
	index  int
}

func (p projectItem) Title() string {
	prefix := fmt.Sprintf("%d. ", p.index+1)
	if p.active {
		return prefix + "● " + p.Project.Name
	}
	return prefix + p.Project.Name
}

func (p projectItem) Description() string { return p.Project.Path }
func (p projectItem) FilterValue() string { return p.Project.Name }

type Model struct {
	list           list.Model
	activeSessions map[string]bool // project ID → has active tmux session
	width          int
	height         int
	numberBuffer   string
}

func New(projects []model.Project, width, height int) Model {
	m := Model{width: width, height: height, activeSessions: map[string]bool{}}
	items := m.projectsToItems(projects)
	delegate := highlightWhileFilteringDelegate{list.NewDefaultDelegate()}
	bannerHeight := lipgloss.Height(bannerStyle.Render(banner))
	l := list.New(items, delegate, width, height-bannerHeight-2)
	l.Title = "Projects"
	l.SetShowHelp(true)
	// Strip single-letter nav bindings so any typed character flows into filter.
	l.KeyMap.CursorUp.SetKeys("up")
	l.KeyMap.CursorDown.SetKeys("down")
	l.KeyMap.PrevPage.SetKeys("left", "pgup")
	l.KeyMap.NextPage.SetKeys("right", "pgdown")
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
	help := "enter open  type filter  ctrl+n add  ctrl+d delete  ctrl+x kill  ctrl+c quit"
	if m.numberBuffer != "" {
		help = fmt.Sprintf("→ %s    ", m.numberBuffer) + help
	}
	return lipgloss.JoinVertical(lipgloss.Left, b, m.list.View(), helpStyle.Render(help))
}

func (m Model) SelectedProject() (model.Project, bool) {
	if pi, ok := m.list.SelectedItem().(projectItem); ok {
		return pi.Project, true
	}
	return model.Project{}, false
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	bannerHeight := lipgloss.Height(bannerStyle.Render(banner))
	m.list.SetSize(w, h-bannerHeight-2)
}

func (m *Model) SetProjects(projects []model.Project) tea.Cmd {
	return m.list.SetItems(m.projectsToItems(projects))
}

// SetActiveSessions updates which projects have running tmux sessions and
// refreshes the indicators on each item.
func (m *Model) SetActiveSessions(active map[string]bool) {
	m.activeSessions = active
	items := m.list.Items()
	refreshed := make([]list.Item, len(items))
	for i, item := range items {
		if pi, ok := item.(projectItem); ok {
			pi.active = active[pi.Project.ID]
			refreshed[i] = pi
		} else {
			refreshed[i] = item
		}
	}
	m.list.SetItems(refreshed)
}

// ActiveSessions returns the current active sessions map.
func (m Model) ActiveSessions() map[string]bool {
	return m.activeSessions
}

// highlightWhileFilteringDelegate is a DefaultDelegate that also applies the
// selected style while the user is still typing a filter, so the top match is
// visibly highlighted.
type highlightWhileFilteringDelegate struct {
	list.DefaultDelegate
}

const ellipsis = "…"

func (d highlightWhileFilteringDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	di, ok := item.(list.DefaultItem)
	if !ok {
		return
	}
	title := di.Title()
	desc := di.Description()

	width := m.Width()
	if width <= 0 {
		return
	}

	s := &d.Styles
	textwidth := width - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()
	title = ansi.Truncate(title, textwidth, ellipsis)
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

	isSelected := index == m.Index()
	filterState := m.FilterState()
	emptyFilter := filterState == list.Filtering && m.FilterValue() == ""
	isFiltered := filterState == list.Filtering || filterState == list.FilterApplied

	var matchedRunes []int
	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	switch {
	case emptyFilter:
		title = s.DimmedTitle.Render(title)
		desc = s.DimmedDesc.Render(desc)
	case isSelected:
		if isFiltered {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.SelectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	default:
		if isFiltered {
			unmatched := s.NormalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.NormalTitle.Render(title)
		desc = s.NormalDesc.Render(desc)
	}

	if d.ShowDescription {
		fmt.Fprintf(w, "%s\n%s", title, desc)
		return
	}
	fmt.Fprintf(w, "%s", title)
}

func (m Model) projectsToItems(projects []model.Project) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{
			Project: p,
			active:  m.activeSessions[p.ID],
			index:   i,
		}
	}
	return items
}
