package onboarding

import (
	"fmt"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/misaelabanto/vibemux/internal/mux"
	"github.com/misaelabanto/vibemux/internal/ui/styles"
)

var (
	titleStyle  = styles.Accent
	cursorStyle = styles.Accent
	dimStyle    = styles.Muted
	helpStyle   = styles.Muted
)

type state int

const (
	stateInstall state = iota // zero multiplexers installed
	stateConfirm              // exactly one installed, confirm it
	stateSelect               // two or more installed, pick one
)

// reDetectedMsg carries a fresh installed-multiplexer scan triggered by the
// user pressing "r" on the install screen.
type reDetectedMsg struct {
	installed []mux.Kind
}

// Model resolves which multiplexer vibemux will use when none is validly
// saved, by inspecting which are installed.
type Model struct {
	state     state
	installed []mux.Kind
	cursor    int

	chosen    mux.Kind
	hasChosen bool
	quit      bool
}

// New builds the onboarding model from the set of currently-installed
// multiplexers and picks the starting screen from how many there are.
func New(installed []mux.Kind) Model {
	m := Model{installed: installed}
	m.syncState()
	return m
}

// syncState selects the screen that matches the current installed set.
func (m *Model) syncState() {
	switch len(m.installed) {
	case 0:
		m.state = stateInstall
	case 1:
		m.state = stateConfirm
	default:
		m.state = stateSelect
	}
	m.cursor = 0
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reDetectedMsg:
		m.installed = msg.installed
		m.syncState()
		return m, nil
	case tea.KeyPressMsg:
		switch m.state {
		case stateInstall:
			return m.updateInstall(msg)
		case stateConfirm:
			return m.updateConfirm(msg)
		case stateSelect:
			return m.updateSelect(msg)
		}
	}
	return m, nil
}

func (m Model) updateInstall(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "r":
		return m, reDetect
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

func (m Model) updateConfirm(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		m.chosen = m.installed[0]
		m.hasChosen = true
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

func (m Model) updateSelect(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.installed)-1 {
			m.cursor++
		}
	case "enter":
		m.chosen = m.installed[m.cursor]
		m.hasChosen = true
	case "q", "esc":
		m.quit = true
	}
	return m, nil
}

// reDetect re-scans for installed multiplexers (the user may have installed
// one in another shell) and reports the result back into Update.
func reDetect() tea.Msg {
	return reDetectedMsg{installed: mux.Installed()}
}

// Chosen reports the multiplexer the user settled on, once they have.
func (m Model) Chosen() (mux.Kind, bool) {
	return m.chosen, m.hasChosen
}

// Quit reports that the user abandoned onboarding without choosing.
func (m Model) Quit() bool { return m.quit }

func (m Model) View() string {
	switch m.state {
	case stateInstall:
		return m.viewInstall()
	case stateConfirm:
		return m.viewConfirm()
	case stateSelect:
		return m.viewSelect()
	}
	return ""
}

func (m Model) viewInstall() string {
	out := "\n" + titleStyle.Render("  No terminal multiplexer found") + "\n\n"
	out += "  vibemux needs tmux or zellij. Install one in another shell, then press r.\n\n"
	for _, k := range mux.All() {
		out += "  " + string(k) + ":\n"
		out += dimStyle.Render("    "+installHint(k, runtime.GOOS)) + "\n\n"
	}
	out += helpStyle.Render("  r re-check  q quit")
	return out + "\n"
}

func (m Model) viewConfirm() string {
	only := m.installed[0]
	out := "\n" + titleStyle.Render(fmt.Sprintf("  %s will be used", only)) + "\n\n"
	out += dimStyle.Render(fmt.Sprintf("  %s is the only multiplexer installed.", only)) + "\n\n"
	out += helpStyle.Render("  enter continue  q quit")
	return out + "\n"
}

func (m Model) viewSelect() string {
	out := "\n" + titleStyle.Render("  Choose a multiplexer") + "\n\n"
	for i, k := range m.installed {
		if i == m.cursor {
			out += cursorStyle.Render("  > "+string(k)) + "\n"
		} else {
			out += "    " + string(k) + "\n"
		}
	}
	out += "\n" + helpStyle.Render("  ↑↓ move  enter select  q quit")
	return out + "\n"
}
