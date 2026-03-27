package model

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"
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
	ModeDashboard
	ModeBroadcastSelect // ghq repo multi-select for broadcast
	ModeBroadcastPrompt // prompt/skill input for broadcast
	ModeDashboardPrompt // overlay text input on dashboard
	ModeListPrompt      // overlay text input on list
)

// Custom message types.
type (
	sessionsMsg         []session.Session
	errMsg              error
	tickMsg             time.Time
	ghqDirsMsg          []string
	windowKilledMsg     struct{}
	previewMsg          string // pane content for preview
	dashPreviewsMsg     map[int]string
	broadcastGhqDirsMsg []string // ghq dirs for broadcast select (separate from newSession)
	ghqRootMsg          string   // cached ghq root path
)

// broadcastCompletedMsg is returned after all broadcast windows have been created.
type broadcastCompletedMsg struct {
	errors []string
}

// broadcastTarget holds the directory for a broadcast send.
type broadcastTarget struct {
	dir string
}

// overlayCursor represents the cursor position within overlay content (before border/padding).
type overlayCursor struct {
	x, y int
}

// ListState holds state for the main list view.
type ListState struct {
	cursor int
}

// DashboardState holds state for the dashboard view.
type DashboardState struct {
	cursor      int            // index into m.filtered for focused cell
	previews    map[int]string // windowIndex -> pane content for each session
	pageOffset  int            // index of first displayed item in dashboard
	promptInput textinput.Model // inline input for sending text to a session from dashboard
}

// BroadcastState holds state for broadcast mode.
type BroadcastState struct {
	dirs        []string           // all ghq dirs (loaded once)
	filtered    []string           // filtered by input
	selected    map[string]bool    // selected dir paths (persists across filter changes)
	input       textinput.Model    // filter input for repo select
	cursor      int                // cursor in filtered
	promptInput textinput.Model    // prompt text input
	targets     []broadcastTarget  // resolved targets after selection
	errors      []string           // dir validation errors collected during resolution
}

// NewSessionState holds state for the new session overlay.
type NewSessionState struct {
	input      textinput.Model
	cursor     int
	returnMode Mode
}

// ConfirmState holds state for kill confirmation.
type ConfirmState struct {
	target      string // window name for display
	windowIndex string // window index for tmux command
	paneIndex   string // pane index for tmux command
	sessionName string // tmux session name
	returnMode  Mode   // mode to return to after confirm
}

// PreviewState holds state for the preview panel.
type PreviewState struct {
	enabled      bool   // toggle state
	content      string // captured pane content for selected session
	scrollOffset int    // lines scrolled up from bottom; 0 = live view
}

// Model is the main Bubble Tea model for Clux.
type Model struct {
	// Shared state
	sessions      []session.Session
	filtered      []session.Session // filtered view of sessions
	mode          Mode
	width         int
	height        int
	err           error
	filterInput   textinput.Model
	cfg           *config.Config // persistent config
	configErr     error          // non-nil if config file failed to load (app uses defaults)
	ghqRoot       string         // cached result of `ghq root`
	repoDirs      []string       // all dirs from ghq
	filteredDirs  []string       // filtered dirs for new session overlay
	prevStatuses  map[string]session.Status
	dashboardOnly bool // when true, Esc/q quits the app (popup mode)
	groupEnabled  bool // toggle for grouped display

	// Sub-models
	list      ListState
	dashboard DashboardState
	broadcast BroadcastState
	newSess   NewSessionState
	confirm   ConfirmState
	preview   PreviewState
}

