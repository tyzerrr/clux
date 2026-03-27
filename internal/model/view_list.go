package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderSessionRows writes session rows (grouped or flat) to the builder.
// groups must be non-nil when m.groupEnabled is true.
// panelWidth controls the width used for group header separators.
func (m Model) renderSessionRows(b *strings.Builder, groups []sessionGroup, nameWidth, branchWidth, panelWidth int) {
	b.WriteString(styleHelpBar.Render(fmt.Sprintf(" %-16s %-*s  %-*s %s", "Status", nameWidth, "Name", branchWidth, "Branch", "Dir")))
	b.WriteString("\n")
	if m.groupEnabled {
		for _, g := range groups {
			groupHeader := fmt.Sprintf("── %s (%d) ", g.name, len(g.sessions))
			remaining := panelWidth - len([]rune(groupHeader))
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
				if is.index == m.list.cursor {
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
			if i == m.list.cursor {
				row = styleSelected.Render(row)
			}
			b.WriteString(row)
			b.WriteString("\n")
		}
	}
}

// viewListBase renders the session list view dimmed, used as overlay background.
func (m Model) viewListBase() string {
	var leftBuf strings.Builder
	leftWidth := m.listPanelWidth()

	header := fmt.Sprintf("Clux — Sessions (%d)", len(m.filtered))
	if summary := statusSummary(m.filtered); summary != "" {
		header += " — " + summary
	}
	leftBuf.WriteString(styleHeader.Render(header))
	leftBuf.WriteString("\n\n")

	if m.configErr != nil {
		leftBuf.WriteString(styleWarning.Render("Warning: config load failed: " + m.configErr.Error() + " (using defaults)"))
		leftBuf.WriteString("\n\n")
	}

	nameWidth, branchWidth := m.columnWidths()

	if len(m.filtered) == 0 {
		leftBuf.WriteString("No Claude Code sessions found.\n")
	} else {
		var groups []sessionGroup
		if m.groupEnabled {
			groups = buildGroups(m.filtered, m.ghqRoot)
		}
		m.renderSessionRows(&leftBuf, groups, nameWidth, branchWidth, leftWidth)
	}

	showPreviewPanel := m.preview.enabled && len(m.filtered) > 0 && m.width >= 80
	if !showPreviewPanel {
		return styleDimmed.Render(leftBuf.String())
	}

	// Two-column layout for overlay background.
	rightWidth := m.previewPanelWidth()
	leftLines := strings.Split(leftBuf.String(), "\n")
	if len(leftLines) > 0 && leftLines[len(leftLines)-1] == "" {
		leftLines = leftLines[:len(leftLines)-1]
	}

	contentHeight := m.height
	if contentHeight < 1 {
		contentHeight = 1
	}

	var b strings.Builder
	sep := stylePreview.Render("│")
	for i := 0; i < contentHeight; i++ {
		var left string
		if i < len(leftLines) {
			left = leftLines[i]
		}
		lw := lipgloss.Width(left)
		if lw > leftWidth {
			left = ansi.Truncate(left, leftWidth, "")
		} else if lw < leftWidth {
			left += strings.Repeat(" ", leftWidth-lw)
		}

		right := strings.Repeat(" ", rightWidth)

		b.WriteString(left)
		b.WriteString(sep)
		b.WriteString(right)
		b.WriteString("\n")
	}

	return styleDimmed.Render(b.String())
}

func (m Model) View() tea.View {
	var b strings.Builder

	if m.mode == ModeConfirmKill {
		if m.confirm.returnMode == ModeDashboard {
			return m.viewWithOverlayOn(styleDimmed.Render(m.viewDashboard(&b)), m.viewConfirmKill)
		}
		return m.viewWithOverlay(m.viewConfirmKill)
	}

	if m.mode == ModeNewSession {
		if m.newSess.returnMode == ModeDashboard {
			return m.viewWithOverlayOn(styleDimmed.Render(m.viewDashboard(&b)), m.viewNewSession)
		}
		return m.viewWithOverlay(m.viewNewSession)
	}

	if m.mode == ModeDashboardPrompt {
		return m.viewWithOverlayOn(styleDimmed.Render(m.viewDashboard(&b)), m.viewSendPrompt)
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

	// Build helpbar first (spans full width at bottom).
	var helpBar string
	if m.mode == ModeFilter {
		helpBar = " / " + m.filterInput.View() + "\n\n" + styleHelpBar.Render("↵:apply  Esc:clear")
	} else {
		km := m.cfg.Keymaps
		previewLabel := km.HintTogglePreview() + ":preview"
		if m.preview.enabled {
			previewLabel = km.HintTogglePreview() + ":preview  " + km.HintScroll() + ":scroll"
		}
		groupLabel := km.HintGroup() + ":group"
		helpBar = styleHelpBar.Render("↵:attach  " + km.HintNavigate() + ":navigate  " + km.HintNewSession() + ":new  " + km.HintInput() + ":send  " + km.HintBroadcast() + ":broadcast  " + km.HintKill() + ":kill  " + previewLabel + "  " + groupLabel + "  " + km.HintDashboard() + ":dashboard  " + km.HintFilter() + ":filter  " + km.HintQuit() + "/Esc:quit")
	}

	showPreviewPanel := m.preview.enabled && len(m.filtered) > 0 && m.width >= 80
	leftWidth := m.listPanelWidth()

	nameWidth, branchWidth := m.columnWidths()

	// --- Build left panel (session list) ---
	var leftBuf strings.Builder
	header := fmt.Sprintf("Clux — Sessions (%d)", len(m.filtered))
	if summary := statusSummary(m.filtered); summary != "" {
		header += " — " + summary
	}
	leftBuf.WriteString(styleHeader.Render(header))
	leftBuf.WriteString("\n\n")

	if m.configErr != nil {
		leftBuf.WriteString(styleWarning.Render("Warning: config load failed: " + m.configErr.Error() + " (using defaults)"))
		leftBuf.WriteString("\n\n")
	}

	if m.err != nil {
		leftBuf.WriteString(styleError.Render("Error: " + m.err.Error()))
		leftBuf.WriteString("\n\n")
	}

	var groups []sessionGroup
	if len(m.filtered) == 0 {
		leftBuf.WriteString("No Claude Code sessions found. Start Claude Code in another tmux session.\n")
	} else {
		if m.groupEnabled {
			groups = buildGroups(m.filtered, m.ghqRoot)
		}
		m.renderSessionRows(&leftBuf, groups, nameWidth, branchWidth, leftWidth)
	}

	if !showPreviewPanel {
		// No preview panel: write left content + helpbar directly.
		b.WriteString(leftBuf.String())
		b.WriteString("\n")
		b.WriteString(helpBar)
	} else {
		// Two-column layout: left panel | right panel, helpbar at bottom.
		rightWidth := m.previewPanelWidth()

		// --- Build right panel (preview) ---
		var rightLines []string

		// Preview header.
		selectedName := m.filtered[m.list.cursor].DisplayName()
		scrollIndicator := ""
		if m.preview.scrollOffset > 0 {
			scrollIndicator = " ↑scrolled"
		}
		previewTitle := " Preview: " + selectedName + scrollIndicator
		if lipgloss.Width(previewTitle) > rightWidth {
			previewTitle = ansi.Truncate(previewTitle, rightWidth, "…")
		}
		rightLines = append(rightLines, styleHeader.Render(previewTitle))

		// Separator line.
		rightLines = append(rightLines, stylePreview.Render(strings.Repeat("─", rightWidth)))

		// Preview content lines.
		ph := previewHeight(m)
		previewContentLines := strings.Split(m.preview.content, "\n")
		// Remove trailing empty lines.
		for len(previewContentLines) > 0 && strings.TrimSpace(previewContentLines[len(previewContentLines)-1]) == "" {
			previewContentLines = previewContentLines[:len(previewContentLines)-1]
		}
		if len(previewContentLines) == 0 {
			rightLines = append(rightLines, stylePreview.Render("  No preview available"))
			for i := 1; i < ph; i++ {
				rightLines = append(rightLines, "")
			}
		} else {
			start := len(previewContentLines) - ph
			if start < 0 {
				start = 0
			}
			displayLines := previewContentLines[start:]
			rightLines = append(rightLines, displayLines...)
			// Pad to fill remaining height
			for i := len(displayLines); i < ph; i++ {
				rightLines = append(rightLines, "")
			}
		}

		// --- Join left and right panels line by line ---
		leftLines := strings.Split(leftBuf.String(), "\n")
		// Remove trailing empty string from Split.
		if len(leftLines) > 0 && leftLines[len(leftLines)-1] == "" {
			leftLines = leftLines[:len(leftLines)-1]
		}

		// Total content height (excluding helpbar line).
		contentHeight := m.height - 1
		if contentHeight < 1 {
			contentHeight = 1
		}

		sep := stylePreview.Render("│")

		for i := 0; i < contentHeight; i++ {
			// Left line: pad/truncate to leftWidth.
			var left string
			if i < len(leftLines) {
				left = leftLines[i]
			}
			lw := lipgloss.Width(left)
			if lw > leftWidth {
				left = ansi.Truncate(left, leftWidth, "")
			} else if lw < leftWidth {
				left += strings.Repeat(" ", leftWidth-lw)
			}

			// Right line: pad/truncate to rightWidth.
			var right string
			if i < len(rightLines) {
				right = rightLines[i]
			}
			rw := lipgloss.Width(right)
			if rw > rightWidth {
				right = ansi.Truncate(right, rightWidth, "")
			} else if rw < rightWidth {
				right += strings.Repeat(" ", rightWidth-rw)
			}

			b.WriteString(left)
			b.WriteString(sep)
			b.WriteString(right)
			b.WriteString("\x1b[0m")
			b.WriteString("\n")
		}

		// Helpbar at the bottom, spanning full width.
		b.WriteString(helpBar)
	}

	if m.mode == ModeListPrompt {
		return m.viewWithOverlayOn(styleDimmed.Render(b.String()), m.viewSendPrompt)
	}

	return newView(b.String())
}
