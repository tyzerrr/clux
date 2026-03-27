package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) viewDashboard(b *strings.Builder) string {
	cols := m.dashCols()
	maxVisible := m.dashMaxVisible()
	pageItems := len(m.filtered) - m.dashboard.pageOffset
	if pageItems > maxVisible {
		pageItems = maxVisible
	}
	if pageItems < 0 {
		pageItems = 0
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
		currentPage := m.dashboard.pageOffset/maxVisible + 1
		totalPages := (totalSessions + maxVisible - 1) / maxVisible
		header += fmt.Sprintf("  Page %d/%d", currentPage, totalPages)
	}
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n\n")

	if m.configErr != nil {
		b.WriteString(styleWarning.Render("Warning: config load failed: " + m.configErr.Error() + " (using defaults)"))
		b.WriteString("\n\n")
	}

	if pageItems == 0 {
		b.WriteString("No sessions to display.\n")
	}

	// Calculate cell dimensions
	// Distribute horizontal remainder across left columns.
	borderWidth := 2  // RoundedBorder left + right
	paddingWidth := 2 // Padding(0, 1) left + right
	cellChrome := borderWidth + paddingWidth
	baseCellWidth := m.width / cols
	widthRemainder := m.width % cols
	if baseCellWidth < 20 {
		baseCellWidth = 20
		widthRemainder = 0
	}
	// colCellWidths[col] is the total cell width (lipgloss Width, includes border+padding).
	// colContentWidths[col] is the usable text area for truncation.
	colCellWidths := make([]int, cols)
	colContentWidths := make([]int, cols)
	for c := 0; c < cols; c++ {
		w := baseCellWidth
		if c < widthRemainder {
			w++
		}
		colCellWidths[c] = w
		cw := w - cellChrome
		if cw < 16 {
			cw = 16
		}
		colContentWidths[c] = cw
	}
	// availableHeight excludes header, helpbar, border lines (2 per row), and
	// the trailing newline after each grid row (1 per row).
	// cellHeight is the inner (content-only) height of each cell.
	headerLines := 2
	if m.configErr != nil {
		headerLines += 2
	}
	helpLines := 2
	borderHeight := 2 // lipgloss RoundedBorder adds top + bottom border lines per row
	availableHeight := m.height - headerLines - helpLines - (rows * (borderHeight + 1))
	baseCellHeight := availableHeight / rows
	heightRemainder := availableHeight % rows
	if baseCellHeight < 5 {
		baseCellHeight = 5
		heightRemainder = 0
	}

	// Render grid row by row
	for row := 0; row < rows; row++ {
		// Distribute remainder lines to top rows
		rowCellHeight := baseCellHeight
		if row < heightRemainder {
			rowCellHeight++
		}
		rowPreviewLines := rowCellHeight - 2 // minus header line and separator
		if rowPreviewLines < 1 {
			rowPreviewLines = 1
		}

		// Build each cell for this row
		var cellContents []string
		for col := 0; col < cols; col++ {
			cw := colContentWidths[col]
			localIdx := row*cols + col
			if localIdx >= pageItems {
				// Empty cell
				cellContents = append(cellContents, strings.Repeat(" ", cw))
				continue
			}
			s := m.filtered[m.dashboard.pageOffset+localIdx]

			// Cell header: icon + status + name
			icon := s.Status.Icon()
			statusStr := s.Status.String()
			displayName := s.DisplayName()
			cellHeaderText := fmt.Sprintf(" %s %s %s", icon, statusStr, displayName)
			// Truncate if needed
			if lipgloss.Width(cellHeaderText) > cw {
				cellHeaderText = ansi.Truncate(cellHeaderText, cw-1, "…")
			}

			// Get preview content
			preview := ""
			if m.dashboard.previews != nil {
				preview = m.dashboard.previews[localIdx]
			}

			// Get last N lines of preview, applying scroll offset for selected tile
			lines := strings.Split(preview, "\n")
			// Remove trailing empty lines
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			scrollOff := 0
			if localIdx == m.dashboard.cursor {
				scrollOff = m.preview.scrollOffset
				maxScroll := len(lines) - rowPreviewLines
				if maxScroll < 0 {
					maxScroll = 0
				}
				if scrollOff > maxScroll {
					scrollOff = maxScroll
				}
			}
			start := len(lines) - rowPreviewLines - scrollOff
			if start < 0 {
				start = 0
			}
			end := start + rowPreviewLines
			if end > len(lines) {
				end = len(lines)
			}
			displayPreview := lines[start:end]

			// Build cell string
			var cell strings.Builder
			cell.WriteString(cellHeaderText)
			cell.WriteString("\n")
			cell.WriteString(strings.Repeat("─", cw))
			cell.WriteString("\n")
			for i := 0; i < rowPreviewLines; i++ {
				if i < len(displayPreview) {
					line := displayPreview[i]
					// Truncate line to cell width
					if lipgloss.Width(line) > cw {
						line = ansi.Truncate(line, cw-1, "…")
					}
					cell.WriteString(line)
				}
				if i < rowPreviewLines-1 {
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
			// lipgloss v2 Width/Height include border and padding.
			style := lipgloss.NewStyle().
				Width(colCellWidths[col]).
				Height(rowCellHeight + borderHeight).
				MaxHeight(rowCellHeight + borderHeight).
				Padding(0, 1)
			if localIdx == m.dashboard.cursor && localIdx < pageItems {
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
	km := m.cfg.Keymaps
	b.WriteString(styleHelpBar.Render("↵:attach  " + km.HintNavigateFull() + ":navigate  " + km.HintNewSession() + ":new  " + km.HintInput() + ":send  " + km.HintBroadcast() + ":broadcast  " + km.HintKill() + ":kill  " + km.HintScroll() + ":scroll  " + km.HintPrevPage() + ":prev " + km.HintNextPage() + ":next  " + km.HintFilter() + ":filter  Esc/" + km.HintDashboard() + ":back  " + km.HintQuit() + ":quit"))

	return b.String()
}
