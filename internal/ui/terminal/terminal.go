package terminal

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"vibemux/internal/pty"
	"vibemux/internal/ui/statusbar"

	vt10x "github.com/hinshun/vt10x"
)

type InputMode int

const (
	ModeNormal InputMode = iota
	ModePrefix
)

// DetachMsg is sent when the user detaches from the terminal (ctrl+a d or ctrl+a p).
type DetachMsg struct{}

// KillMsg is sent when the user explicitly kills the session (ctrl+a x).
type KillMsg struct{}

// SessionOutputMsg is PTY output tagged with a project ID so background sessions
// can be routed correctly.
type SessionOutputMsg struct {
	ProjectID string
	Data      []byte
}

// SessionExitedMsg indicates a PTY exited, tagged with project ID.
type SessionExitedMsg struct {
	ProjectID string
}

type Model struct {
	ptyInst     *pty.Pty
	vterm       vt10x.Terminal
	projectID   string
	projectName string
	mode        InputMode
	width       int
	height      int
}

func New(ptyInst *pty.Pty, projectID, projectName string, width, height int) Model {
	termH := height - 1
	if termH < 1 {
		termH = 1
	}
	vterm := vt10x.New(vt10x.WithSize(width, termH))
	return Model{
		ptyInst:     ptyInst,
		vterm:       vterm,
		projectID:   projectID,
		projectName: projectName,
		mode:        ModeNormal,
		width:       width,
		height:      height,
	}
}

func (m Model) Init() tea.Cmd {
	return m.readCmd()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SessionOutputMsg:
		m.vterm.Write(msg.Data)
		return m, m.readCmd()

	case SessionExitedMsg:
		return m, func() tea.Msg { return DetachMsg{} }

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) readCmd() tea.Cmd {
	id, p := m.projectID, m.ptyInst
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := p.Read(buf)
		if err != nil {
			return SessionExitedMsg{ProjectID: id}
		}
		return SessionOutputMsg{ProjectID: id, Data: buf[:n]}
	}
}

func (m Model) Close() {
	m.ptyInst.Close()
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	key := msg.Key()

	if m.mode == ModePrefix {
		m.mode = ModeNormal
		switch {
		case key.Code == 'a' && key.Mod == tea.ModCtrl:
			// Send literal Ctrl+a to PTY
			m.ptyInst.Write([]byte{0x01})
			return m, nil
		case key.Code == 'p', key.Code == 'd':
			// Detach: keep PTY running in background
			return m, func() tea.Msg { return DetachMsg{} }
		case key.Code == 'x':
			// Kill: close PTY and destroy session
			m.ptyInst.Close()
			return m, func() tea.Msg { return KillMsg{} }
		default:
			return m, nil
		}
	}

	// Detect prefix key: Ctrl+a
	if key.Code == 'a' && key.Mod == tea.ModCtrl {
		m.mode = ModePrefix
		return m, nil
	}

	// Normal mode: forward key to PTY
	data := keyToBytes(msg)
	if data != nil {
		m.ptyInst.Write(data)
	}
	return m, nil
}

func (m Model) View() string {
	content := renderANSI(m.vterm)
	bar := statusbar.Render(m.width, m.projectName, m.mode == ModePrefix)
	return content + bar
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	termH := h - 1
	if termH < 1 {
		termH = 1
	}
	m.ptyInst.Resize(w, termH)
	m.vterm.Resize(w, termH)
}

func (m Model) Mode() InputMode {
	return m.mode
}

func (m Model) PrefixKeyPressed() bool {
	return m.mode == ModePrefix
}

