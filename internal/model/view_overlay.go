package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/tanaka0325/clux/internal/session"
)

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

		// Preserve background on both sides of the overlay.
		left := ansi.Truncate(bgLines[bgIdx], startX, "")
		leftW := lipgloss.Width(left)
		if leftW < startX {
			left += strings.Repeat(" ", startX-leftW)
		}

		endX := startX + fgWidth
		bgW := lipgloss.Width(bgLines[bgIdx])
		right := ""
		if bgW > endX {
			right = ansi.Cut(bgLines[bgIdx], endX, bgW)
		}

		bgLines[bgIdx] = left + padded + right
	}

	return strings.Join(bgLines, "\n")
}

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

func (m Model) viewWithOverlayOn(base string, viewFn func(*strings.Builder) (string, *overlayCursor)) tea.View {
	var b strings.Builder
	overlay, cur := viewFn(&b)
	if m.width > 0 && m.height > 0 {
		v := newView(placeOverlay(base, overlay, m.width, m.height))
		if cur != nil {
			// Calculate absolute screen position of the cursor.
			// The overlay is centered on the background.
			fgLines := strings.Split(overlay, "\n")
			fgWidth := 0
			for _, line := range fgLines {
				if w := lipgloss.Width(line); w > fgWidth {
					fgWidth = w
				}
			}
			startX := (m.width - fgWidth) / 2
			if startX < 0 {
				startX = 0
			}
			bgLines := m.height
			startY := (bgLines - len(fgLines)) / 2
			if startY < 0 {
				startY = 0
			}
			// Border adds 1 on each side, padding adds 1 top/bottom and 2 left/right.
			// Content offset from overlay top-left: X = 1(border) + 2(padding) = 3, Y = 1(border) + 1(padding) = 2
			absX := startX + 3 + cur.x
			absY := startY + 2 + cur.y
			v.Cursor = tea.NewCursor(absX, absY)
		}
		return v
	}
	return newView(overlay)
}

func (m Model) viewWithOverlay(viewFn func(*strings.Builder) (string, *overlayCursor)) tea.View {
	return m.viewWithOverlayOn(m.viewListBase(), viewFn)
}

func (m Model) viewNewSession(b *strings.Builder) (string, *overlayCursor) {
	overlayWidth, overlayHeight := m.overlayListDims()

	cursorY := 0
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
		cursorY += 2 // error line + \n
	}
	b.WriteString(styleOverlayTitle.Render("New Session — Select Repository"))
	b.WriteString("\n\n")
	cursorY += 2 // title + blank line from \n\n
	b.WriteString(" ")
	b.WriteString(m.newSess.input.View())
	b.WriteString("\n\n")

	listHeight := overlayListHeight(overlayHeight)

	if len(m.filteredDirs) == 0 {
		b.WriteString(styleHelpBar.Render(" No repositories found."))
	} else {
		maxShow := listHeight
		offset := 0
		if m.newSess.cursor >= maxShow {
			offset = m.newSess.cursor - maxShow + 1
		}
		end := offset + maxShow
		if end > len(m.filteredDirs) {
			end = len(m.filteredDirs)
		}
		for i := offset; i < end; i++ {
			dir := shortenDir(m.filteredDirs[i])
			if i == m.newSess.cursor {
				b.WriteString(styleSelected.Render(fmt.Sprintf(" > %s", dir)))
			} else {
				fmt.Fprintf(b, "   %s", dir)
			}
			b.WriteString("\n")
		}
		if end < len(m.filteredDirs) {
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("   ... and %d more", len(m.filteredDirs)-end)))
		}
	}

	b.WriteString("\n\n")
	km := m.cfg.Keymaps
	b.WriteString(styleHelpBar.Render("↵:select  Esc:cancel  " + km.HintNavigate() + ":navigate"))

	cur := &overlayCursor{x: 1 + textInputCursorX(m.newSess.input), y: cursorY}
	return renderOverlayBox(b.String(), overlayWidth), cur
}

