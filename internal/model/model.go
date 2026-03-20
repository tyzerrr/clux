package model

import (
	"fmt"
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
	ModeNewSessionPrompt // prompt input for new session
	ModeConfirmKill
	ModeAddExternal
	ModeDashboard
	ModeBroadcastSelect // ghq repo multi-select for broadcast
	ModeBroadcastPrompt // prompt/skill input for broadcast
	ModeBroadcastWait   // waiting for sessions to become idle before sending
)

// Custom message types.
type (
	sessionsMsg         []session.Session
	errMsg              error
	tickMsg             time.Time
	ghqDirsMsg          []string
	windowKilledMsg     struct{}
	sessionUnregistered struct{} // external session unregistered from config
	previewMsg          string   // pane content for preview
	externalWindowsMsg  []tmux.ExternalWindowInfo
	externalAddedMsg    struct{} // session registered successfully
	dashPreviewsMsg     map[int]string
	broadcastGhqDirsMsg        []string          // ghq dirs for broadcast select (separate from newSession)
	newSessionCreatedWithPromptMsg struct { // window created; store pending prompt
		windowIndex string
		prompt      string
	}
	broadcastReadyMsg struct{} // all broadcast targets are idle; send the prompt
	broadcastTargetsUpdatedMsg []broadcastTarget // updated ready flags from poll
	broadcastTimeoutMsg        struct{}          // waiting exceeded 120s; abort broadcast
	ghqRootMsg                 string            // cached ghq root path
)

// broadcastTargetsResolvedMsg is returned after resolving broadcast targets.
// It carries both the resolved window targets and any directory errors encountered.
type broadcastTargetsResolvedMsg struct {
	targets []broadcastTarget
	errors  []string
}

// broadcastTarget holds the resolved target for a broadcast send.
type broadcastTarget struct {
	dir         string
	windowIndex string
	paneIndex   string // tmux pane index within the window
	sessionName string // tmux session name; empty means clux session
	ready       bool   // true once the session reaches Idle status
}

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
	confirmPaneIndex   string // pane index for tmux command
	confirmExternal    bool   // true if confirming unregister (not kill)
	confirmSessionName string // tmux session name for external sessions

	// New session mode
	repoDirs         []string // all dirs from ghq
	filteredDirs     []string // filtered dirs
	newSessionInput  textinput.Model
	newSessionCursor int

	// New session prompt
	newSessionPromptInput textinput.Model
	selectedRepoDir       string // repo selected in ModeNewSession, used in ModeNewSessionPrompt

	// Pending prompt for newly created session
	pendingPrompt         string    // prompt to send when session becomes Idle
	pendingPromptTarget   string    // window index of the target session
	pendingPromptDeadline time.Time // deadline after which pending prompt is abandoned

	// Preview mode
	previewEnabled      bool   // toggle state, default false
	previewContent      string // captured pane content for selected session
	previewScrollOffset int    // lines scrolled up from bottom; 0 = live view

	// Add external session mode
	externalWindows    []tmux.ExternalWindowInfo // all available windows
	filteredExtWindows []tmux.ExternalWindowInfo // filtered by search
	addExtInput        textinput.Model
	addExtCursor       int

	// Status change tracking for bell notification
	prevStatuses map[string]session.Status

	// Dashboard mode
	dashCursor     int            // index into m.filtered for focused cell
	dashPreviews   map[int]string // windowIndex -> pane content for each session
	dashPageOffset int            // index of first displayed item in dashboard
	dashFocused    bool           // true when in single-session focus mode

	// Grouping mode
	groupEnabled bool   // toggle for grouped display
	ghqRoot      string // cached result of `ghq root`

	// Broadcast mode
	broadcastDirs        []string           // all ghq dirs (loaded once)
	broadcastFiltered    []string           // filtered by broadcastInput
	broadcastSelected    map[string]bool    // selected dir paths (persists across filter changes)
	broadcastInput       textinput.Model    // filter input for repo select
	broadcastCursor      int                // cursor in broadcastFiltered
	broadcastPromptInput textinput.Model    // prompt text input
	broadcastTargets     []broadcastTarget  // resolved targets after selection
	broadcastPrompt      string             // the prompt being sent (saved when entering wait)
	broadcastStartTime   time.Time          // when ModeBroadcastWait was entered
	broadcastErrors      []string           // dir validation errors collected during resolution
}