// New creates and returns an initialized Model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 64

	ni := textinput.New()
	ni.Placeholder = "Search repository..."
	ni.CharLimit = 64

	bci := textinput.New()
	bci.Placeholder = "Search repository..."
	bci.CharLimit = 64

	bpi := textinput.New()
	bpi.Placeholder = "Prompt or skill to broadcast..."
	bpi.CharLimit = 512

	di := textinput.New()
	di.Placeholder = "Send to session..."
	di.CharLimit = 512

	cfg, configErr := config.Load()
	if cfg == nil {
		cfg = config.NewConfig()
	}

	return Model{
		mode:        ModeList,
		filterInput: ti,
		cfg:         cfg,
		configErr:   configErr,
		groupEnabled: *cfg.GroupDefault,
		prevStatuses: make(map[string]session.Status),
		newSess: NewSessionState{
			input: ni,
		},
		broadcast: BroadcastState{
			input:       bci,
			promptInput: bpi,
			selected:    make(map[string]bool),
		},
		dashboard: DashboardState{
			previews:    make(map[int]string),
			promptInput: di,
		},
		preview: PreviewState{
			enabled: *cfg.PreviewDefault,
		},
	}
}

// NewDashboard creates a Model that starts directly in dashboard mode.
// When dashboardOnly is true, Esc/q quits the app instead of returning to list.
func NewDashboard() Model {
	m := New()
	m.mode = ModeDashboard
	m.dashboard.previews = make(map[int]string)
	m.dashboardOnly = true
	return m
}

// --- Accessor methods ---

func (m Model) Sessions() []session.Session  { return m.sessions }
func (m Model) Filtered() []session.Session  { return m.filtered }
func (m Model) Cursor() int                  { return m.list.cursor }
func (m Model) Mode() Mode                   { return m.mode }
func (m Model) Width() int                   { return m.width }
func (m Model) Height() int                  { return m.height }
func (m Model) FilterInput() textinput.Model { return m.filterInput }
func (m Model) Err() error                   { return m.err }
func (m Model) GroupEnabled() bool            { return m.groupEnabled }

// --- Command functions ---

func fetchSessionsCmd() tea.Msg {
	sessions, err := tmux.ListWindows()
	if err != nil {
		return errMsg(err)
	}
	return sessionsMsg(sessions)
}

func doTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func doImmediateTick() tea.Cmd {
	return func() tea.Msg {
		return tickMsg(time.Now())
	}
}

func fetchPreviewCmdForSession(s session.Session) tea.Cmd {
	return func() tea.Msg {
		sessionName := s.SessionName
		if sessionName == "" {
			sessionName = tmux.SessionName
		}
		paneIndex := resolvePaneIndex(s.PaneIndex)
		content, err := tmux.CapturePaneForSession(sessionName, s.WindowIndex, paneIndex)
		if err != nil {
			return previewMsg("")
		}
		return previewMsg(content)
	}
}

func fetchPreviewCmdForSessionWithOffset(s session.Session, scrollOffset, height int) tea.Cmd {
	return func() tea.Msg {
		sessionName := s.SessionName
		if sessionName == "" {
			sessionName = tmux.SessionName
		}
		paneIndex := resolvePaneIndex(s.PaneIndex)
		content, err := tmux.CapturePaneForSessionWithOffset(sessionName, s.WindowIndex, paneIndex, scrollOffset, height)
		if err != nil {
			return previewMsg("")
		}
		return previewMsg(content)
	}
}

func fetchDashboardPreviewWithScrollback(s session.Session, tileIdx int) tea.Cmd {
	return func() tea.Msg {
		sessionName := s.SessionName
		if sessionName == "" {
			sessionName = tmux.SessionName
		}
		paneIndex := resolvePaneIndex(s.PaneIndex)
		content, err := tmux.CapturePaneWithScrollback(sessionName, s.WindowIndex, paneIndex, dashScrollbackLines)
		if err != nil {
			return dashPreviewsMsg(map[int]string{tileIdx: ""})
		}
		return dashPreviewsMsg(map[int]string{tileIdx: content})
	}
}

func fetchDashboardPreviews(sessions []session.Session, skipIdx ...int) tea.Cmd {
	return func() tea.Msg {
		skip := -1
		if len(skipIdx) > 0 {
			skip = skipIdx[0]
		}
		result := make(map[int]string, len(sessions))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i, s := range sessions {
			if i == skip {
				continue
			}
			wg.Add(1)
			go func(idx int, s session.Session) {
				defer wg.Done()
				sessionName := s.SessionName
				if sessionName == "" {
					sessionName = tmux.SessionName
				}
				paneIndex := resolvePaneIndex(s.PaneIndex)
				content, err := tmux.CapturePaneForSession(sessionName, s.WindowIndex, paneIndex)
				mu.Lock()
				if err == nil {
					result[idx] = content
				}
				mu.Unlock()
			}(i, s)
		}
		wg.Wait()
		return dashPreviewsMsg(result)
	}
}

