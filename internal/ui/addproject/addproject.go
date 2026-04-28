package addproject

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/misaelabanto/vibemux/internal/gitops"
)

var (
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

type step int

const (
	stepMenu step = iota
	stepPickParent
	stepEnterName
	stepRunning
)

// CloneFinishedMsg / MkdirFinishedMsg are emitted when the long-running
// operation completes. They're unexported message types delivered via tea.Cmd
// so they only affect this sub-model.
type cloneFinishedMsg struct {
	path string
	err  error
}

type mkdirFinishedMsg struct {
	path string
	err  error
}

type Model struct {
	step step
	mode Mode

	// menu
	menu menuModel

	// parent picker (the original directory navigator)
	currentDir string
	entries    []string
	cursor     int

	// name input (used for both empty-folder name and clone URL)
	nameInput nameInputModel

	// spinner + cancel
	running spinnerModel
	cancel  context.CancelFunc

	// outcome
	selectedPath string

	width  int
	height int
}

func New() Model {
	home, _ := os.UserHomeDir()
	startDir := home

	preferredDirs := []string{"Code", "code", "Projects", "projects"}
	for _, dir := range preferredDirs {
		candidate := filepath.Join(home, dir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			startDir = candidate
			break
		}
	}

	m := Model{
		step:       stepMenu,
		menu:       newMenu(),
		currentDir: startDir,
	}
	m.loadEntries()
	return m
}

func (m *Model) loadEntries() {
	entries, err := os.ReadDir(m.currentDir)
	m.entries = nil
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				m.entries = append(m.entries, e.Name())
			}
		}
		sort.Strings(m.entries)
	}
	m.cursor = 0
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
	}

	switch m.step {
	case stepMenu:
		return m.updateMenu(msg)
	case stepPickParent:
		return m.updatePickParent(msg)
	case stepEnterName:
		return m.updateNameInput(msg)
	case stepRunning:
		return m.updateRunning(msg)
	}
	return m, nil
}

func (m Model) updateMenu(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	if m.menu.chosen {
		m.mode = m.menu.choice
		m.step = stepPickParent
	}
	return m, cmd
}

func (m Model) updatePickParent(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		// Back to the mode menu.
		m.menu = newMenu()
		m.step = stepMenu
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "right", "space":
		if len(m.entries) > 0 {
			m.currentDir = filepath.Join(m.currentDir, m.entries[m.cursor])
			m.loadEntries()
		}
	case "left":
		parent := filepath.Dir(m.currentDir)
		if parent != m.currentDir {
			m.currentDir = parent
			m.loadEntries()
		}
	case "enter":
		if m.mode == ModePickExisting {
			if len(m.entries) > 0 {
				m.selectedPath = filepath.Join(m.currentDir, m.entries[m.cursor])
			} else {
				m.selectedPath = m.currentDir
			}
			return m, nil
		}
		// For Create / Clone, the "enter" key on the picker chooses the
		// current directory as the parent and advances to the name step.
		var title, hint, placeholder string
		switch m.mode {
		case ModeCreateEmpty:
			title = "New folder name:"
			hint = "enter create  esc back"
			placeholder = "my-new-app"
		case ModeClone:
			title = "Repo URL or owner/repo:"
			hint = "enter clone  esc back"
			placeholder = "owner/repo or git@github.com:owner/repo.git"
		}
		m.nameInput = newNameInput(title, hint, placeholder, m.currentDir)
		m.step = stepEnterName
		return m, nil
	}
	return m, nil
}

func (m Model) updateNameInput(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)

	if m.nameInput.canceled {
		m.step = stepPickParent
		m.nameInput.canceled = false
		return m, nil
	}

	if !m.nameInput.submit {
		return m, cmd
	}
	m.nameInput.submit = false

	switch m.mode {
	case ModeCreateEmpty:
		name := strings.TrimSpace(m.nameInput.Value())
		if err := validateFolderName(name); err != nil {
			m.nameInput.SetError(err.Error())
			return m, cmd
		}
		target := filepath.Join(m.currentDir, name)
		if _, err := os.Stat(target); err == nil {
			m.nameInput.SetError(fmt.Sprintf("%q already exists", name))
			return m, cmd
		}
		m.running = newSpinner("Creating " + target)
		m.step = stepRunning
		return m, tea.Batch(m.running.Init(), runMkdir(target))

	case ModeClone:
		url, dirName, err := gitops.NormalizeURL(m.nameInput.Value())
		if err != nil {
			m.nameInput.SetError(err.Error())
			return m, cmd
		}
		target := filepath.Join(m.currentDir, dirName)
		if _, err := os.Stat(target); err == nil {
			m.nameInput.SetError(fmt.Sprintf("%q already exists", dirName))
			return m, cmd
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.running = newSpinner("Cloning " + url)
		m.step = stepRunning
		return m, tea.Batch(m.running.Init(), runClone(ctx, url, m.currentDir, dirName))
	}
	return m, cmd
}

