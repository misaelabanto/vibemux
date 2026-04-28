package addproject

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type nameInputModel struct {
	title    string
	hint     string
	parent   string
	input    textinput.Model
	err      string
	submit   bool
	canceled bool
}

func newNameInput(title, hint, placeholder, parent string) nameInputModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	return nameInputModel{
		title:  title,
		hint:   hint,
		parent: parent,
		input:  ti,
	}
}

func (m *nameInputModel) SetError(s string) {
	m.err = s
	m.submit = false
}

func (m nameInputModel) Value() string {
	return m.input.Value()
}

func (m nameInputModel) Update(msg tea.Msg) (nameInputModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			if m.input.Value() != "" {
				m.submit = true
				return m, nil
			}
		case "esc":
			m.canceled = true
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.err != "" {
		// Any keystroke after an error clears it.
		if _, ok := msg.(tea.KeyPressMsg); ok {
			m.err = ""
		}
	}
	return m, cmd
}

func (m nameInputModel) View() string {
	out := "\n  " + m.title + "\n"
	out += dimStyle.Render("  Parent: "+m.parent) + "\n\n"
	out += "  " + m.input.View() + "\n"
	if m.err != "" {
		out += "\n" + errorStyle.Render("  "+m.err) + "\n"
	}
	out += "\n" + helpStyle.Render("  "+m.hint)
	return out + "\n"
}
