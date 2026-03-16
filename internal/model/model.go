package model

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/tanaka0325/clux/internal/session"
	"github.com/tanaka0325/clux/internal/tmux"
)

// Mode represents the current UI mode.
type Mode int

const (
	ModeList Mode = iota
	ModeFilter
	ModeNewSession
	ModeConfirmKill
)

// Custom message types.
type (
	sessionsMsg     []session.Session
	errMsg          error
	tickMsg         time.Time
	ghqDirsMsg      []string
	windowKilledMsg struct{}
	previewMsg      string // pane content for preview
)

// Model is the main Bubble Tea model for Clux.
type Model struct {
	sessions    []session.Session
	filtered    []session.Session // filtered view of sessions
	cursor      int
	mode        Mode
	width       int
	height      int
	err         error
	filterInput textinput.Model

	// Confirm kill mode
	confirmTarget      string // window name for display
	confirmWindowIndex string // window index for tmux command

	// New session mode
	repoDirs         []string // all dirs from ghq
	filteredDirs     []string // filtered dirs
	newSessionInput  textinput.Model
	newSessionCursor int

	// Preview mode
	previewEnabled bool   // toggle state, default false
	previewContent string // captured pane content for selected session
}

// New creates and returns an initialized Model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 64

	ni := textinput.New()
	ni.Placeholder = "Search repository..."
	ni.CharLimit = 64

	return Model{
		mode:            ModeList,
		filterInput:     ti,
		newSessionInput: ni,
	}
}

// --- Accessor methods ---

func (m Model) Sessions() []session.Session  { return m.sessions }
func (m Model) Filtered() []session.Session  { return m.filtered }
func (m Model) Cursor() int                  { return m.cursor }
func (m Model) Mode() Mode                   { return m.mode }
func (m Model) Width() int                   { return m.width }
func (m Model) Height() int                  { return m.height }
func (m Model) FilterInput() textinput.Model { return m.filterInput }
func (m Model) Err() error                   { return m.err }

// --- Command functions ---

func fetchSessionsCmd() tea.Msg {
	sessions, err := tmux.ListWindows()
	if err != nil {
		return errMsg(err)
	}
	return sessionsMsg(sessions)
}

func doTick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchPreviewCmd(windowIndex string) tea.Cmd {
	return func() tea.Msg {
		content, err := tmux.CapturePane(windowIndex)
		if err != nil {
			return previewMsg("")
		}
		return previewMsg(content)
	}
}

func fetchGhqDirs() tea.Msg {
	out, err := exec.Command("ghq", "list", "-p").Output()
	if err != nil {
		return errMsg(fmt.Errorf("ghq list: %w", err))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var dirs []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			dirs = append(dirs, l)
		}
	}
	return ghqDirsMsg(dirs)
}

// applyFilter returns sessions matching the query using fuzzy matching on Name or Dir.
func applyFilter(sessions []session.Session, query string) []session.Session {
	if query == "" {
		return sessions
	}
	var result []session.Session
	seen := make(map[int]bool)

	// Match against names
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}
	for _, m := range fuzzy.Find(query, names) {
		if !seen[m.Index] {
			seen[m.Index] = true
			result = append(result, sessions[m.Index])
		}
	}

	// Match against summaries
	summaries := make([]string, len(sessions))
	for i, s := range sessions {
		summaries[i] = s.Summary
	}
	for _, m := range fuzzy.Find(query, summaries) {
		if !seen[m.Index] {
			seen[m.Index] = true
			result = append(result, sessions[m.Index])
		}
	}

	// Match against dirs
	dirs := make([]string, len(sessions))
	for i, s := range sessions {
		dirs[i] = s.Dir
	}
	for _, m := range fuzzy.Find(query, dirs) {
		if !seen[m.Index] {
			seen[m.Index] = true
			result = append(result, sessions[m.Index])
		}
	}

	return result
}

