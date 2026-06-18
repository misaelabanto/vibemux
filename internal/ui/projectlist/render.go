package projectlist

import (
	"fmt"
	"strings"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

// Git status glyphs. These are Nerd Font octicons (the U+F400 block, stable
// across Nerd Fonts v2/v3) and are NOT taken from Settings. They require a
// patched Nerd Font in the terminal to render.
const (
	glyphStaged    = "" // nf-oct-diff_added
	glyphModified  = "" // nf-oct-diff_modified
	glyphUntracked = "" // nf-oct-question
	glyphStashed   = "" // nf-oct-archive
	glyphConflict  = "" // nf-oct-alert
	glyphAhead     = "" // nf-oct-arrow_up
	glyphBehind    = "" // nf-oct-arrow_down
	glyphDiverged  = "" // nf-oct-git_compare
	glyphClean     = "" // nf-oct-check
)

// AgentIcon returns the icon for the given agent state from the settings Icons map.
// For unknown or empty states it returns an empty string.
func AgentIcon(state agent.State, s config.Settings) string {
	switch state {
	case agent.Working, agent.Done, agent.Blocked, agent.Stale:
		return s.Icons[string(state)]
	default:
		return ""
	}
}

// upstreamToken returns the upstream divergence glyph for a status, or "" if
// HasUpstream is false or the branch is level with its upstream (nothing to
// show when in sync). Returns glyphDiverged if both Ahead>0 and Behind>0,
// glyphAhead if only ahead, or glyphBehind if only behind.
func upstreamToken(g gitstatus.Status) string {
	if !g.HasUpstream {
		return ""
	}
	switch {
	case g.Ahead > 0 && g.Behind > 0:
		return glyphDiverged
	case g.Ahead > 0:
		return glyphAhead
	case g.Behind > 0:
		return glyphBehind
	default:
		return ""
	}
}

// GitBadge composes a space-joined badge string from the given git status.
// If the repo is not a git repo, returns s.Icons["no_git"].
// Git glyphs are literal Unicode constants defined in this file; only no_git comes from Settings.
func GitBadge(g gitstatus.Status, s config.Settings) string {
	if !g.IsRepo {
		return s.Icons["no_git"]
	}

	// Fully clean: show checkmark and upstream state if any.
	if g.Clean {
		tokens := []string{glyphClean}
		if u := upstreamToken(g); u != "" {
			tokens = append(tokens, u)
		}
		return strings.Join(tokens, " ")
	}

	// Dirty: compose tokens in order.
	var tokens []string

	if g.Staged {
		tokens = append(tokens, glyphStaged)
	}
	if g.Modified {
		tokens = append(tokens, glyphModified)
	}
	if g.Untracked {
		tokens = append(tokens, glyphUntracked)
	}
	if g.Stashed {
		tokens = append(tokens, glyphStashed)
	}
	if g.Conflict {
		tokens = append(tokens, glyphConflict)
	}

	if u := upstreamToken(g); u != "" {
		tokens = append(tokens, u)
	}

	return strings.Join(tokens, " ")
}

// StatusLine builds the status line string for a project row.
// left = GitBadge; right = agent icon (or active dot if session but no agent) + optional "+N".
// The two parts are joined with two spaces when both are present.
func StatusLine(focused agent.Status, derived agent.State, otherCount int, g gitstatus.Status, active bool, s config.Settings) string {
	gitPart := GitBadge(g, s)

	var agentPart string
	if icon := AgentIcon(derived, s); icon != "" {
		agentPart = icon
	} else if active {
		agentPart = s.Icons["active"]
	}

	if otherCount > 0 && agentPart != "" {
		agentPart += fmt.Sprintf("+%d", otherCount)
	}

	switch {
	case gitPart != "" && agentPart != "":
		return gitPart + "  " + agentPart
	case gitPart != "":
		return gitPart
	default:
		return agentPart
	}
}
