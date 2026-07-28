// Package toast renders a short-lived, floating notification over the rest of
// the UI. It exists so error and confirmation messages never reach os.Stderr
// while the alternate screen is active: a raw stderr write lands on top of the
// rendered frame, and bubbletea has no idea those cells changed, so the
// corruption persists until the next full repaint.
package toast

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/misaelabanto/vibemux/internal/ui/styles"
)

// Kind selects a toast's severity styling.
type Kind int

const (
	// KindError styles the toast for a failure the user should act on.
	KindError Kind = iota
	// KindInfo styles the toast for a confirmation of something that worked.
	KindInfo
)

// lifetime is how long a toast stays up before dismissing itself.
const lifetime = 4 * time.Second

// Box width bounds. The lower bound is load-bearing: a plain
// min(maxBoxWidth, maxWidth-borderAndMargin) goes non-positive on a very
// narrow terminal, and lipgloss treats Width(0) as "size to content", which
// turns the cap into no cap at all.
const (
	minBoxWidth      = 20
	maxBoxWidth      = 60
	borderAndPadding = 4
)

// ExpiredMsg asks the model to clear the toast identified by Seq. A toast is
// cleared only when Seq matches the current one, so an older timer cannot wipe
// a toast raised after it was scheduled.
type ExpiredMsg struct {
	Seq int
}

// Model holds the single toast currently on screen, if any. One toast is shown
// at a time and a new one replaces the current one.
type Model struct {
	message string
	kind    Kind
	visible bool
	seq     int
}

// New builds an empty, invisible toast model.
func New() Model { return Model{} }

// Show replaces the current toast and returns the command that dismisses it
// after lifetime elapses.
//
// Callers MUST batch the returned command into their own return value. A
// dropped command leaves the toast up until the next key press.
func (m *Model) Show(kind Kind, text string) tea.Cmd {
	m.message = text
	m.kind = kind
	m.visible = true
	m.seq++

	scheduledSeq := m.seq
	return tea.Tick(lifetime, func(time.Time) tea.Msg {
		return ExpiredMsg{Seq: scheduledSeq}
	})
}

// Update clears the toast on its own expiry or on any key press.
//
// A key press dismisses the toast but is never reported as handled: the toast
// has no focus, so every binding underneath it keeps working.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ExpiredMsg:
		if msg.Seq == m.seq {
			m.visible = false
		}
	case tea.KeyPressMsg:
		m.visible = false
	}
	return m, nil
}

// Visible reports whether a toast is currently on screen.
func (m Model) Visible() bool { return m.visible }

// Message returns the raw, unwrapped toast text. Rendered output is wrapped
// and carries ANSI codes, so tests assert against this instead.
func (m Model) Message() string { return m.message }

// Render draws the toast box, sized to fit within maxWidth. Returns the empty
// string when no toast is visible.
func (m Model) Render(maxWidth int) string {
	if !m.visible {
		return ""
	}

	boxWidth := maxWidth - borderAndPadding
	if boxWidth < minBoxWidth {
		boxWidth = minBoxWidth
	}
	if boxWidth > maxBoxWidth {
		boxWidth = maxBoxWidth
	}

	borderColor := styles.Error.GetForeground()
	if m.kind == KindInfo {
		borderColor = styles.Muted.GetForeground()
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(boxWidth)

	plain := box.Render(m.message)

	// lipgloss recolors every border segment and every text line on its own,
	// via BorderForeground/Foreground: fine at normal widths, but on a narrow
	// box the repeated escape sequences balloon the raw byte length far past
	// what's visible. A single open/reset pair wrapped around the whole
	// rendered block produces the identical color on screen, since SGR state
	// persists across newlines, without paying that per-line cost.
	open := ansi.Style{}.ForegroundColor(borderColor).String()
	return open + plain + ansi.ResetStyle
}