func filterDirs(dirs []string, query string) []string {
	if query == "" {
		return dirs
	}
	matches := fuzzy.Find(query, dirs)
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = dirs[m.Index]
	}
	return result
}

// --- tea.Model interface ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchSessionsCmd,
		doTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case sessionsMsg:
		m.sessions = []session.Session(msg)
		m.err = nil
		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		// Clamp cursor.
		if len(m.filtered) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.previewEnabled && len(m.filtered) > 0 {
			return m, fetchPreviewCmd(m.filtered[m.cursor].WindowIndex)
		}
		return m, nil

	case previewMsg:
		m.previewContent = string(msg)
		return m, nil

	case errMsg:
		m.err = error(msg)
		return m, nil

	case tickMsg:
		if m.mode == ModeList || m.mode == ModeFilter {
			cmds := []tea.Cmd{fetchSessionsCmd, doTick()}
			if m.previewEnabled && len(m.filtered) > 0 {
				cmds = append(cmds, fetchPreviewCmd(m.filtered[m.cursor].WindowIndex))
			}
			return m, tea.Batch(cmds...)
		}
		return m, doTick()

	case windowKilledMsg:
		return m, fetchSessionsCmd

	case ghqDirsMsg:
		m.repoDirs = []string(msg)
		m.filteredDirs = filterDirs(m.repoDirs, m.newSessionInput.Value())
		if len(m.filteredDirs) == 0 {
			m.newSessionCursor = 0
		} else if m.newSessionCursor >= len(m.filteredDirs) {
			m.newSessionCursor = len(m.filteredDirs) - 1
		}
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+." {
			return m, tea.Quit
		}
		switch m.mode {
		case ModeList:
			return m.updateList(msg)
		case ModeFilter:
			return m.updateFilter(msg)
		case ModeNewSession:
			return m.updateNewSession(msg)
		case ModeConfirmKill:
			return m.updateConfirmKill(msg)
		}
	}

	return m, nil
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "k", "up":
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor - 1 + len(m.filtered)) % len(m.filtered)
			if m.previewEnabled {
				return m, fetchPreviewCmd(m.filtered[m.cursor].WindowIndex)
			}
		}

	case "j", "down":
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor + 1) % len(m.filtered)
			if m.previewEnabled {
				return m, fetchPreviewCmd(m.filtered[m.cursor].WindowIndex)
			}
		}

	case "enter":
		if len(m.filtered) > 0 {
			windowIndex := m.filtered[m.cursor].WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SwitchWindow(windowIndex); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}

	case "n":
		m.mode = ModeNewSession
		m.err = nil
		m.newSessionInput.SetValue("")
		m.newSessionCursor = 0
		cmds := []tea.Cmd{m.newSessionInput.Focus()}
		if len(m.repoDirs) == 0 {
			cmds = append(cmds, fetchGhqDirs)
		} else {
			m.filteredDirs = m.repoDirs
		}
		return m, tea.Batch(cmds...)

	case "K":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			m.confirmTarget = s.DisplayName()
			m.confirmWindowIndex = s.WindowIndex
			m.err = nil
			m.mode = ModeConfirmKill
		}

	case "R":
		return m, fetchSessionsCmd

	case "p":
		m.previewEnabled = !m.previewEnabled
		if m.previewEnabled && len(m.filtered) > 0 {
			return m, fetchPreviewCmd(m.filtered[m.cursor].WindowIndex)
		}
		if !m.previewEnabled {
			m.previewContent = ""
		}

	case "/":
		m.mode = ModeFilter
		return m, m.filterInput.Focus()

	case "q", "esc":
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) updateConfirmKill(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		windowIndex := m.confirmWindowIndex
		m.confirmTarget = ""
		m.confirmWindowIndex = ""
		m.mode = ModeList
		return m, func() tea.Msg {
			if err := tmux.KillWindow(windowIndex); err != nil {
				return errMsg(err)
			}
			// Return a dedicated message, handled by Update to trigger fetch
			return windowKilledMsg{}
		}
	case "n", "esc":
		m.confirmTarget = ""
		m.confirmWindowIndex = ""
		m.mode = ModeList
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Exit filter mode and attach to the selected session directly.
		m.filterInput.Blur()
		m.mode = ModeList
		if len(m.filtered) > 0 {
			windowIndex := m.filtered[m.cursor].WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SwitchWindow(windowIndex); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}
		return m, nil

	case "esc":
		// Clear filter, back to ModeList with full list.
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.mode = ModeList
		m.filtered = m.sessions
		m.cursor = 0
		return m, nil

	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		if m.cursor >= len(m.filtered) {
			m.cursor = 0
		}
		return m, cmd
	}
}

