package model

import (
	"fmt"
	"strings"

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

// resolvePaneIndex returns p if non-empty, otherwise "0".
func resolvePaneIndex(p string) string {
	if p == "" {
		return "0"
	}
	return p
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
