package mux

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
		ok   bool
	}{
		{"tmux", Tmux, true},
		{"zellij", Zellij, true},
		{"fish", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestActive(t *testing.T) {
	both := []Kind{Tmux, Zellij}
	cases := []struct {
		name      string
		saved     string
		installed []Kind
		want      Kind
		ok        bool
	}{
		{"saved and installed", "zellij", both, Zellij, true},
		{"saved not installed", "zellij", []Kind{Tmux}, "", false},
		{"empty saved", "", []Kind{Tmux}, "", false},
		{"unknown saved", "fish", both, "", false},
	}
	for _, c := range cases {
		got, ok := Active(c.saved, c.installed)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: Active(%q, %v) = (%q, %v), want (%q, %v)",
				c.name, c.saved, c.installed, got, ok, c.want, c.ok)
		}
	}
}

// TestNewKnownKinds also enforces, at compile time, that both backends
// satisfy Multiplexer (New returns them typed as Multiplexer).
func TestNewKnownKinds(t *testing.T) {
	for _, k := range All() {
		m, err := New(k)
		if err != nil {
			t.Fatalf("New(%q) error: %v", k, err)
		}
		if m.Name() != string(k) {
			t.Errorf("New(%q).Name() = %q, want %q", k, m.Name(), string(k))
		}
	}
}

func TestNewUnknownKind(t *testing.T) {
	if _, err := New("fish"); err == nil {
		t.Error(`New("fish") = nil error, want error`)
	}
}