// renderANSI renders the terminal state as a string with ANSI SGR color codes.
// It iterates each cell, emits SGR sequences for color/attribute changes, and
// marks the cursor position with reverse video.
func renderANSI(vterm vt10x.Terminal) string {
	cols, rows := vterm.Size()
	cursor := vterm.Cursor()
	cursorVisible := vterm.CursorVisible()

	vterm.Lock()
	defer vterm.Unlock()

	var sb strings.Builder

	// Track current SGR state to minimize redundant sequences.
	var prevFG, prevBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	var prevMode int16 = 0
	rowStart := true

	for y := 0; y < rows; y++ {
		rowStart = true
		for x := 0; x < cols; x++ {
			g := vterm.Cell(x, y)

			fg := g.FG
			bg := g.BG
			mode := g.Mode

			// Show cursor as reverse-video at cursor position.
			atCursor := cursorVisible && x == cursor.X && y == cursor.Y
			if atCursor {
				fg, bg = bg, fg
				mode |= 1 // attrReverse bit
			}

			// Emit SGR when attributes change, or at start of each row.
			if rowStart || fg != prevFG || bg != prevBG || mode != prevMode {
				writeSGR(&sb, fg, bg, mode)
				prevFG = fg
				prevBG = bg
				prevMode = mode
				rowStart = false
			}

			char := g.Char
			if char == 0 {
				char = ' '
			}
			sb.WriteRune(char)
		}
		// Reset SGR at end of each row, then newline.
		sb.WriteString("\x1b[0m\n")
		prevFG, prevBG = vt10x.DefaultFG, vt10x.DefaultBG
		prevMode = 0
	}

	return sb.String()
}

// writeSGR emits a single ANSI SGR escape sequence covering all attributes.
func writeSGR(sb *strings.Builder, fg, bg vt10x.Color, mode int16) {
	sb.WriteString("\x1b[0")

	// Text attributes (bit positions from vt10x/state.go).
	if mode&1 != 0 { // attrReverse
		sb.WriteString(";7")
	}
	if mode&2 != 0 { // attrUnderline
		sb.WriteString(";4")
	}
	if mode&4 != 0 { // attrBold
		sb.WriteString(";1")
	}
	if mode&16 != 0 { // attrItalic
		sb.WriteString(";3")
	}
	if mode&32 != 0 { // attrBlink
		sb.WriteString(";5")
	}

	// Foreground color.
	writeColor(sb, fg, true)

	// Background color.
	writeColor(sb, bg, false)

	sb.WriteByte('m')
}

// writeColor emits the SGR color parameters for fg (isFG=true) or bg (isFG=false).
func writeColor(sb *strings.Builder, c vt10x.Color, isFG bool) {
	// DefaultFG = 1<<24, DefaultBG = 1<<24+1 — use terminal default (no code needed).
	if c >= vt10x.DefaultFG {
		return
	}

	base := 30
	brightBase := 90
	if !isFG {
		base = 40
		brightBase = 100
	}

	switch {
	case c < 8:
		fmt.Fprintf(sb, ";%d", base+int(c))
	case c < 16:
		fmt.Fprintf(sb, ";%d", brightBase+int(c)-8)
	case c <= 255:
		// 256-color palette.
		if isFG {
			fmt.Fprintf(sb, ";38;5;%d", c)
		} else {
			fmt.Fprintf(sb, ";48;5;%d", c)
		}
	default:
		// 24-bit RGB encoded as r<<16 | g<<8 | b.
		r := (c >> 16) & 0xFF
		g := (c >> 8) & 0xFF
		b := c & 0xFF
		if isFG {
			fmt.Fprintf(sb, ";38;2;%d;%d;%d", r, g, b)
		} else {
			fmt.Fprintf(sb, ";48;2;%d;%d;%d", r, g, b)
		}
	}
}

func keyToBytes(msg tea.KeyPressMsg) []byte {
	key := msg.Key()

	// Handle Ctrl combinations.
	if key.Mod&tea.ModCtrl != 0 {
		if key.Code >= 'a' && key.Code <= 'z' {
			return []byte{byte(key.Code - 'a' + 1)}
		}
		if key.Code >= 'A' && key.Code <= 'Z' {
			return []byte{byte(key.Code - 'A' + 1)}
		}
	}

	// Special keys.
	switch key.Code {
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyEscape:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tea.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tea.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tea.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	case tea.KeyHome:
		return []byte{0x1b, '[', 'H'}
	case tea.KeyEnd:
		return []byte{0x1b, '[', 'F'}
	case tea.KeyDelete:
		return []byte{0x1b, '[', '3', '~'}
	case tea.KeyPgUp:
		return []byte{0x1b, '[', '5', '~'}
	case tea.KeyPgDown:
		return []byte{0x1b, '[', '6', '~'}
	}

	// Printable characters.
	if key.Text != "" {
		return []byte(key.Text)
	}

	// Single rune.
	if key.Code > 0 && key.Code < 128 {
		return []byte{byte(key.Code)}
	}

	return nil
}