func (m Model) updateRunning(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, nil
		}
	case cloneFinishedMsg:
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		if msg.err != nil {
			m.nameInput.SetError(truncateError(msg.err.Error()))
			m.step = stepEnterName
			return m, nil
		}
		m.selectedPath = msg.path
		return m, nil
	case mkdirFinishedMsg:
		if msg.err != nil {
			m.nameInput.SetError(msg.err.Error())
			m.step = stepEnterName
			return m, nil
		}
		m.selectedPath = msg.path
		return m, nil
	}
	var cmd tea.Cmd
	m.running, cmd = m.running.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	switch m.step {
	case stepMenu:
		return m.menu.View()
	case stepPickParent:
		return m.viewParent()
	case stepEnterName:
		return m.nameInput.View()
	case stepRunning:
		return m.running.View()
	}
	return ""
}

func (m Model) viewParent() string {
	visibleRows := m.height - 8
	if visibleRows < 1 {
		visibleRows = 10
	}

	start := m.cursor - visibleRows/2
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(m.entries) {
		end = len(m.entries)
		start = end - visibleRows
		if start < 0 {
			start = 0
		}
	}

	var rows string
	for i := start; i < end; i++ {
		if i == m.cursor {
			rows += cursorStyle.Render("> "+m.entries[i]) + "\n"
		} else {
			rows += "  " + m.entries[i] + "\n"
		}
	}
	if len(m.entries) == 0 {
		rows = dimStyle.Render("  (no subdirectories)") + "\n"
	}

	var prompt, help string
	switch m.mode {
	case ModePickExisting:
		prompt = "  Pick a project directory:"
		help = "  ↑↓ move  space/→ in  ← out  enter select  esc back  ctrl+c cancel"
	case ModeCreateEmpty:
		prompt = "  Pick a parent directory for the new folder:"
		help = "  ↑↓ move  space/→ in  ← out  enter use this dir  esc back  ctrl+c cancel"
	case ModeClone:
		prompt = "  Pick a parent directory to clone into:"
		help = "  ↑↓ move  space/→ in  ← out  enter use this dir  esc back  ctrl+c cancel"
	}

	path := dimStyle.Render("  " + m.currentDir)
	return "\n" + prompt + "\n" + path + "\n\n" + rows + "\n" + helpStyle.Render(help) + "\n"
}

func (m Model) SelectedPath() string {
	return m.selectedPath
}

func (m *Model) ClearSelection() {
	m.selectedPath = ""
}

// IsRunning reports whether a long-running operation (git clone, mkdir) is
// in progress. The parent uses this to decide whether ctrl+c should cancel
// the whole add-project flow or just the running operation.
func (m Model) IsRunning() bool {
	return m.step == stepRunning
}

// Canceled reports whether the user asked to abort the entire add-project
// flow (esc on the menu / parent picker / name input). The parent should
// dismiss the view when this is true.
func (m Model) Canceled() bool {
	return m.menu.canceled
}

// Cancel kills any in-flight clone process. Safe to call when nothing is running.
func (m *Model) Cancel() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func validateFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name must not contain path separators")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("name is not valid")
	}
	return nil
}

func truncateError(s string) string {
	const max = 240
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func runMkdir(target string) tea.Cmd {
	return func() tea.Msg {
		err := os.MkdirAll(target, 0o755)
		return mkdirFinishedMsg{path: target, err: err}
	}
}

func runClone(ctx context.Context, url, parent, dirName string) tea.Cmd {
	return func() tea.Msg {
		path, err := gitops.Clone(ctx, url, parent, dirName)
		return cloneFinishedMsg{path: path, err: err}
	}
}
