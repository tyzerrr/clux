package model

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/tanaka0325/clux/internal/config"
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
	ModeAddExternal
)

// Custom message types.
type (
	sessionsMsg          []session.Session
	errMsg               error
	tickMsg              time.Time
	ghqDirsMsg           []string
	windowKilledMsg      struct{}
	sessionUnregistered  struct{} // external session unregistered from config
	previewMsg           string   // pane content for preview
	externalWindowsMsg   []tmux.ExternalWindowInfo
	externalAddedMsg     struct{} // session registered successfully
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
	cfg         *config.Config // persistent config for external sessions

	// Confirm kill mode
	confirmTarget      string // window name for display
	confirmWindowIndex string // window index for tmux command
	confirmExternal    bool   // true if confirming unregister (not kill)
	confirmSessionName string // tmux session name for external sessions

	// New session mode
	repoDirs         []string // all dirs from ghq
	filteredDirs     []string // filtered dirs
	newSessionInput  textinput.Model
	newSessionCursor int

	// Preview mode
	previewEnabled bool   // toggle state, default false
	previewContent string // captured pane content for selected session

	// Add external session mode
	externalWindows    []tmux.ExternalWindowInfo // all available windows
	filteredExtWindows []tmux.ExternalWindowInfo // filtered by search
	addExtInput        textinput.Model
	addExtCursor       int

	// Status change tracking for bell notification
	prevStatuses map[string]session.Status
}