func (m Model) viewBroadcastSelect(b *strings.Builder) (string, *overlayCursor) {
	overlayWidth, overlayHeight := m.overlayListDims()

	cursorY := 0
	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n")
		cursorY += 2 // error line + \n
	}

	selectedCount := 0
	for _, sel := range m.broadcast.selected {
		if sel {
			selectedCount++
		}
	}

	b.WriteString(styleOverlayTitle.Render(fmt.Sprintf("Broadcast — Select Repositories (%d selected)", selectedCount)))
	b.WriteString("\n\n")
	cursorY += 2 // title + blank line from \n\n
	b.WriteString(" ")
	b.WriteString(m.broadcast.input.View())
	b.WriteString("\n\n")

	listHeight := overlayListHeight(overlayHeight)

	if len(m.broadcast.filtered) == 0 {
		b.WriteString(styleHelpBar.Render(" No repositories found."))
	} else {
		maxShow := listHeight
		offset := 0
		if m.broadcast.cursor >= maxShow {
			offset = m.broadcast.cursor - maxShow + 1
		}
		end := offset + maxShow
		if end > len(m.broadcast.filtered) {
			end = len(m.broadcast.filtered)
		}
		for i := offset; i < end; i++ {
			fullDir := m.broadcast.filtered[i]
			dir := shortenDir(fullDir)
			checkmark := "[ ]"
			if m.broadcast.selected[fullDir] {
				checkmark = "[x]"
			}
			if i == m.broadcast.cursor {
				b.WriteString(styleSelected.Render(fmt.Sprintf(" > %s %s", checkmark, dir)))
			} else {
				fmt.Fprintf(b, "   %s %s", checkmark, dir)
			}
			b.WriteString("\n")
		}
		if end < len(m.broadcast.filtered) {
			b.WriteString(styleHelpBar.Render(fmt.Sprintf("   ... and %d more", len(m.broadcast.filtered)-end)))
		}
	}

	b.WriteString("\n\n")
	km := m.cfg.Keymaps
	b.WriteString(styleHelpBar.Render(km.HintToggleSelect() + ":toggle  ↵:confirm  Esc:cancel  " + km.HintNavigate() + ":navigate"))

	cur := &overlayCursor{x: 1 + textInputCursorX(m.broadcast.input), y: cursorY}
	return renderOverlayBox(b.String(), overlayWidth), cur
}

func (m Model) viewBroadcastPrompt(b *strings.Builder) (string, *overlayCursor) {
	overlayWidth := m.overlayWidth(50, 60, 80)

	selectedCount := len(m.broadcast.targets)
	b.WriteString(styleOverlayTitle.Render(fmt.Sprintf("Broadcast — Enter Prompt (%d repos selected)", selectedCount)))
	b.WriteString("\n\n")

	// Line 0: title, line 1: blank (\n\n)
	cursorY := 2
	for _, t := range m.broadcast.targets {
		fmt.Fprintf(b, "  • %s\n", styleDir.Render(shortenDir(t.dir)))
		cursorY++ // one line per target
	}
	b.WriteString("\n")
	cursorY++ // blank line from \n

	b.WriteString(" ")
	b.WriteString(m.broadcast.promptInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(styleHelpBar.Render("↵:send  Esc:back"))

	cur := &overlayCursor{x: 1 + textInputCursorX(m.broadcast.promptInput), y: cursorY}
	return renderOverlayBox(b.String(), overlayWidth), cur
}

func (m Model) viewSendPrompt(b *strings.Builder) (string, *overlayCursor) {
	overlayWidth := m.overlayWidth(50, 60, 80)

	// Determine the target session based on the current mode.
	var target *session.Session
	if m.mode == ModeDashboardPrompt {
		if idx := m.dashboard.pageOffset + m.dashboard.cursor; idx < len(m.filtered) {
			target = &m.filtered[idx]
		}
	} else {
		if m.list.cursor < len(m.filtered) {
			target = &m.filtered[m.list.cursor]
		}
	}

	targetName := ""
	if target != nil {
		targetName = target.DisplayName()
	}

	b.WriteString(styleOverlayTitle.Render(fmt.Sprintf("Send to [%s]", targetName)))
	b.WriteString("\n")

	// Show dir and branch info for the target session.
	if target != nil {
		dir := target.Dir
		// Strip worktree paths to show the base repo.
		if idx := strings.Index(dir, "/.claude/worktrees/"); idx >= 0 {
			dir = dir[:idx]
		}
		info := shortenDir(dir)
		if target.Branch != "" {
			info += "  " + target.Branch
		}
		b.WriteString(styleDir.Render(" " + info))
	}
	b.WriteString("\n")

	// Line 0: title, line 1: dir/branch info
	cursorY := 2

	b.WriteString(" ")
	b.WriteString(m.dashboard.promptInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(styleHelpBar.Render("↵:send  Esc:cancel"))

	cur := &overlayCursor{x: 1 + textInputCursorX(m.dashboard.promptInput), y: cursorY}
	return renderOverlayBox(b.String(), overlayWidth), cur
}

func (m Model) viewConfirmKill(b *strings.Builder) (string, *overlayCursor) {
	overlayWidth := m.overlayWidth(40, 50, 60)

	b.WriteString(styleOverlayTitle.Render("Kill Session"))
	b.WriteString("\n\n")

	fmt.Fprintf(b, "Kill session %q? (y/n)", m.confirm.target)
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(styleError.Render("Error: " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(styleHelpBar.Render("y:kill  n/Esc:cancel"))

	return renderOverlayBox(b.String(), overlayWidth), nil
}
