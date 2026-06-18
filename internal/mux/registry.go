package mux

import (
	"fmt"

	"github.com/misaelabanto/vibemux/internal/tmux"
	"github.com/misaelabanto/vibemux/internal/zellij"
)

// New constructs the backend for a Kind. The returned value implements
// Multiplexer; an unknown kind is an error.
func New(k Kind) (Multiplexer, error) {
	switch k {
	case Tmux:
		return tmux.Backend{}, nil
	case Zellij:
		return zellij.Backend{}, nil
	}
	return nil, fmt.Errorf("unknown multiplexer %q", k)
}

// Installed returns the subset of All() whose backend binary is present,
// preserving All()'s order.
func Installed() []Kind {
	var out []Kind
	for _, k := range All() {
		m, err := New(k)
		if err == nil && m.IsInstalled() {
			out = append(out, k)
		}
	}
	return out
}