// New creates and returns an initialized Model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.CharLimit = 64

	ni := textinput.New()
	ni.Placeholder = "Search repository..."
	ni.CharLimit = 64

	npi := textinput.New()
	npi.Placeholder = "Enter prompt (optional, Enter to skip)..."
	npi.CharLimit = 512

	ai := textinput.New()
	ai.Placeholder = "Search session:window..."
	ai.CharLimit = 64

	bci := textinput.New()
	bci.Placeholder = "Search repository..."
	bci.CharLimit = 64

	bpi := textinput.New()
	bpi.Placeholder = "Prompt or skill to broadcast..."
	bpi.CharLimit = 512

	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.Config{}
	}

	return Model{
		mode:                 ModeList,
		filterInput:          ti,
		newSessionInput:      ni,
		newSessionPromptInput: npi,
		addExtInput:          ai,
		broadcastInput:       bci,
		broadcastPromptInput: bpi,
		cfg:                  cfg,
		previewEnabled:       cfg.PreviewDefault,
		groupEnabled:         cfg.GroupDefault,
		prevStatuses:         make(map[string]session.Status),
		dashPreviews:         make(map[int]string),
		broadcastSelected:    make(map[string]bool),
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
func (m Model) GroupEnabled() bool            { return m.groupEnabled }

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

func doImmediateTick() tea.Cmd {
	return func() tea.Msg {
		return tickMsg(time.Now())
	}
}

func fetchPreviewCmdForSession(s session.Session) tea.Cmd {
	return func() tea.Msg {
		sessionName := tmux.SessionName
		if s.External && s.SessionName != "" {
			sessionName = s.SessionName
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
		sessionName := tmux.SessionName
		if s.External && s.SessionName != "" {
			sessionName = s.SessionName
		}
		paneIndex := resolvePaneIndex(s.PaneIndex)
		content, err := tmux.CapturePaneForSessionWithOffset(sessionName, s.WindowIndex, paneIndex, scrollOffset, height)
		if err != nil {
			return previewMsg("")
		}
		return previewMsg(content)
	}
}

func fetchDashboardPreviews(sessions []session.Session) tea.Cmd {
	return func() tea.Msg {
		result := make(map[int]string)
		max := len(sessions)
		for i := 0; i < max; i++ {
			s := sessions[i]
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			paneIndex := resolvePaneIndex(s.PaneIndex)
			content, err := tmux.CapturePaneForSession(sessionName, s.WindowIndex, paneIndex)
			if err != nil {
				result[i] = ""
			} else {
				result[i] = content
			}
		}
		return dashPreviewsMsg(result)
	}
}

func listGhqDirs() ([]string, error) {
	out, err := exec.Command("ghq", "list", "-p").Output()
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

// sortByStatus sorts sessions so that those needing attention appear first.
// Priority order: Waiting (3) > Working (2) > Idle (1) > Unknown (0).
func sortByStatus(sessions []session.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return statusPriority(sessions[i].Status) > statusPriority(sessions[j].Status)
	})
}

func statusSummary(sessions []session.Session) string {
	counts := make(map[session.Status]int)
	for _, s := range sessions {
		counts[s.Status]++
	}
	var parts []string
	if n := counts[session.StatusWaiting]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d Waiting", n))
	}
	if n := counts[session.StatusWorking]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d Working", n))
	}
	if n := counts[session.StatusIdle]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d Idle", n))
	}
	if n := counts[session.StatusUnknown]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d Unknown", n))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
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
	out, err := exec.Command("ghq", "root").Output()
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
	if s.External {
		return "External"
	}
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
		fetchSessionsCmdWithExternals(m.cfg),
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
		sortByStatus(m.filtered)
		// Clamp cursor.
		if len(m.filtered) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}

		if m.mode == ModeDashboard {
			maxVisible := m.dashMaxVisible()
			// Snap page offset to a valid page boundary.
			if maxVisible > 0 {
				m.dashPageOffset = (m.dashPageOffset / maxVisible) * maxVisible
			}
			if m.dashPageOffset >= len(m.filtered) {
				m.dashPageOffset = 0
			}
			pageCount := len(m.filtered) - m.dashPageOffset
			if pageCount > maxVisible {
				pageCount = maxVisible
			}
			if pageCount < 0 {
				pageCount = 0
			}
			if m.dashCursor >= pageCount {
				m.dashCursor = 0
			}
		}

		var cmds []tea.Cmd
		if shouldBell {
			cmds = append(cmds, ringBell())
		}
		if m.previewEnabled && len(m.filtered) > 0 {
			if m.previewScrollOffset > 0 {
				cmds = append(cmds, fetchPreviewCmdForSessionWithOffset(m.filtered[m.cursor], m.previewScrollOffset, previewHeight(m)))
			} else {
				cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.cursor]))
			}
		}

		// Check if there's a pending prompt to send to a newly created session.
		if m.pendingPrompt != "" && m.pendingPromptTarget != "" {
			// Check if the deadline has passed.
			if !m.pendingPromptDeadline.IsZero() && time.Now().After(m.pendingPromptDeadline) {
				m = m.clearPendingPrompt()
				m.err = fmt.Errorf("pending prompt timed out: session did not become idle within 120s")
			} else {
				targetFound := false
				for _, s := range m.sessions {
					if s.WindowIndex == m.pendingPromptTarget {
						targetFound = true
						if s.Status == session.StatusIdle {
							prompt := m.pendingPrompt
							target := m.pendingPromptTarget
							m = m.clearPendingPrompt()
							cmds = append(cmds, func() tea.Msg {
								_ = tmux.SendKeysLiteral(tmux.SessionName, target, "0", prompt)
								return fetchSessionsCmdWithExternals(m.cfg)()
							})
							break
						}
					}
				}
				// If target window was not found in sessions, it was killed.
				if !targetFound {
					m = m.clearPendingPrompt()
					m.err = fmt.Errorf("pending prompt cancelled: target window no longer exists")
				}
			}
		}

		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case previewMsg:
		m.previewContent = string(msg)
		return m, nil

	case dashPreviewsMsg:
		m.dashPreviews = map[int]string(msg)
		return m, nil

	case errMsg:
		m.err = error(msg)
		return m, nil

	case tickMsg:
		if m.mode == ModeList || m.mode == ModeFilter {
			cmds := []tea.Cmd{fetchSessionsCmdWithExternals(m.cfg), doTick()}
			if m.previewEnabled && len(m.filtered) > 0 {
				if m.previewScrollOffset > 0 {
					cmds = append(cmds, fetchPreviewCmdForSessionWithOffset(m.filtered[m.cursor], m.previewScrollOffset, previewHeight(m)))
				} else {
					cmds = append(cmds, fetchPreviewCmdForSession(m.filtered[m.cursor]))
				}
			}
			return m, tea.Batch(cmds...)
		} else if m.mode == ModeDashboard {
			pageItems := len(m.filtered) - m.dashPageOffset
			maxVisible := m.dashMaxVisible()
			if pageItems > maxVisible {
				pageItems = maxVisible
			}
			if pageItems < 0 {
				pageItems = 0
			}
			pageSessions := m.filtered[m.dashPageOffset : m.dashPageOffset+pageItems]
			cmds := []tea.Cmd{fetchSessionsCmdWithExternals(m.cfg), doTick(), fetchDashboardPreviews(pageSessions)}
			return m, tea.Batch(cmds...)
		} else if m.mode == ModeBroadcastWait {
			return m, tea.Batch(doTick(), m.checkBroadcastTargetsCmd())
		}
		// If there's a pending prompt, keep polling sessions even in other modes.
		if m.pendingPrompt != "" && m.pendingPromptTarget != "" {
			return m, tea.Batch(doTick(), fetchSessionsCmdWithExternals(m.cfg))
		}
		return m, doTick()

	case newSessionCreatedWithPromptMsg:
		m.pendingPrompt = msg.prompt
		m.pendingPromptTarget = msg.windowIndex
		m.pendingPromptDeadline = time.Now().Add(120 * time.Second)
		return m, fetchSessionsCmdWithExternals(m.cfg)

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

	case broadcastGhqDirsMsg:
		m.broadcastDirs = []string(msg)
		m.broadcastFiltered = filterDirs(m.broadcastDirs, m.broadcastInput.Value())
		m.broadcastCursor = 0
		return m, nil

	case broadcastReadyMsg:
		// All targets are idle; send the prompt to each.
		targets := m.broadcastTargets
		prompt := m.broadcastPrompt
		m.broadcastTargets = nil
		m.broadcastPrompt = ""
		m.mode = ModeList
		cfg := m.cfg
		return m, func() tea.Msg {
			for _, t := range targets {
				sn := tmux.SessionName
				if t.sessionName != "" {
					sn = t.sessionName
				}
				_ = tmux.SendKeysLiteral(sn, t.windowIndex, resolvePaneIndex(t.paneIndex), prompt)
			}
			return fetchSessionsCmdWithExternals(cfg)()
		}

	case broadcastTargetsResolvedMsg:
		m.broadcastTargets = msg.targets
		m.broadcastErrors = msg.errors
		if len(m.broadcastTargets) == 0 {
			m.mode = ModeList
			return m, nil
		}
		m.mode = ModeBroadcastWait
		m.broadcastStartTime = time.Now()
		return m, tea.Batch(doTick(), m.checkBroadcastTargetsCmd())

	case broadcastTargetsUpdatedMsg:
		m.broadcastTargets = []broadcastTarget(msg)
		return m, nil

	case broadcastTimeoutMsg:
		m.broadcastTargets = nil
		m.broadcastPrompt = ""
		m.broadcastErrors = nil
		m.mode = ModeList
		m.err = fmt.Errorf("Broadcast timed out: some sessions did not become idle")
		return m, nil

	case ghqRootMsg:
		m.ghqRoot = string(msg)
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
		case ModeNewSessionPrompt:
			return m.updateNewSessionPrompt(msg)
		case ModeConfirmKill:
			return m.updateConfirmKill(msg)
		case ModeAddExternal:
			return m.updateAddExternal(msg)
		case ModeDashboard:
			return m.updateDashboard(msg)
		case ModeBroadcastSelect:
			return m.updateBroadcastSelect(msg)
		case ModeBroadcastPrompt:
			return m.updateBroadcastPrompt(msg)
		case ModeBroadcastWait:
			return m.updateBroadcastWait(msg)
		}

	case tea.PasteMsg:
		switch m.mode {
		case ModeFilter:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filtered = applyFilter(m.sessions, m.filterInput.Value())
			sortByStatus(m.filtered)
			if m.cursor >= len(m.filtered) {
				m.cursor = 0
			}
			return m, cmd

		case ModeNewSession:
			var cmd tea.Cmd
			m.newSessionInput, cmd = m.newSessionInput.Update(msg)
			m.filteredDirs = filterDirs(m.repoDirs, m.newSessionInput.Value())
			if m.newSessionCursor >= len(m.filteredDirs) {
				m.newSessionCursor = 0
			}
			return m, cmd

		case ModeNewSessionPrompt:
			var cmd tea.Cmd
			m.newSessionPromptInput, cmd = m.newSessionPromptInput.Update(msg)
			return m, cmd

		case ModeAddExternal:
			var cmd tea.Cmd
			m.addExtInput, cmd = m.addExtInput.Update(msg)
			allFiltered := filterExtWindows(m.externalWindows, m.addExtInput.Value())
			m.filteredExtWindows = excludeRegistered(allFiltered, m.cfg)
			if m.addExtCursor >= len(m.filteredExtWindows) {
				m.addExtCursor = 0
			}
			return m, cmd

		case ModeBroadcastSelect:
			var cmd tea.Cmd
			m.broadcastInput, cmd = m.broadcastInput.Update(msg)
			m.broadcastFiltered = filterDirs(m.broadcastDirs, m.broadcastInput.Value())
			if m.broadcastCursor >= len(m.broadcastFiltered) {
				m.broadcastCursor = 0
			}
			return m, cmd

		case ModeBroadcastPrompt:
			var cmd tea.Cmd
			m.broadcastPromptInput, cmd = m.broadcastPromptInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "k", "up", "ctrl+p":
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor - 1 + len(m.filtered)) % len(m.filtered)
			m.previewScrollOffset = 0
			if m.previewEnabled {
				return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
			}
		}

	case "j", "down", "ctrl+n":
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor + 1) % len(m.filtered)
			m.previewScrollOffset = 0
			if m.previewEnabled {
				return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
			}
		}

	case "ctrl+u":
		if m.previewEnabled && len(m.filtered) > 0 && previewHeight(m) > 0 {
			m.previewScrollOffset += previewScrollStep(m)
			return m, fetchPreviewCmdForSessionWithOffset(m.filtered[m.cursor], m.previewScrollOffset, previewHeight(m))
		}

	case "ctrl+d":
		if m.previewEnabled && len(m.filtered) > 0 && previewHeight(m) > 0 {
			m.previewScrollOffset -= previewScrollStep(m)
			if m.previewScrollOffset < 0 {
				m.previewScrollOffset = 0
			}
			return m, fetchPreviewCmdForSessionWithOffset(m.filtered[m.cursor], m.previewScrollOffset, previewHeight(m))
		}

	case "enter":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			windowIndex := s.WindowIndex
			paneIndex := resolvePaneIndex(s.PaneIndex)
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, windowIndex, paneIndex); err != nil {
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
			m.confirmPaneIndex = resolvePaneIndex(s.PaneIndex)
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
			paneIndex := resolvePaneIndex(s.PaneIndex)
			return m, func() tea.Msg {
				if err := tmux.SendKeys(sessionName, windowIndex, paneIndex, "y"); err != nil {
					return errMsg(err)
				}
				return fetchSessionsCmdWithExternals(m.cfg)()
			}
		}

	case "R":
		tmux.ClearAllPaneCache()
		return m, fetchSessionsCmdWithExternals(m.cfg)

	case "p":
		m.previewEnabled = !m.previewEnabled
		if m.previewEnabled && len(m.filtered) > 0 {
			return m, fetchPreviewCmdForSession(m.filtered[m.cursor])
		}
		if !m.previewEnabled {
			m.previewContent = ""
			m.previewScrollOffset = 0
		}

	case "d":
		m.mode = ModeDashboard
		m.dashCursor = 0
		m.dashPageOffset = 0
		m.dashFocused = false
		m.previewScrollOffset = 0
		m.dashPreviews = make(map[int]string)
		maxVisible := m.dashMaxVisible()
		pageItems := len(m.filtered)
		if pageItems > maxVisible {
			pageItems = maxVisible
		}
		return m, fetchDashboardPreviews(m.filtered[:pageItems])

	case "b":
		m.mode = ModeBroadcastSelect
		m.err = nil
		m.broadcastInput.SetValue("")
		m.broadcastCursor = 0
		m.broadcastSelected = make(map[string]bool)
		cmds := []tea.Cmd{m.broadcastInput.Focus()}
		if len(m.broadcastDirs) == 0 {
			cmds = append(cmds, fetchBroadcastGhqDirs)
		} else {
			m.broadcastFiltered = m.broadcastDirs
		}
		return m, tea.Batch(cmds...)

	case "g":
		m.groupEnabled = !m.groupEnabled

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
		m = m.clearConfirm()
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
		m = m.clearConfirm()
	}
	return m, nil
}

