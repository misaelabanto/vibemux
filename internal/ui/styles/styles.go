// Package styles holds vibemux's shared TUI palette so the accent, muted, and
// error styles are defined once and reused across the UI packages instead of
// being re-declared (and at risk of drifting) in each one.
package styles

import "charm.land/lipgloss/v2"

var (
	// Accent is the bold magenta used for titles, banners, and selected items.
	Accent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	// Muted is the dim gray used for help lines and secondary hint text.
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// Error is the red used for error messages.
	Error = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)
