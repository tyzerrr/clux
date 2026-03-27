package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tanaka0325/clux/internal/session"
)

// wrapCursor wraps cursor movement within [0, length) using modular arithmetic.
func wrapCursor(current, delta, length int) int {
	if length == 0 {
		return 0
	}
	return (current + delta + length) % length
}

// clampCursor ensures cursor is within [0, length-1].
func clampCursor(cursor, length int) int {
	if length == 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	if cursor < 0 {
		return 0
	}
	return cursor
}

// shortenDir returns the last two path components of a directory path.
func shortenDir(dir string) string {
	parts := strings.Split(dir, "/")
	if len(parts) <= 2 {
		return dir
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

// statusSummary builds a string like "1 Waiting, 2 Working, 3 Idle".
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

// enterNewSessionMode prepares the model for ModeNewSession from the given returnMode.
func (m *Model) enterNewSessionMode(returnMode Mode) tea.Cmd {
	m.mode = ModeNewSession
	m.newSess.returnMode = returnMode
	m.err = nil
	m.newSess.input.SetValue("")
	m.newSess.cursor = 0
	cmds := []tea.Cmd{m.newSess.input.Focus()}
	if len(m.repoDirs) == 0 {
		cmds = append(cmds, fetchGhqDirs)
	} else {
		m.filteredDirs = m.repoDirs
	}
	return tea.Batch(cmds...)
}

// enterBroadcastSelectMode prepares the model for ModeBroadcastSelect.
func (m *Model) enterBroadcastSelectMode() tea.Cmd {
	m.mode = ModeBroadcastSelect
	m.err = nil
	m.broadcast.input.SetValue("")
	m.broadcast.cursor = 0
	m.broadcast.selected = make(map[string]bool)
	cmds := []tea.Cmd{m.broadcast.input.Focus()}
	if len(m.broadcast.dirs) == 0 {
		cmds = append(cmds, fetchBroadcastGhqDirs)
	} else {
		m.broadcast.filtered = m.broadcast.dirs
	}
	return tea.Batch(cmds...)
}

// dashPageSessions returns the slice of sessions visible on the current dashboard page.
func (m Model) dashPageSessions() []session.Session {
	maxVisible := m.dashMaxVisible()
	end := m.dashboard.pageOffset + maxVisible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	return m.filtered[m.dashboard.pageOffset:end]
}

// dashNavigateToNextPage moves to the next dashboard page if available and returns
// a command to fetch previews. Returns nil if no next page.
func (m *Model) dashNavigateToNextPage() tea.Cmd {
	maxVisible := m.dashMaxVisible()
	nextOffset := m.dashboard.pageOffset + maxVisible
	if nextOffset >= len(m.filtered) {
		return nil
	}
	m.dashboard.pageOffset = nextOffset
	m.dashboard.cursor = 0
	m.preview.scrollOffset = 0
	return fetchDashboardPreviews(m.dashPageSessions())
}

// dashNavigateToPrevPage moves to the previous dashboard page if available and returns
// a command to fetch previews. Returns nil if already on the first page.
func (m *Model) dashNavigateToPrevPage() tea.Cmd {
	maxVisible := m.dashMaxVisible()
	prevOffset := m.dashboard.pageOffset - maxVisible
	if prevOffset < 0 {
		prevOffset = 0
	}
	if prevOffset == m.dashboard.pageOffset {
		return nil
	}
	m.dashboard.pageOffset = prevOffset
	m.dashboard.cursor = 0
	m.preview.scrollOffset = 0
	return fetchDashboardPreviews(m.dashPageSessions())
}

// dashMoveGrid handles cursor movement within the dashboard grid.
// direction: "left", "right", "up", "down"
// Returns a cmd that is non-nil if a page change occurred requiring preview fetches.
func (m *Model) dashMoveGrid(direction string, cols, pageItems int) tea.Cmd {
	switch direction {
	case "left":
		if m.dashboard.cursor%cols > 0 {
			m.dashboard.cursor--
			m.preview.scrollOffset = 0
		}
	case "right":
		if m.dashboard.cursor%cols < cols-1 && m.dashboard.cursor+1 < pageItems {
			m.dashboard.cursor++
			m.preview.scrollOffset = 0
		}
	case "up":
		if m.dashboard.cursor-cols >= 0 {
			m.dashboard.cursor -= cols
			m.preview.scrollOffset = 0
		} else if m.dashboard.pageOffset > 0 {
			return m.dashNavigateToPrevPage()
		}
	case "down":
		if m.dashboard.cursor+cols < pageItems {
			m.dashboard.cursor += cols
			m.preview.scrollOffset = 0
		} else if m.dashboard.pageOffset+m.dashMaxVisible() < len(m.filtered) {
			return m.dashNavigateToNextPage()
		}
	}
	return nil
}

// --- Styles ---

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
	styleWarning = styleWaiting
	styleIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	styleUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta

	styleOverlayBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("6")). // cyan
				Padding(1, 2)
	styleOverlayTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleDimmed       = lipgloss.NewStyle().Faint(true)
)

// --- Constants ---

const overlayBorderPadding = 6 // Border(2) + Padding(1,2)*2 = 6
const dashScrollbackLines = 500