// New creates and returns an initialized Model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 64

	ni := textinput.New()
	ni.Placeholder = "Search repository..."
	ni.CharLimit = 64

	ai := textinput.New()
	ai.Placeholder = "Search session:window..."
	ai.CharLimit = 64

	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.Config{}
	}

	return Model{
		mode:            ModeList,
		filterInput:     ti,
		newSessionInput: ni,
		addExtInput:     ai,
		cfg:             cfg,
		previewEnabled:  cfg.PreviewDefault,
		prevStatuses:    make(map[string]session.Status),
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

func fetchSessionsCmdWithExternals(cfg *config.Config) func() tea.Msg {
	return func() tea.Msg {
		sessions, err := tmux.ListWindows()
		if err != nil {
			return errMsg(err)
		}
		if cfg != nil && len(cfg.ExternalSessions) > 0 {
			ext := tmux.ListExternalWindows(cfg.ExternalSessions)
			sessions = append(sessions, ext...)
		}
		return sessionsMsg(sessions)
	}
}

func doTick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchPreviewCmdForSession(s session.Session) tea.Cmd {
	return func() tea.Msg {
		sessionName := tmux.SessionName
		if s.External && s.SessionName != "" {
			sessionName = s.SessionName
		}
		content, err := tmux.CapturePaneForSession(sessionName, s.WindowIndex)
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

	// Match against summaries (skip empty summaries to avoid false positives)
	summaries := make([]string, len(sessions))
	for i, s := range sessions {
		summaries[i] = s.Summary
	}
	for _, m := range fuzzy.Find(query, summaries) {
		if summaries[m.Index] == "" {
			continue
		}
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

// sortByStatus sorts sessions so that those needing attention appear first.
// Priority order: Waiting (3) > Working (2) > Idle (1) > Unknown (0).
func sortByStatus(sessions []session.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return statusPriority(sessions[i].Status) > statusPriority(sessions[j].Status)
	})
}

func statusPriority(s session.Status) int {
	switch s {
	case session.StatusWaiting:
		return 3
	case session.StatusWorking:
		return 2
	case session.StatusIdle:
		return 1
	default:
		return 0
	}
}

// ringBell returns a command that prints a terminal bell character.
func ringBell() tea.Cmd {
	return tea.Println("\a")
}

func fetchExternalWindowsCmd() tea.Msg {
	windows, err := tmux.ListAllWindows()
	if err != nil {
		return errMsg(err)
	}
	return externalWindowsMsg(windows)
}

// filterExtWindows does fuzzy matching on "session:index windowname dir" combined.
func filterExtWindows(windows []tmux.ExternalWindowInfo, query string) []tmux.ExternalWindowInfo {
	if query == "" {
		return windows
	}
	combined := make([]string, len(windows))
	for i, w := range windows {
		combined[i] = w.Session + ":" + w.WindowIndex + " " + w.WindowName + " " + w.Dir
	}
	matches := fuzzy.Find(query, combined)
	result := make([]tmux.ExternalWindowInfo, len(matches))
	for i, m := range matches {
		result[i] = windows[m.Index]
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
		fetchSessionsCmdWithExternals(m.cfg),
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

		// Check for Working→Waiting transitions and update prevStatuses.
		shouldBell := false
		for _, s := range m.sessions {
			key := s.SessionName + ":" + s.WindowIndex
			if s.SessionName == "" {
				key = tmux.SessionName + ":" + s.WindowIndex
			}
			prev, exists := m.prevStatuses[key]
			if exists && prev == session.StatusWorking && s.Status == session.StatusWaiting {
				shouldBell = true
			}
			m.prevStatuses[key] = s.Status
		}

		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		sortByStatus(m.filtered)
		// Clamp cursor.
		if len(m.filtered) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}

		var cmds []tea.Cmd
		if shouldBell {
			cmds = append(cmds, ringBell())
		}
		if m.previewEnabled && len(m.filtered) > 0 {
			cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.cursor]))
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
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
			cmds := []tea.Cmd{fetchSessionsCmdWithExternals(m.cfg), doTick()}
			if m.previewEnabled && len(m.filtered) > 0 {
				cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.cursor]))
			}
			return m, tea.Batch(cmds...)
		}
		return m, doTick()

	case windowKilledMsg:
		return m, fetchSessionsCmdWithExternals(m.cfg)

	case sessionUnregistered:
		return m, fetchSessionsCmdWithExternals(m.cfg)

	case ghqDirsMsg:
		m.repoDirs = []string(msg)
		m.filteredDirs = filterDirs(m.repoDirs, m.newSessionInput.Value())
		if len(m.filteredDirs) == 0 {
			m.newSessionCursor = 0
		} else if m.newSessionCursor >= len(m.filteredDirs) {
			m.newSessionCursor = len(m.filteredDirs) - 1
		}
		return m, nil

	case externalWindowsMsg:
		m.externalWindows = []tmux.ExternalWindowInfo(msg)
		filtered := filterExtWindows(m.externalWindows, m.addExtInput.Value())
		m.filteredExtWindows = excludeRegistered(filtered, m.cfg)
		if len(m.filteredExtWindows) == 0 {
			m.addExtCursor = 0
		} else if m.addExtCursor >= len(m.filteredExtWindows) {
			m.addExtCursor = len(m.filteredExtWindows) - 1
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
		case ModeAddExternal:
			return m.updateAddExternal(msg)
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
				return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
			}
		}

	case "j", "down":
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor + 1) % len(m.filtered)
			if m.previewEnabled {
				return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
			}
		}

	case "enter":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			windowIndex := s.WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, windowIndex); err != nil {
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

	case "a":
		m.mode = ModeAddExternal
		m.err = nil
		m.addExtInput.SetValue("")
		m.addExtCursor = 0
		cmds := []tea.Cmd{m.addExtInput.Focus(), fetchExternalWindowsCmd}
		return m, tea.Batch(cmds...)

	case "K":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			m.confirmTarget = s.DisplayName()
			m.confirmWindowIndex = s.WindowIndex
			m.confirmExternal = s.External
			m.confirmSessionName = s.SessionName
			m.err = nil
			m.mode = ModeConfirmKill
		}

	case "y":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			if s.Status != session.StatusWaiting {
				break
			}
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			windowIndex := s.WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SendKeys(sessionName, windowIndex, "y"); err != nil {
					return errMsg(err)
				}
				return fetchSessionsCmdWithExternals(m.cfg)()
			}
		}

	case "R":
		return m, fetchSessionsCmdWithExternals(m.cfg)

	case "p":
		m.previewEnabled = !m.previewEnabled
		if m.previewEnabled && len(m.filtered) > 0 {
			return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
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
		isExternal := m.confirmExternal
		extSessionName := m.confirmSessionName
		cfg := m.cfg
		m.confirmTarget = ""
		m.confirmWindowIndex = ""
		m.confirmExternal = false
		m.confirmSessionName = ""
		m.mode = ModeList
		if isExternal {
			// Unregister external session (don't kill the window).
			return m, func() tea.Msg {
				if err := cfg.Remove(extSessionName, windowIndex); err != nil {
					return errMsg(err)
				}
				if err := cfg.Save(); err != nil {
					return errMsg(err)
				}
				return sessionUnregistered{}
			}
		}
		return m, func() tea.Msg {
			if err := tmux.KillWindow(windowIndex); err != nil {
				return errMsg(err)
			}
			return windowKilledMsg{}
		}
	case "n", "esc":
		m.confirmTarget = ""
		m.confirmWindowIndex = ""
		m.confirmExternal = false
		m.confirmSessionName = ""
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
			s := m.filtered[m.cursor]
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			windowIndex := s.WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, windowIndex); err != nil {
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
		sortByStatus(m.filtered)
		m.cursor = 0
		return m, nil

	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		sortByStatus(m.filtered)
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

func (m Model) updateAddExternal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.filteredExtWindows) > 0 {
			ext := m.filteredExtWindows[m.addExtCursor]
			if err := m.cfg.Add(ext.Session, ext.WindowIndex); err != nil {
				m.err = err
				return m, nil
			}
			if err := m.cfg.Save(); err != nil {
				m.err = err
				return m, nil
			}
			m.mode = ModeList
			m.addExtInput.Blur()
			return m, fetchSessionsCmdWithExternals(m.cfg)
		}
		return m, nil

	case "esc":
		m.mode = ModeList
		m.addExtInput.Blur()
		return m, nil

	case "up", "ctrl+k":
		if len(m.filteredExtWindows) > 0 {
			m.addExtCursor = (m.addExtCursor - 1 + len(m.filteredExtWindows)) % len(m.filteredExtWindows)
		}
		return m, nil

	case "down", "ctrl+j":
		if len(m.filteredExtWindows) > 0 {
			m.addExtCursor = (m.addExtCursor + 1) % len(m.filteredExtWindows)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.addExtInput, cmd = m.addExtInput.Update(msg)
		// Re-filter; also exclude already-registered windows.
		allFiltered := filterExtWindows(m.externalWindows, m.addExtInput.Value())
		m.filteredExtWindows = excludeRegistered(allFiltered, m.cfg)
		if m.addExtCursor >= len(m.filteredExtWindows) {
			m.addExtCursor = 0
		}
		return m, cmd
	}
}