func (m Model) updateNewSession(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.filteredDirs) > 0 {
			dir := m.filteredDirs[m.newSessionCursor]
			if err := tmux.ValidateDir(dir); err != nil {
				m.err = err
				return m, nil
			}
			name := tmux.GenerateWindowName(dir)
			m.newSessionInput.Blur()
			return m, func() tea.Msg {
				if err := tmux.CreateWindow(name, dir); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}
		return m, nil

	case "esc":
		m.mode = ModeList
		m.newSessionInput.Blur()
		return m, nil

	case "up", "ctrl+k":
		if len(m.filteredDirs) > 0 {
			m.newSessionCursor = (m.newSessionCursor - 1 + len(m.filteredDirs)) % len(m.filteredDirs)
		}
		return m, nil

	case "down", "ctrl+j":
		if len(m.filteredDirs) > 0 {
			m.newSessionCursor = (m.newSessionCursor + 1) % len(m.filteredDirs)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.newSessionInput, cmd = m.newSessionInput.Update(msg)
		m.filteredDirs = filterDirs(m.repoDirs, m.newSessionInput.Value())
		if m.newSessionCursor >= len(m.filteredDirs) {
			m.newSessionCursor = 0
		}
		return m, cmd
	}
}

// --- View ---

var (
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleHelpBar  = lipgloss.NewStyle().Faint(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleDir      = lipgloss.NewStyle().Faint(true)
	stylePreview  = lipgloss.NewStyle().Faint(true)

	styleWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleWaiting = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	styleUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
)

func shortenDir(dir string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(dir, home) {
		return "~" + dir[len(home):]
	}
	return dir
}

func statusStyle(s session.Status) lipgloss.Style {
	switch s {
	case session.StatusWorking:
		return styleWorking
	case session.StatusWaiting:
		return styleWaiting
	case session.StatusIdle:
		return styleIdle
	default:
		return styleUnknown
	}
}

// newView creates a tea.View with AltScreen enabled.
func newView(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m Model) View() tea.View {
	var b strings.Builder

	if m.mode == ModeConfirmKill {
		return newView(m.viewConfirmKill(&b))
	}

	if m.mode == ModeNewSession {
		return newView(m.viewNewSession(&b))
	}

	// Header.
	header := fmt.Sprintf("Clux — Sessions (%d)", len(m.filtered))
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n\n")

	// Error display.
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	// Session list.
	if len(m.filtered) == 0 {
		b.WriteString("No Claude Code sessions found. Start Claude Code in another tmux session.\n")
	} else {
		b.WriteString(styleHelpBar.Render(" Status          Name                            Dir"))
		b.WriteString("\n")
		for i, s := range m.filtered {
			statusText := statusStyle(s.Status).Render(fmt.Sprintf("%s %-7s", s.Status.Icon(), s.Status.String()))
			dir := styleDir.Render(shortenDir(s.Dir))
			row := fmt.Sprintf(" %s  %-30s  %s", statusText, s.DisplayName(), dir)
			if i == m.cursor {
				row = styleSelected.Render(row)
			}
			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Preview area (when enabled and terminal is tall enough).
	if m.previewEnabled && len(m.filtered) > 0 && m.height >= 15 {
		// header(2) + column-header(1) + session-rows + blank(1) + helpbar(1) = fixed overhead
		// Reserve lines for session list rows and help bar.
		sessionRows := len(m.filtered)
		// Total fixed overhead: 2 (header) + 1 (col header) + sessionRows + 1 (blank) + 1 (helpbar)
		overhead := 2 + 1 + sessionRows + 1 + 1
		available := m.height - overhead
		// Give roughly half the remaining height to the preview, minimum 5.
		previewHeight := available / 2
		if previewHeight < 5 {
			previewHeight = 5
		}

		// Build separator line.
		sepLabel := " Preview "
		sepWidth := m.width
		if sepWidth <= 0 {
			sepWidth = 80
		}
		sep := strings.Repeat("─", (sepWidth-len(sepLabel))/2) + sepLabel + strings.Repeat("─", (sepWidth-len(sepLabel)+1)/2)
		b.WriteString(stylePreview.Render(sep))
		b.WriteString("\n")

		// Get last N lines from preview content.
		previewLines := strings.Split(m.previewContent, "\n")
		// Remove trailing empty lines.
		for len(previewLines) > 0 && strings.TrimSpace(previewLines[len(previewLines)-1]) == "" {
			previewLines = previewLines[:len(previewLines)-1]
		}
		if len(previewLines) == 0 {
			b.WriteString(stylePreview.Render("  No preview available"))
			b.WriteString("\n")
		} else {
			// Take last previewHeight lines.
			start := len(previewLines) - previewHeight
			if start < 0 {
				start = 0
			}
			displayLines := previewLines[start:]
			maxWidth := m.width
			if maxWidth <= 0 {
				maxWidth = 80
			}
			for _, line := range displayLines {
				// Truncate to terminal width to avoid wrapping.
				runes := []rune(line)
				if len(runes) > maxWidth {
					line = string(runes[:maxWidth])
				}
				b.WriteString(stylePreview.Render(line))
				b.WriteString("\n")
			}
		}
	}

	// Filter input (when in filter mode).
	if m.mode == ModeFilter {
		b.WriteString(" / ")
		b.WriteString(m.filterInput.View())
		b.WriteString("\n\n")
		b.WriteString(styleHelpBar.Render("Enter:apply  Esc:clear"))
	} else {
		previewLabel := "p:preview"
		if m.previewEnabled {
			previewLabel = "p:preview(on)"
		}
		b.WriteString(styleHelpBar.Render("Enter:attach  n:new  K:kill  R:refresh  " + previewLabel + "  /:filter  q:quit"))
	}

	return newView(b.String())
}

func (m Model) viewNewSession(b *strings.Builder) string {
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(styleHeader.Render("New Session — Select Repository"))
	b.WriteString("\n\n")
	b.WriteString(" ")
	b.WriteString(m.newSessionInput.View())
	b.WriteString("\n\n")

	if len(m.filteredDirs) == 0 {
		b.WriteString(styleHelpBar.Render(" No repositories found."))
	} else {
		// Show at most 20 items with viewport offset to keep cursor visible.
		maxShow := 20
		offset := 0
		if m.newSessionCursor >= maxShow {
			offset = m.newSessionCursor - maxShow + 1
		}
		end := offset + maxShow
		if end > len(m.filteredDirs) {
			end = len(m.filteredDirs)
		}
		for i := offset; i < end; i++ {
			dir := shortenDir(m.filteredDirs[i])
			if i == m.newSessionCursor {
				b.WriteString(styleSelected.Render(fmt.Sprintf(" > %s", dir)))
			} else {
				b.WriteString(fmt.Sprintf("   %s", dir))
			}
			b.WriteString("\n")
		}
		if end < len(m.filteredDirs) {
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("\n   ... and %d more", len(m.filteredDirs)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Enter:create  Esc:cancel  ↑/↓:navigate"))

	return b.String()
}

func (m Model) viewConfirmKill(b *strings.Builder) string {
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("Kill session %q? (y/n)", m.confirmTarget))
	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("y:kill  n/Esc:cancel"))

	return b.String()
}