// clearConfirm resets all confirm-kill fields and returns to ModeList.
func (m Model) clearConfirm() Model {
	m.confirmTarget = ""
	m.confirmWindowIndex = ""
	m.confirmPaneIndex = ""
	m.confirmExternal = false
	m.confirmSessionName = ""
	m.mode = ModeList
	return m
}

// clearPendingPrompt resets all pending-prompt fields.
func (m Model) clearPendingPrompt() Model {
	m.pendingPrompt = ""
	m.pendingPromptTarget = ""
	m.pendingPromptDeadline = time.Time{}
	return m
}

func (m Model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Confirm filter and return to list mode (filter preserved).
		m.filterInput.Blur()
		m.mode = ModeList
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
			m.selectedRepoDir = dir
			m.mode = ModeNewSessionPrompt
			m.newSessionPromptInput.SetValue("")
			m.newSessionInput.Blur()
			return m, m.newSessionPromptInput.Focus()
		}
		return m, nil

	case "esc":
		m.mode = ModeList
		m.newSessionInput.Blur()
		return m, nil

	case "up", "ctrl+k", "ctrl+p":
		if len(m.filteredDirs) > 0 {
			m.newSessionCursor = (m.newSessionCursor - 1 + len(m.filteredDirs)) % len(m.filteredDirs)
		}
		return m, nil

	case "down", "ctrl+j", "ctrl+n":
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