func listGhqDirs() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ghq", "list", "-p").Output()
	if err != nil {
		return nil, fmt.Errorf("ghq list: %w", err)
	}
	var dirs []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			dirs = append(dirs, l)
		}
	}
	return dirs, nil
}

func fetchGhqDirs() tea.Msg {
	dirs, err := listGhqDirs()
	if err != nil {
		return errMsg(err)
	}
	return ghqDirsMsg(dirs)
}

func fetchBroadcastGhqDirs() tea.Msg {
	dirs, err := listGhqDirs()
	if err != nil {
		return errMsg(err)
	}
	return broadcastGhqDirsMsg(dirs)
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

	// Match against branches
	branches := make([]string, len(sessions))
	for i, s := range sessions {
		branches[i] = s.Branch
	}
	for _, m := range fuzzy.Find(query, branches) {
		if branches[m.Index] == "" {
			continue
		}
		if !seen[m.Index] {
			seen[m.Index] = true
			result = append(result, sessions[m.Index])
		}
	}

	return result
}

// ringBell returns a command that prints a terminal bell character.
func ringBell() tea.Cmd {
	return tea.Println("\a")
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

// --- Grouping ---

// sessionGroup holds a group of sessions for display.
type sessionGroup struct {
	name     string
	sessions []indexedSession
}

// indexedSession pairs a session with its index in the filtered slice.
type indexedSession struct {
	index   int
	session session.Session
}

func fetchGhqRoot() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ghq", "root").Output()
	if err != nil {
		return ghqRootMsg("")
	}
	root := strings.TrimSpace(string(out))
	if root != "" && !strings.HasSuffix(root, "/") {
		root += "/"
	}
	return ghqRootMsg(root)
}

// groupKey returns the group key for a session based on its Dir.
func groupKey(s session.Session, ghqRoot string) string {
	dir := s.Dir
	// Strip worktree paths to group by base repo.
	if idx := strings.Index(dir, "/.claude/worktrees/"); idx >= 0 {
		dir = dir[:idx]
	}
	if ghqRoot != "" && strings.HasPrefix(dir, ghqRoot) {
		return strings.TrimPrefix(dir, ghqRoot)
	}
	// Fallback: use last 2 path components.
	parts := strings.Split(dir, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return dir
}

// buildGroups creates sorted session groups from filtered sessions.
func buildGroups(sessions []session.Session, ghqRoot string) []sessionGroup {
	groups := make(map[string][]indexedSession)
	var order []string
	for i, s := range sessions {
		key := groupKey(s, ghqRoot)
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], indexedSession{index: i, session: s})
	}

	// Convert map to sorted slice.
	result := make([]sessionGroup, 0, len(groups))
	for _, name := range order {
		result = append(result, sessionGroup{name: name, sessions: groups[name]})
	}

	// Sort groups by activity (most active first).
	sort.SliceStable(result, func(i, j int) bool {
		return groupPriority(result[i]) > groupPriority(result[j])
	})

	return result
}

// groupPriority returns the highest status priority among sessions in the group.
func groupPriority(g sessionGroup) int {
	maxPri := 0
	for _, is := range g.sessions {
		if p := statusPriority(is.session.Status); p > maxPri {
			maxPri = p
		}
	}
	return maxPri
}

// groupedSessionRows calculates the total visual rows in grouped mode
// (group headers + session rows).
func groupedSessionRows(groups []sessionGroup) int {
	total := 0
	for _, g := range groups {
		total++ // group header
		total += len(g.sessions)
	}
	return total
}