// excludeRegistered removes windows that are already registered in cfg.
func excludeRegistered(windows []tmux.ExternalWindowInfo, cfg *config.Config) []tmux.ExternalWindowInfo {
	if cfg == nil || len(cfg.ExternalSessions) == 0 {
		return windows
	}
	registered := make(map[string]bool, len(cfg.ExternalSessions))
	for _, es := range cfg.ExternalSessions {
		registered[es.Session+":"+es.Window] = true
	}
	var out []tmux.ExternalWindowInfo
	for _, w := range windows {
		if !registered[w.Session+":"+w.WindowIndex] {
			out = append(out, w)
		}
	}
	return out
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

	if m.mode == ModeAddExternal {
		return newView(m.viewAddExternal(&b))
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
		b.WriteString(styleHelpBar.Render(" Status          Name                            Branch          Dir"))
		b.WriteString("\n")
		for i, s := range m.filtered {
			statusText := statusStyle(s.Status).Render(fmt.Sprintf("%s %-7s", s.Status.Icon(), s.Status.String()))
			branch := s.Branch
			if len(branch) > 15 {
				branch = branch[:14] + "…"
			}
			dir := styleDir.Render(shortenDir(s.Dir))
			row := fmt.Sprintf(" %s  %-30s  %-15s %s", statusText, s.DisplayName(), branch, dir)
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
			for _, line := range displayLines {
				// Output directly to preserve ANSI color sequences.
				b.WriteString(line)
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
		b.WriteString(styleHelpBar.Render("Enter:attach  y:approve  n:new  a:add-ext  K:kill  R:refresh  " + previewLabel + "  /:filter  q:quit"))
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

func (m Model) viewAddExternal(b *strings.Builder) string {
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	b.WriteString(styleHeader.Render("Add External Session"))
	b.WriteString("\n\n")
	b.WriteString(" ")
	b.WriteString(m.addExtInput.View())
	b.WriteString("\n\n")

	if len(m.filteredExtWindows) == 0 {
		b.WriteString(styleHelpBar.Render(" No external windows found."))
	} else {
		// Show at most 20 items with viewport offset to keep cursor visible.
		maxShow := 20
		offset := 0
		if m.addExtCursor >= maxShow {
			offset = m.addExtCursor - maxShow + 1
		}
		end := offset + maxShow
		if end > len(m.filteredExtWindows) {
			end = len(m.filteredExtWindows)
		}
		for i := offset; i < end; i++ {
			w := m.filteredExtWindows[i]
			dir := styleDir.Render(shortenDir(w.Dir))
			row := fmt.Sprintf(" %s:%s  %-20s  %s", w.Session, w.WindowIndex, w.WindowName, dir)
			if i == m.addExtCursor {
				b.WriteString(styleSelected.Render(row))
			} else {
				b.WriteString(row)
			}
			b.WriteString("\n")
		}
		if end < len(m.filteredExtWindows) {
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("\n   ... and %d more", len(m.filteredExtWindows)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Enter:add  Esc:cancel  ↑/↓:navigate"))

	return b.String()
}

func (m Model) viewConfirmKill(b *strings.Builder) string {
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}
	if m.confirmExternal {
		b.WriteString(fmt.Sprintf("Unregister external session %q? (y/n)", m.confirmTarget))
		b.WriteString("\n\n")
		b.WriteString(styleHelpBar.Render("y:unregister  n/Esc:cancel"))
	} else {
		b.WriteString(fmt.Sprintf("Kill session %q? (y/n)", m.confirmTarget))
		b.WriteString("\n\n")
		b.WriteString(styleHelpBar.Render("y:kill  n/Esc:cancel"))
	}

	return b.String()
}