func (m Model) updateNewSessionPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := strings.TrimSpace(m.newSessionPromptInput.Value())
		dir := m.selectedRepoDir
		m.newSessionPromptInput.Blur()
		m.mode = ModeList

		if prompt == "" {
			// No prompt — just create the window
			name := tmux.GenerateWindowName(dir)
			return m, func() tea.Msg {
				if err := tmux.CreateWindow(name, dir); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}

		// Create window silently and store pending prompt
		name := tmux.GenerateWindowName(dir)
		return m, func() tea.Msg {
			newIdx, err := tmux.CreateWindowSilent(name, dir)
			if err != nil {
				return errMsg(err)
			}
			return newSessionCreatedWithPromptMsg{windowIndex: newIdx, prompt: prompt}
		}

	case "esc":
		m.mode = ModeList
		m.newSessionPromptInput.Blur()
		return m, nil

	default:
		var cmd tea.Cmd
		m.newSessionPromptInput, cmd = m.newSessionPromptInput.Update(msg)
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

	case "up", "ctrl+k", "ctrl+p":
		if len(m.filteredExtWindows) > 0 {
			m.addExtCursor = (m.addExtCursor - 1 + len(m.filteredExtWindows)) % len(m.filteredExtWindows)
		}
		return m, nil

	case "down", "ctrl+j", "ctrl+n":
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

func (m Model) updateDashboard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cols := m.dashCols()
	maxVisible := m.dashMaxVisible()
	pageItems := len(m.filtered) - m.dashPageOffset
	if pageItems > maxVisible {
		pageItems = maxVisible
	}
	if pageItems < 0 {
		pageItems = 0
	}

	switch msg.String() {
	case "esc", "d":
		if m.dashFocused {
			m.dashFocused = false
			return m, nil
		}
		m.mode = ModeList
		return m, nil
	case "q":
		return m, tea.Quit
	case "f":
		if pageItems > 0 {
			m.dashFocused = !m.dashFocused
		}
	case "]":
		// Next page
		if m.dashFocused {
			m.dashFocused = false
		}
		nextOffset := m.dashPageOffset + maxVisible
		if nextOffset < len(m.filtered) {
			m.dashPageOffset = nextOffset
			m.dashCursor = 0
			pageSessions := m.filtered[m.dashPageOffset : m.dashPageOffset+min(m.dashMaxVisible(), len(m.filtered)-m.dashPageOffset)]
			return m, fetchDashboardPreviews(pageSessions)
		}
	case "[":
		// Previous page
		if m.dashFocused {
			m.dashFocused = false
		}
		prevOffset := m.dashPageOffset - maxVisible
		if prevOffset < 0 {
			prevOffset = 0
		}
		if prevOffset != m.dashPageOffset {
			m.dashPageOffset = prevOffset
			m.dashCursor = 0
			pageSessions := m.filtered[m.dashPageOffset : m.dashPageOffset+min(m.dashMaxVisible(), len(m.filtered)-m.dashPageOffset)]
			return m, fetchDashboardPreviews(pageSessions)
		}
	case "h", "left":
		if m.dashFocused {
			m.dashFocused = false
		}
		if m.dashCursor%cols > 0 {
			m.dashCursor--
		}
	case "l", "right":
		if m.dashFocused {
			m.dashFocused = false
		}
		if m.dashCursor%cols < cols-1 && m.dashCursor+1 < pageItems {
			m.dashCursor++
		}
	case "k", "up", "ctrl+p":
		if m.dashFocused {
			m.dashFocused = false
		}
		if m.dashCursor-cols >= 0 {
			m.dashCursor -= cols
		} else if m.dashPageOffset > 0 {
			// Go to previous page
			prevOffset := m.dashPageOffset - maxVisible
			if prevOffset < 0 {
				prevOffset = 0
			}
			m.dashPageOffset = prevOffset
			m.dashCursor = 0
			pageSessions := m.filtered[m.dashPageOffset : m.dashPageOffset+min(m.dashMaxVisible(), len(m.filtered)-m.dashPageOffset)]
			return m, fetchDashboardPreviews(pageSessions)
		}
	case "j", "down", "ctrl+n":
		if m.dashFocused {
			m.dashFocused = false
		}
		if m.dashCursor+cols < pageItems {
			m.dashCursor += cols
		} else if m.dashPageOffset+maxVisible < len(m.filtered) {
			// Go to next page
			m.dashPageOffset += maxVisible
			m.dashCursor = 0
			pageSessions := m.filtered[m.dashPageOffset : m.dashPageOffset+min(m.dashMaxVisible(), len(m.filtered)-m.dashPageOffset)]
			return m, fetchDashboardPreviews(pageSessions)
		}
	case "enter":
		if pageItems > 0 && m.dashPageOffset+m.dashCursor < len(m.filtered) {
			s := m.filtered[m.dashPageOffset+m.dashCursor]
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			paneIndex := resolvePaneIndex(s.PaneIndex)
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, s.WindowIndex, paneIndex); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}
	case "y":
		if pageItems > 0 && m.dashPageOffset+m.dashCursor < len(m.filtered) {
			s := m.filtered[m.dashPageOffset+m.dashCursor]
			if s.Status != session.StatusWaiting {
				break
			}
			sessionName := tmux.SessionName
			if s.External && s.SessionName != "" {
				sessionName = s.SessionName
			}
			windowIndex := s.WindowIndex
			paneIndex := resolvePaneIndex(s.PaneIndex)
			return m, func() tea.Msg {
				if err := tmux.SendKeys(sessionName, windowIndex, paneIndex, "y"); err != nil {
					return errMsg(err)
				}
				return fetchSessionsCmdWithExternals(m.cfg)()
			}
		}

	case "K":
		if len(m.filtered) > 0 && m.dashPageOffset+m.dashCursor < len(m.filtered) {
			s := m.filtered[m.dashPageOffset+m.dashCursor]
			m.confirmTarget = s.DisplayName()
			m.confirmWindowIndex = s.WindowIndex
			m.confirmPaneIndex = resolvePaneIndex(s.PaneIndex)
			m.confirmExternal = s.External
			m.confirmSessionName = s.SessionName
			m.err = nil
			m.mode = ModeConfirmKill
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

	case "/":
		m.mode = ModeFilter
		return m, m.filterInput.Focus()

	case "b":
		m.mode = ModeBroadcastSelect
		m.err = nil
		m.broadcastInput.SetValue("")
		m.broadcastCursor = 0
		m.broadcastSelected = make(map[string]bool)
		cmds := []tea.Cmd{m.broadcastInput.Focus()}
		if len(m.broadcastDirs) == 0 {
			cmds = append(cmds, fetchBroadcastGhqDirs)
		} else {
			m.broadcastFiltered = m.broadcastDirs
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updateBroadcastSelect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Collect selected dirs; if nothing selected, treat cursor item as selected.
		var selectedDirs []string
		for dir, sel := range m.broadcastSelected {
			if sel {
				selectedDirs = append(selectedDirs, dir)
			}
		}
		if len(selectedDirs) == 0 && len(m.broadcastFiltered) > 0 {
			selectedDirs = append(selectedDirs, m.broadcastFiltered[m.broadcastCursor])
		}
		if len(selectedDirs) == 0 {
			return m, nil
		}
		uniqueDirs := selectedDirs // already unique since map keys are unique
		// Store as a temporary field; transition to prompt.
		// We reuse broadcastTargets with dir only (windowIndex/sessionName resolved later).
		m.broadcastTargets = make([]broadcastTarget, len(uniqueDirs))
		for i, d := range uniqueDirs {
			m.broadcastTargets[i] = broadcastTarget{dir: d}
		}
		m.broadcastInput.Blur()
		m.mode = ModeBroadcastPrompt
		m.broadcastPromptInput.SetValue("")
		return m, m.broadcastPromptInput.Focus()

	case "esc":
		m.mode = ModeList
		m.broadcastInput.Blur()
		m.broadcastSelected = make(map[string]bool)
		return m, nil

	case " ", "space":
		// Toggle selection of item at cursor.
		if len(m.broadcastFiltered) > 0 {
			dir := m.broadcastFiltered[m.broadcastCursor]
			m.broadcastSelected[dir] = !m.broadcastSelected[dir]
		}
		return m, nil

	case "up", "ctrl+k", "ctrl+p":
		if len(m.broadcastFiltered) > 0 {
			m.broadcastCursor = (m.broadcastCursor - 1 + len(m.broadcastFiltered)) % len(m.broadcastFiltered)
		}
		return m, nil

	case "down", "ctrl+j", "ctrl+n":
		if len(m.broadcastFiltered) > 0 {
			m.broadcastCursor = (m.broadcastCursor + 1) % len(m.broadcastFiltered)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.broadcastInput, cmd = m.broadcastInput.Update(msg)
		m.broadcastFiltered = filterDirs(m.broadcastDirs, m.broadcastInput.Value())
		if m.broadcastCursor >= len(m.broadcastFiltered) {
			m.broadcastCursor = 0
		}
		return m, cmd
	}
}

func (m Model) updateBroadcastPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := strings.TrimSpace(m.broadcastPromptInput.Value())
		if prompt == "" {
			return m, nil
		}
		m.broadcastPromptInput.Blur()
		m.broadcastPrompt = prompt

		// Always create new windows for each target dir.
		targets := m.broadcastTargets
		return m, func() tea.Msg {
			resolved := make([]broadcastTarget, 0, len(targets))
			var dirErrors []string
			for _, t := range targets {
				// Validate the directory before attempting to create a window.
				if err := tmux.ValidateDir(t.dir); err != nil {
					dirErrors = append(dirErrors, fmt.Sprintf("%s: %v", t.dir, err))
					continue
				}
				// Always create a new window for this dir without switching the client.
				name := tmux.GenerateWindowName(t.dir)
				newIdx, err := tmux.CreateWindowSilent(name, t.dir)
				if err != nil {
					dirErrors = append(dirErrors, fmt.Sprintf("%s: %v", t.dir, err))
					continue
				}
				resolved = append(resolved, broadcastTarget{
					dir:         t.dir,
					windowIndex: newIdx,
					paneIndex:   "0",
					sessionName: "",
					ready:       false,
				})
			}
			return broadcastTargetsResolvedMsg{targets: resolved, errors: dirErrors}
		}

	case "esc":
		m.broadcastPromptInput.Blur()
		m.mode = ModeBroadcastSelect
		return m, m.broadcastInput.Focus()

	default:
		var cmd tea.Cmd
		m.broadcastPromptInput, cmd = m.broadcastPromptInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateBroadcastWait(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel broadcast.
		m.broadcastTargets = nil
		m.broadcastPrompt = ""
		m.mode = ModeList
		return m, nil
	}
	return m, nil
}

// checkBroadcastTargetsCmd returns a command that checks whether all broadcast targets
// are idle. If all are ready, it returns broadcastReadyMsg; otherwise it updates the
// ready flags and returns broadcastTargetsUpdatedMsg. Returns broadcastTimeoutMsg if
// the wait has exceeded 120 seconds.
func (m Model) checkBroadcastTargetsCmd() tea.Cmd {
	targets := m.broadcastTargets
	startTime := m.broadcastStartTime
	return func() tea.Msg {
		// Timeout: if waiting more than 120 seconds, abort.
		if time.Since(startTime) > 120*time.Second {
			return broadcastTimeoutMsg{}
		}

		graceExceeded := time.Since(startTime) >= 30*time.Second
		allReady := true
		updated := make([]broadcastTarget, len(targets))
		for i, t := range targets {
			if graceExceeded {
				// After 30s, force-ready all targets regardless of status.
				// Claude Code should have started by then; if not, SendKeys is a no-op.
				t.ready = true
			} else {
				paneIdx := resolvePaneIndex(t.paneIndex)
				sn := t.sessionName
				if sn == "" {
					sn = tmux.SessionName
				}
				st, isClaudeCode := tmux.GetWindowStatus(sn, t.windowIndex, paneIdx)
				if isClaudeCode {
					t.ready = st == session.StatusIdle
				} else {
					t.ready = false
				}
			}
			updated[i] = t
			if !t.ready {
				allReady = false
			}
		}
		if allReady && len(updated) > 0 {
			return broadcastReadyMsg{}
		}
		// Return updated targets as a special message.
		return broadcastTargetsUpdatedMsg(updated)
	}
}

// resolvePaneIndex returns p if non-empty, otherwise "0".
func resolvePaneIndex(p string) string {
	if p == "" {
		return "0"
	}
	return p
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
	headerLines := 3
	helpLines := 2
	minCellHeight := 7 // minimum usable cell height (header + separator + some content)
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
func previewHeight(m Model) int {
	sessionRows := len(m.filtered)
	if m.groupEnabled && sessionRows > 0 {
		groups := buildGroups(m.filtered, m.ghqRoot)
		sessionRows = groupedSessionRows(groups)
	}
	topLines := 2 + 1 + sessionRows + 1
	if m.err != nil {
		topLines += 2
	}
	maxPreviewHeight := m.height - topLines - 2
	if maxPreviewHeight < 1 {
		return 0
	}
	h := maxPreviewHeight / 2
	if h < 5 {
		h = 5
	}
	if h > maxPreviewHeight {
		h = maxPreviewHeight
	}
	return h
}

// previewScrollStep returns the number of lines to scroll per Ctrl+U/D press.
func previewScrollStep(m Model) int {
	h := previewHeight(m)
	if h < 2 {
		return 1
	}
	return h / 2
}

// --- View ---

var (
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleHelpBar  = lipgloss.NewStyle().Faint(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleDir      = lipgloss.NewStyle().Faint(true)
	stylePreview     = lipgloss.NewStyle().Faint(true)
	styleGroupHeader = lipgloss.NewStyle().Faint(true).Bold(true)

	styleWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	styleWaiting = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	styleIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	styleUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta

	styleOverlayBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("6")). // cyan
				Padding(1, 2)
	styleOverlayTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleDimmed       = lipgloss.NewStyle().Faint(true)
)

func shortenDir(dir string) string {
	parts := strings.Split(dir, "/")
	if len(parts) <= 2 {
		return dir
	}
	return strings.Join(parts[len(parts)-2:], "/")
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

// placeOverlay composites the foreground string on top of the background string,
// centered horizontally and vertically. Both strings may contain ANSI sequences.
func placeOverlay(bg, fg string, bgWidth, bgHeight int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for len(bgLines) < bgHeight {
		bgLines = append(bgLines, "")
	}

	fgWidth := 0
	for _, line := range fgLines {
		if w := lipgloss.Width(line); w > fgWidth {
			fgWidth = w
		}
	}

	startX := (bgWidth - fgWidth) / 2
	if startX < 0 {
		startX = 0
	}
	startY := (len(bgLines) - len(fgLines)) / 2
	if startY < 0 {
		startY = 0
	}

	leftPad := strings.Repeat(" ", startX)
	for i, fgLine := range fgLines {
		bgIdx := startY + i
		if bgIdx >= len(bgLines) {
			break
		}
		pad := fgWidth - lipgloss.Width(fgLine)
		if pad < 0 {
			pad = 0
		}
		padded := fgLine + strings.Repeat(" ", pad)
		bgLines[bgIdx] = leftPad + padded
	}

	return strings.Join(bgLines, "\n")
}

const overlayBorderPadding = 6 // Border(2) + Padding(1,2)*2 = 6

func (m Model) overlayWidth(pct, minW, maxW int) int {
	return min(max(m.width*pct/100, minW), maxW)
}

func (m Model) overlayHeight(pct, minH, maxH int) int {
	return min(max(m.height*pct/100, minH), maxH)
}

// overlayListDims returns the standard overlay dimensions for list-style modals.
func (m Model) overlayListDims() (int, int) {
	return m.overlayWidth(60, 60, 100), m.overlayHeight(60, 10, 25)
}

// overlayListHeight returns the number of visible list items for a given overlay height.
func overlayListHeight(overlayHeight int) int {
	return min(max(overlayHeight-10, 3), overlayHeight)
}

func renderOverlayBox(content string, overlayWidth int) string {
	innerWidth := overlayWidth - overlayBorderPadding
	if innerWidth < 20 {
		innerWidth = 20
	}
	return styleOverlayBorder.Width(innerWidth).Render(content)
}

func (m Model) viewWithOverlay(viewFn func(*strings.Builder) string) tea.View {
	var b strings.Builder
	overlay := viewFn(&b)
	if m.width > 0 && m.height > 0 {
		base := m.viewListBase()
		return newView(placeOverlay(base, overlay, m.width, m.height))
	}
	return newView(overlay)
}

// columnWidths returns the name and branch column widths based on terminal width.
func (m Model) columnWidths() (int, int) {
	nameWidth := 20
	branchWidth := 15
	if m.width > 100 {
		nameWidth = 30
	} else if m.width > 80 {
		nameWidth = 25
	}
	return nameWidth, branchWidth
}

// renderSessionRows writes session rows (grouped or flat) to the builder.
// groups must be non-nil when m.groupEnabled is true.
func (m Model) renderSessionRows(b *strings.Builder, groups []sessionGroup, nameWidth, branchWidth int) {
	b.WriteString(styleHelpBar.Render(fmt.Sprintf(" %-16s %-*s  %-*s %s", "Status", nameWidth, "Name", branchWidth, "Branch", "Dir")))
	b.WriteString("\n")
	if m.groupEnabled {
		for _, g := range groups {
			groupHeader := fmt.Sprintf("── %s (%d) ", g.name, len(g.sessions))
			remaining := m.width - len([]rune(groupHeader))
			if remaining > 0 {
				groupHeader += strings.Repeat("─", remaining)
			}
			b.WriteString(styleGroupHeader.Render(groupHeader))
			b.WriteString("\n")
			for _, is := range g.sessions {
				s := is.session
				statusText := statusStyle(s.Status).Render(fmt.Sprintf("%s %-7s", s.Status.Icon(), s.Status.String()))
				displayName := s.DisplayName()
				if runes := []rune(displayName); len(runes) > nameWidth {
					displayName = string(runes[:nameWidth-1]) + "…"
				}
				branch := s.Branch
				if runes := []rune(branch); len(runes) > branchWidth {
					branch = string(runes[:branchWidth-1]) + "…"
				}
				dir := styleDir.Render(shortenDir(s.Dir))
				row := fmt.Sprintf("   %s  %-*s  %-*s %s", statusText, nameWidth, displayName, branchWidth, branch, dir)
				if is.index == m.cursor {
					row = styleSelected.Render(row)
				}
				b.WriteString(row)
				b.WriteString("\n")
			}
		}
	} else {
		for i, s := range m.filtered {
			statusText := statusStyle(s.Status).Render(fmt.Sprintf("%s %-7s", s.Status.Icon(), s.Status.String()))
			displayName := s.DisplayName()
			if runes := []rune(displayName); len(runes) > nameWidth {
				displayName = string(runes[:nameWidth-1]) + "…"
			}
			branch := s.Branch
			if runes := []rune(branch); len(runes) > branchWidth {
				branch = string(runes[:branchWidth-1]) + "…"
			}
			dir := styleDir.Render(shortenDir(s.Dir))
			row := fmt.Sprintf(" %s  %-*s  %-*s %s", statusText, nameWidth, displayName, branchWidth, branch, dir)
			if i == m.cursor {
				row = styleSelected.Render(row)
			}
			b.WriteString(row)
			b.WriteString("\n")
		}
	}
}

// viewListBase renders the session list view dimmed, used as overlay background.
func (m Model) viewListBase() string {
	var b strings.Builder

	header := fmt.Sprintf("Clux — Sessions (%d)", len(m.filtered))
	if summary := statusSummary(m.filtered); summary != "" {
		header += " — " + summary
	}
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n\n")

	nameWidth, branchWidth := m.columnWidths()

	if len(m.filtered) == 0 {
		b.WriteString("No Claude Code sessions found.\n")
	} else {
		var groups []sessionGroup
		if m.groupEnabled {
			groups = buildGroups(m.filtered, m.ghqRoot)
		}
		m.renderSessionRows(&b, groups, nameWidth, branchWidth)
	}

	return styleDimmed.Render(b.String())
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
		return m.viewWithOverlay(m.viewNewSession)
	}

	if m.mode == ModeNewSessionPrompt {
		return m.viewWithOverlay(m.viewNewSessionPrompt)
	}

	if m.mode == ModeAddExternal {
		return m.viewWithOverlay(m.viewAddExternal)
	}

	if m.mode == ModeDashboard {
		return newView(m.viewDashboard(&b))
	}

	if m.mode == ModeBroadcastSelect {
		return m.viewWithOverlay(m.viewBroadcastSelect)
	}

	if m.mode == ModeBroadcastPrompt {
		return m.viewWithOverlay(m.viewBroadcastPrompt)
	}

	if m.mode == ModeBroadcastWait {
		return newView(m.viewBroadcastWait(&b))
	}

	// Header.
	header := fmt.Sprintf("Clux — Sessions (%d)", len(m.filtered))
	if summary := statusSummary(m.filtered); summary != "" {
		header += " — " + summary
	}
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n\n")

	// Error display.
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	nameWidth, branchWidth := m.columnWidths()

	// Session list.
	var groups []sessionGroup
	if len(m.filtered) == 0 {
		b.WriteString("No Claude Code sessions found. Start Claude Code in another tmux session.\n")
	} else {
		if m.groupEnabled {
			groups = buildGroups(m.filtered, m.ghqRoot)
		}
		m.renderSessionRows(&b, groups, nameWidth, branchWidth)
	}

	b.WriteString("\n")

	// Preview area (when enabled and terminal is tall enough).
	if m.previewEnabled && len(m.filtered) > 0 && m.height >= 15 {
		// Calculate layout.
		// Top section: header(2) + col-header(1) + sessions + blank(1)
		sessionRows := len(m.filtered)
		if m.groupEnabled && groups != nil {
			sessionRows = groupedSessionRows(groups)
		}
		topLines := 2 + 1 + sessionRows + 1
		if m.err != nil {
			topLines += 2 // error + blank
		}
		// Bottom section needs at least: separator(1) + 1 preview line + helpbar(1) = 3
		maxPreviewHeight := m.height - topLines - 2 // minus separator, minus helpbar
		if maxPreviewHeight >= 1 {
			ph := previewHeight(m)
			// Insert padding to push preview to bottom.
			bottomLines := 1 + ph + 1 // separator + preview + helpbar
			padding := m.height - topLines - bottomLines
			if padding > 0 {
				b.WriteString(strings.Repeat("\n", padding))
			}

			// Build separator line.
			selectedName := m.filtered[m.cursor].DisplayName()
			scrollIndicator := ""
			if m.previewScrollOffset > 0 {
				scrollIndicator = " ↑ scrolled (Ctrl+D to go back)"
			}
			sepLabel := " Preview: " + selectedName + scrollIndicator + " "
			sepWidth := m.width
			if sepWidth <= 0 {
				sepWidth = 80
			}
			// Use rune count for width calculations (multi-byte safe).
			labelLen := len([]rune(sepLabel))
			if labelLen >= sepWidth {
				nameRunes := []rune(selectedName)
				maxNameLen := sepWidth - len([]rune(" Preview:  ")) - 2
				if maxNameLen > 0 && len(nameRunes) > maxNameLen {
					selectedName = string(nameRunes[:maxNameLen]) + "…"
				}
				sepLabel = " Preview: " + selectedName + " "
				labelLen = len([]rune(sepLabel))
			}
			leftPad := (sepWidth - labelLen) / 2
			rightPad := sepWidth - labelLen - leftPad
			if leftPad < 0 {
				leftPad = 0
			}
			if rightPad < 0 {
				rightPad = 0
			}
			sep := strings.Repeat("─", leftPad) + sepLabel + strings.Repeat("─", rightPad)
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
				for i := 1; i < ph; i++ {
					b.WriteString("\n")
				}
			} else {
				// Take last ph lines.
				start := len(previewLines) - ph
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
		} // maxPreviewHeight >= 1
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
			previewLabel = "p:preview(on)  Ctrl+U/D:scroll"
		}
		groupLabel := "g:group"
		if m.groupEnabled {
			groupLabel = "g:group(on)"
		}
		b.WriteString(styleHelpBar.Render("Enter:attach  y:approve  n:new  a:add-external  b:broadcast  K:kill  R:refresh  " + previewLabel + "  " + groupLabel + "  d:dash  /:filter  q:quit"))
	}

	return newView(b.String())
}

func (m Model) viewNewSession(b *strings.Builder) string {
	overlayWidth, overlayHeight := m.overlayListDims()

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
	}
	b.WriteString(styleOverlayTitle.Render("New Session — Select Repository"))
	b.WriteString("\n\n")
	b.WriteString(" ")
	b.WriteString(m.newSessionInput.View())
	b.WriteString("\n\n")

	listHeight := overlayListHeight(overlayHeight)

	if len(m.filteredDirs) == 0 {
		b.WriteString(styleHelpBar.Render(" No repositories found."))
	} else {
		maxShow := listHeight
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
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("   ... and %d more", len(m.filteredDirs)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Enter:select  Esc:cancel  ↑/↓:navigate"))

	return renderOverlayBox(b.String(), overlayWidth)
}

func (m Model) viewNewSessionPrompt(b *strings.Builder) string {
	overlayWidth := m.overlayWidth(50, 60, 80)

	b.WriteString(styleOverlayTitle.Render("New Session — Initial Prompt"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf(" Repository: %s\n\n", styleDir.Render(shortenDir(m.selectedRepoDir))))
	b.WriteString(" ")
	b.WriteString(m.newSessionPromptInput.View())
	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Enter:create  Enter(empty):skip prompt  Esc:cancel"))

	if m.err != nil {
		b.WriteString("\n\n")
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
	}

	return renderOverlayBox(b.String(), overlayWidth)
}

func (m Model) viewAddExternal(b *strings.Builder) string {
	overlayWidth, overlayHeight := m.overlayListDims()

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
	}
	b.WriteString(styleOverlayTitle.Render("Add External Session"))
	b.WriteString("\n\n")
	b.WriteString(" ")
	b.WriteString(m.addExtInput.View())
	b.WriteString("\n\n")

	listHeight := overlayListHeight(overlayHeight)

	if len(m.filteredExtWindows) == 0 {
		b.WriteString(styleHelpBar.Render(" No external windows found."))
	} else {
		maxShow := listHeight
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
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("   ... and %d more", len(m.filteredExtWindows)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Enter:add  Esc:cancel  ↑/↓:navigate"))

	return renderOverlayBox(b.String(), overlayWidth)
}

func (m Model) viewDashboard(b *strings.Builder) string {
	cols := m.dashCols()
	maxVisible := m.dashMaxVisible()
	pageItems := len(m.filtered) - m.dashPageOffset
	if pageItems > maxVisible {
		pageItems = maxVisible
	}
	if pageItems < 0 {
		pageItems = 0
	}

	// Focus mode: full-screen view of one session
	if m.dashFocused && pageItems > 0 && m.dashPageOffset+m.dashCursor < len(m.filtered) {
		s := m.filtered[m.dashPageOffset+m.dashCursor]
		icon := s.Status.Icon()
		statusStr := statusStyle(s.Status).Render(s.Status.String())
		displayName := s.DisplayName()
		branch := s.Branch
		dir := shortenDir(s.Dir)
		header := fmt.Sprintf("%s %s  %s", icon, statusStr, displayName)
		if branch != "" {
			header += "  [" + branch + "]"
		}
		if dir != "" {
			header += "  " + styleDir.Render(dir)
		}
		b.WriteString(styleHeader.Render(header))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", m.width))
		b.WriteString("\n")

		// Preview content filling remaining height
		headerLines := 2 // header + separator
		helpLines := 1
		availableLines := m.height - headerLines - helpLines
		if availableLines < 1 {
			availableLines = 1
		}
		preview := m.dashPreviews[m.dashCursor]
		lines := strings.Split(preview, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		start := len(lines) - availableLines
		if start < 0 {
			start = 0
		}
		displayLines := lines[start:]
		for _, line := range displayLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		// Pad remaining lines
		for i := len(displayLines); i < availableLines; i++ {
			b.WriteString("\n")
		}

		b.WriteString(styleHelpBar.Render("Esc:back  f:exit-focus  hjkl:navigate  q:quit"))
		return b.String()
	}

	// Normal dashboard view
	totalSessions := len(m.filtered)
	rows := (pageItems + cols - 1) / cols
	if rows == 0 {
		rows = 1
	}

	// Header with page indicator
	header := fmt.Sprintf("Clux — Dashboard (%d sessions)", totalSessions)
	if summary := statusSummary(m.filtered); summary != "" {
		header += " — " + summary
	}
	if totalSessions > maxVisible {
		currentPage := m.dashPageOffset/maxVisible + 1
		totalPages := (totalSessions + maxVisible - 1) / maxVisible
		header += fmt.Sprintf("  Page %d/%d", currentPage, totalPages)
	}
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n\n")

	if pageItems == 0 {
		b.WriteString("No sessions to display.\n")
		b.WriteString("\n")
		b.WriteString(styleHelpBar.Render("Esc:back  q:quit"))
		return b.String()
	}

	// Calculate cell dimensions
	cellWidth := m.width / cols
	if cellWidth < 20 {
		cellWidth = 20
	}
	// Reserve lines: header(2) + rows*(cellHeight+1) + helpbar(1)
	headerLines := 3
	helpLines := 2
	availableHeight := m.height - headerLines - helpLines
	if rows <= 0 {
		rows = 1
	}
	cellHeight := availableHeight / rows
	if cellHeight < 5 {
		cellHeight = 5
	}
	previewLines := cellHeight - 2 // minus header line and separator
	if previewLines < 1 {
		previewLines = 1
	}

	// Render grid row by row
	for row := 0; row < rows; row++ {
		// Build each cell for this row
		var cellContents []string
		for col := 0; col < cols; col++ {
			localIdx := row*cols + col
			if localIdx >= pageItems {
				// Empty cell
				cellContents = append(cellContents, strings.Repeat(" ", cellWidth-2))
				continue
			}
			s := m.filtered[m.dashPageOffset+localIdx]

			// Cell header: icon + status + name
			icon := s.Status.Icon()
			statusStr := s.Status.String()
			displayName := s.DisplayName()
			cellHeaderText := fmt.Sprintf(" %s %s %s", icon, statusStr, displayName)
			// Truncate if needed
			cellHeaderRunes := []rune(cellHeaderText)
			if len(cellHeaderRunes) > cellWidth-2 {
				cellHeaderText = string(cellHeaderRunes[:cellWidth-3]) + "…"
			}

			// Get preview content
			preview := ""
			if m.dashPreviews != nil {
				preview = m.dashPreviews[localIdx]
			}

			// Get last N lines of preview
			lines := strings.Split(preview, "\n")
			// Remove trailing empty lines
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			start := len(lines) - previewLines
			if start < 0 {
				start = 0
			}
			displayPreview := lines[start:]

			// Build cell string
			var cell strings.Builder
			cell.WriteString(cellHeaderText)
			cell.WriteString("\n")
			cell.WriteString(strings.Repeat("─", cellWidth-2))
			cell.WriteString("\n")
			for i := 0; i < previewLines; i++ {
				if i < len(displayPreview) {
					line := displayPreview[i]
					// Truncate line to cell width
					lineRunes := []rune(line)
					if len(lineRunes) > cellWidth-2 {
						line = string(lineRunes[:cellWidth-3]) + "…"
					}
					cell.WriteString(line)
				}
				if i < previewLines-1 {
					cell.WriteString("\n")
				}
			}
			cellContents = append(cellContents, cell.String())
		}

		// Use lipgloss to join cells horizontally
		// Apply border style to focused cell
		styledCells := make([]string, len(cellContents))
		for col, content := range cellContents {
			localIdx := row*cols + col
			style := lipgloss.NewStyle().
				Width(cellWidth - 2).
				Height(cellHeight).
				Padding(0, 1)
			if localIdx == m.dashCursor && localIdx < pageItems {
				style = style.
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("39"))
			} else if localIdx < pageItems {
				style = style.
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("240"))
			} else {
				style = style.
					Border(lipgloss.HiddenBorder())
			}
			styledCells[col] = style.Render(content)
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, styledCells...))
		b.WriteString("\n")
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(styleHelpBar.Render("Enter:attach  y:approve  K:kill  n:new  /:filter  b:broadcast  f:focus  [/]:page  hjkl:navigate  Esc/d:back  q:quit"))

	return b.String()
}

func (m Model) viewBroadcastSelect(b *strings.Builder) string {
	overlayWidth, overlayHeight := m.overlayListDims()

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
	}

	selectedCount := 0
	for _, sel := range m.broadcastSelected {
		if sel {
			selectedCount++
		}
	}

	b.WriteString(styleOverlayTitle.Render(fmt.Sprintf("Broadcast — Select Repositories (%d selected)", selectedCount)))
	b.WriteString("\n\n")
	b.WriteString(" ")
	b.WriteString(m.broadcastInput.View())
	b.WriteString("\n\n")

	listHeight := overlayListHeight(overlayHeight)

	if len(m.broadcastFiltered) == 0 {
		b.WriteString(styleHelpBar.Render(" No repositories found."))
	} else {
		maxShow := listHeight
		offset := 0
		if m.broadcastCursor >= maxShow {
			offset = m.broadcastCursor - maxShow + 1
		}
		end := offset + maxShow
		if end > len(m.broadcastFiltered) {
			end = len(m.broadcastFiltered)
		}
		for i := offset; i < end; i++ {
			fullDir := m.broadcastFiltered[i]
			dir := shortenDir(fullDir)
			checkmark := "[ ]"
			if m.broadcastSelected[fullDir] {
				checkmark = "[x]"
			}
			if i == m.broadcastCursor {
				b.WriteString(styleSelected.Render(fmt.Sprintf(" > %s %s", checkmark, dir)))
			} else {
				b.WriteString(fmt.Sprintf("   %s %s", checkmark, dir))
			}
			b.WriteString("\n")
		}
		if end < len(m.broadcastFiltered) {
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("   ... and %d more", len(m.broadcastFiltered)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Space:toggle  Enter:confirm  Esc:cancel  ↑/↓:navigate"))

	return renderOverlayBox(b.String(), overlayWidth)
}

func (m Model) viewBroadcastPrompt(b *strings.Builder) string {
	overlayWidth := m.overlayWidth(50, 60, 80)

	selectedCount := len(m.broadcastTargets)
	b.WriteString(styleOverlayTitle.Render(fmt.Sprintf("Broadcast — Enter Prompt (%d repos selected)", selectedCount)))
	b.WriteString("\n\n")

	for _, t := range m.broadcastTargets {
		b.WriteString(fmt.Sprintf("  • %s\n", styleDir.Render(shortenDir(t.dir))))
	}
	b.WriteString("\n")

	b.WriteString(" ")
	b.WriteString(m.broadcastPromptInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(styleHelpBar.Render("Enter:send  Esc:back"))

	return renderOverlayBox(b.String(), overlayWidth)
}

func (m Model) viewBroadcastWait(b *strings.Builder) string {
	total := len(m.broadcastTargets)
	ready := 0
	for _, t := range m.broadcastTargets {
		if t.ready {
			ready++
		}
	}

	b.WriteString(styleHeader.Render("Broadcast — Waiting for Sessions"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Waiting for sessions to become idle... (%d/%d ready)\n\n", ready, total))

	for _, t := range m.broadcastTargets {
		status := "waiting..."
		if t.ready {
			status = styleWorking.Render("idle")
		}
		b.WriteString(fmt.Sprintf("  %s  %s\n", status, styleDir.Render(shortenDir(t.dir))))
	}

	if len(m.broadcastErrors) > 0 {
		b.WriteString("\n")
		b.WriteString(styleError.Render("  Errors:"))
		b.WriteString("\n")
		for _, e := range m.broadcastErrors {
			b.WriteString(styleError.Render("  • " + e))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styleDir.Render(fmt.Sprintf("  Prompt: %s", m.broadcastPrompt)))
	b.WriteString("\n\n")
	b.WriteString(styleHelpBar.Render("Esc:cancel"))

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
