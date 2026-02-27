package statusbar

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(1).
			PaddingRight(1)

	prefixStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FF6F61")).
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(1).
			PaddingRight(1).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#C9B8FF"))
)

func Render(width int, projectName string, prefixMode bool) string {
	left := " " + projectName

	var right string
	if prefixMode {
		right = "d/p: detach  x: close  ^a: literal  esc: cancel "
	} else {
		right = "^a d: detach "
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	inner := left + strings.Repeat(" ", gap) + hintStyle.Render(right)

	bar := barStyle.Width(width).Render(inner)

	if prefixMode {
		bar = prefixStyle.Render("PREFIX") + bar[lipgloss.Width(prefixStyle.Render("PREFIX")):]
	}

	return bar
}
