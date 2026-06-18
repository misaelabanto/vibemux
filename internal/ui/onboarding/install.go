package onboarding

import "github.com/misaelabanto/vibemux/internal/mux"

// installHint returns the recommended shell command to install the given
// multiplexer on the named OS (pass runtime.GOOS). It is copy-paste guidance
// only: vibemux never runs it.
func installHint(k mux.Kind, goos string) string {
	switch k {
	case mux.Tmux:
		switch goos {
		case "darwin":
			return "brew install tmux"
		case "linux":
			return "sudo apt install tmux   (or: sudo dnf install tmux / sudo pacman -S tmux)"
		default:
			return "see https://github.com/tmux/tmux/wiki/Installing"
		}
	case mux.Zellij:
		switch goos {
		case "darwin":
			return "brew install zellij"
		case "linux":
			return "cargo install --locked zellij   (or grab a binary from https://github.com/zellij-org/zellij/releases)"
		default:
			return "see https://zellij.dev/documentation/installation"
		}
	}
	return ""
}