// --- tea.Model interface ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchSessionsCmd,
		doImmediateTick(),
		fetchGhqRoot,
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

		// Check for status transitions and update prevStatuses.
		shouldBell := false
		for _, s := range m.sessions {
			paneIdx := resolvePaneIndex(s.PaneIndex)
			sessionName := s.SessionName
			if sessionName == "" {
				sessionName = tmux.SessionName
			}
			key := sessionName + ":" + s.WindowIndex + "." + paneIdx
			prev, exists := m.prevStatuses[key]
			if exists {
				if prev == session.StatusWorking && s.Status == session.StatusWaiting && m.cfg.Notifications.ShouldNotifyWorkingToWaiting() {
					shouldBell = true
				}
				if prev == session.StatusWorking && s.Status == session.StatusIdle && m.cfg.Notifications.ShouldNotifyWorkingToIdle() {
					shouldBell = true
				}
			}
			m.prevStatuses[key] = s.Status
		}

		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		m.list.cursor = clampCursor(m.list.cursor, len(m.filtered))

		if m.mode == ModeDashboard || m.mode == ModeDashboardPrompt {
			maxVisible := m.dashMaxVisible()
			// Snap page offset to a valid page boundary.
			if maxVisible > 0 {
				m.dashboard.pageOffset = (m.dashboard.pageOffset / maxVisible) * maxVisible
			}
			if m.dashboard.pageOffset >= len(m.filtered) {
				m.dashboard.pageOffset = 0
			}
			pageCount := len(m.filtered) - m.dashboard.pageOffset
			if pageCount > maxVisible {
				pageCount = maxVisible
			}
			if pageCount < 0 {
				pageCount = 0
			}
			if m.dashboard.cursor >= pageCount {
				m.dashboard.cursor = 0
				m.preview.scrollOffset = 0
			}
		}

		var cmds []tea.Cmd
		if shouldBell {
			cmds = append(cmds, ringBell())
		}
		if m.preview.enabled && len(m.filtered) > 0 {
			if m.preview.scrollOffset > 0 {
				cmds = append(cmds, fetchPreviewCmdForSessionWithOffset(m.filtered[m.list.cursor], m.preview.scrollOffset, previewHeight(m)))
			} else {
				cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.list.cursor]))
			}
		}

		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case previewMsg:
		m.preview.content = string(msg)
		return m, nil

	case dashPreviewsMsg:
		if m.dashboard.previews == nil {
			m.dashboard.previews = make(map[int]string)
		}
		for k, v := range map[int]string(msg) {
			m.dashboard.previews[k] = v
		}
		return m, nil

	case errMsg:
		m.err = error(msg)
		return m, nil

	case tickMsg:
		switch m.mode { //nolint:exhaustive
		case ModeList, ModeFilter, ModeListPrompt:
			cmds := []tea.Cmd{fetchSessionsCmd, doTick()}
			if m.preview.enabled && len(m.filtered) > 0 {
				if m.preview.scrollOffset > 0 {
					cmds = append(cmds, fetchPreviewCmdForSessionWithOffset(m.filtered[m.list.cursor], m.preview.scrollOffset, previewHeight(m)))
				} else {
					cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.list.cursor]))
				}
			}
			return m, tea.Batch(cmds...)
		case ModeDashboard, ModeDashboardPrompt:
			pageItems := len(m.filtered) - m.dashboard.pageOffset
			maxVisible := m.dashMaxVisible()
			if pageItems > maxVisible {
				pageItems = maxVisible
			}
			if pageItems < 0 {
				pageItems = 0
			}
			pageSessions := m.filtered[m.dashboard.pageOffset : m.dashboard.pageOffset+pageItems]
			cmds := []tea.Cmd{fetchSessionsCmd, doTick()}
			if m.preview.scrollOffset > 0 && m.dashboard.cursor < len(pageSessions) {
				cmds = append(cmds, fetchDashboardPreviews(pageSessions, m.dashboard.cursor))
			} else {
				cmds = append(cmds, fetchDashboardPreviews(pageSessions))
			}
			return m, tea.Batch(cmds...)
		default:
			return m, doTick()
		}

	case windowKilledMsg:
		return m, fetchSessionsCmd

	case ghqDirsMsg:
		m.repoDirs = []string(msg)
		m.filteredDirs = filterDirs(m.repoDirs, m.newSess.input.Value())
		m.newSess.cursor = clampCursor(m.newSess.cursor, len(m.filteredDirs))
		return m, nil

	case broadcastGhqDirsMsg:
		m.broadcast.dirs = []string(msg)
		m.broadcast.filtered = filterDirs(m.broadcast.dirs, m.broadcast.input.Value())
		m.broadcast.cursor = 0
		return m, nil

	case broadcastCompletedMsg:
		m.mode = ModeList
		m.broadcast.errors = msg.errors
		return m, fetchSessionsCmd

	case ghqRootMsg:
		m.ghqRoot = string(msg)
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
		case ModeDashboard:
			return m.updateDashboard(msg)
		case ModeDashboardPrompt:
			return m.updateDashboardPrompt(msg)
		case ModeListPrompt:
			return m.updateListPrompt(msg)
		case ModeBroadcastSelect:
			return m.updateBroadcastSelect(msg)
		case ModeBroadcastPrompt:
			return m.updateBroadcastPrompt(msg)
		}

	case tea.PasteMsg:
		switch m.mode {
		case ModeFilter:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filtered = applyFilter(m.sessions, m.filterInput.Value())
			if m.list.cursor >= len(m.filtered) {
				m.list.cursor = 0
			}
			return m, cmd

		case ModeNewSession:
			var cmd tea.Cmd
			m.newSess.input, cmd = m.newSess.input.Update(msg)
			m.filteredDirs = filterDirs(m.repoDirs, m.newSess.input.Value())
			if m.newSess.cursor >= len(m.filteredDirs) {
				m.newSess.cursor = 0
			}
			return m, cmd

		case ModeBroadcastSelect:
			var cmd tea.Cmd
			m.broadcast.input, cmd = m.broadcast.input.Update(msg)
			m.broadcast.filtered = filterDirs(m.broadcast.dirs, m.broadcast.input.Value())
			if m.broadcast.cursor >= len(m.broadcast.filtered) {
				m.broadcast.cursor = 0
			}
			return m, cmd

		case ModeBroadcastPrompt:
			var cmd tea.Cmd
			m.broadcast.promptInput, cmd = m.broadcast.promptInput.Update(msg)
			return m, cmd

		case ModeDashboardPrompt, ModeListPrompt:
			var cmd tea.Cmd
			m.dashboard.promptInput, cmd = m.dashboard.promptInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// --- Dashboard layout helpers ---

// dashCols returns the number of columns for the dashboard grid based on terminal width and session count.
func (m Model) dashCols() int {
	n := len(m.filtered)
	if n == 0 {
		return 1
	}
	maxColsByWidth := 1
	if m.width >= 180 {
		maxColsByWidth = 4
	} else if m.width >= 120 {
		maxColsByWidth = 3
	} else if m.width >= 80 {
		maxColsByWidth = 2
	}
	if n < maxColsByWidth {
		return n
	}
	return maxColsByWidth
}

// dashMaxVisible returns the maximum number of dashboard cells visible at once.
func (m Model) dashMaxVisible() int {
	cols := m.dashCols()
	headerLines := 2
	if m.configErr != nil {
		headerLines += 2
	}
	helpLines := 2
	borderHeight := 2                      // lipgloss RoundedBorder adds top + bottom border lines per row
	minCellHeight := 7 + borderHeight + 1  // minimum usable cell height including border and trailing newline
	available := m.height - headerLines - helpLines
	if available < minCellHeight {
		return cols // at least one row
	}
	maxRows := available / minCellHeight
	if maxRows < 1 {
		maxRows = 1
	}
	return cols * maxRows
}

// previewHeight returns the number of lines available for preview content.
// With right-panel layout, preview uses the full height minus header (1 line) and helpbar (1 line).
func previewHeight(m Model) int {
	// Right-panel layout: preview height = total height - header(1) - separator(1) - helpbar(1)
	h := m.height - 3
	if h < 1 {
		return 0
	}
	return h
}

// previewScrollStep returns the number of lines to scroll per ctrl+u/d press.
func previewScrollStep(m Model) int {
	h := previewHeight(m)
	if h < 2 {
		return 1
	}
	return h / 2
}

// dashPreviewScrollStep returns the number of lines to scroll per ctrl+u/d press in dashboard.
func dashPreviewScrollStep() int {
	return 3
}

// dashTilePreviewLines returns the number of preview lines for the tile at dashCursor.
func dashTilePreviewLines(m Model) int {
	cols := m.dashCols()
	maxVisible := m.dashMaxVisible()
	pageItems := len(m.filtered) - m.dashboard.pageOffset
	if pageItems > maxVisible {
		pageItems = maxVisible
	}
	if pageItems < 1 {
		pageItems = 1
	}
	rows := (pageItems + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	availableHeight := m.height - 3 - 2 - (rows * 3) // header, help, border+newline per row
	baseCellHeight := availableHeight / rows
	heightRemainder := availableHeight % rows
	if baseCellHeight < 5 {
		baseCellHeight = 5
		heightRemainder = 0
	}
	rowCellHeight := baseCellHeight
	if m.dashboard.cursor/cols < heightRemainder {
		rowCellHeight++
	}
	pl := rowCellHeight - 2
	if pl < 1 {
		return 1
	}
	return pl
}

// dashMaxScrollOffset returns the maximum scroll offset based on cached preview content.
func dashMaxScrollOffset(m Model) int {
	if m.dashboard.previews == nil {
		return 0
	}
	preview := m.dashboard.previews[m.dashboard.cursor]
	lines := strings.Split(preview, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	maxScroll := len(lines) - dashTilePreviewLines(m)
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

// --- View helpers ---

// newView creates a tea.View with AltScreen enabled.
func newView(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

// textInputCursorX returns the display X position of the cursor within a textinput,
// accounting for the prompt width and double-width (CJK) characters.
func textInputCursorX(ti textinput.Model) int {
	val := []rune(ti.Value())
	pos := ti.Position()
	if pos > len(val) {
		pos = len(val)
	}
	promptWidth := lipgloss.Width(ti.Prompt)
	return promptWidth + uniseg.StringWidth(string(val[:pos]))
}

// columnWidthsForWidth returns the name and branch column widths based on the given width.
func columnWidthsForWidth(w int) (int, int) {
	nameWidth := 20
	branchWidth := 15
	if w > 70 {
		nameWidth = 30
	} else if w > 55 {
		nameWidth = 25
	}
	return nameWidth, branchWidth
}

// columnWidths returns the name and branch column widths based on terminal width.
func (m Model) columnWidths() (int, int) {
	return columnWidthsForWidth(m.listPanelWidth())
}

// listPanelWidth returns the width available for the session list panel.
// When preview is enabled and the terminal is wide enough, the list gets the left portion.
func (m Model) listPanelWidth() int {
	if m.preview.enabled && m.width >= 80 {
		// 45% for list, 1 for separator, rest for preview
		w := m.width * 45 / 100
		if w < 50 {
			w = 50
		}
		// Ensure we don't exceed total width minus separator
		if w > m.width-2 {
			w = m.width - 2
		}
		return w
	}
	return m.width
}

// previewPanelWidth returns the width available for the preview panel.
func (m Model) previewPanelWidth() int {
	if !m.preview.enabled || m.width < 80 {
		return 0
	}
	return m.width - m.listPanelWidth() - 1 // -1 for separator
}

// clearConfirm resets all confirm-kill fields and returns to the mode stored in confirmReturnMode.
func (m Model) clearConfirm() Model {
	m.confirm.target = ""
	m.confirm.windowIndex = ""
	m.confirm.paneIndex = ""
	m.confirm.sessionName = ""
	m.mode = m.confirm.returnMode
	m.confirm.returnMode = 0
	return m
}
