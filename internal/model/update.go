package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tanaka0325/clux/internal/tmux"
)

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	km := m.cfg.Keymaps
	switch {
	case km.IsMoveUp(key):
		if len(m.filtered) > 0 {
			m.list.cursor = wrapCursor(m.list.cursor, -1, len(m.filtered))
			m.preview.scrollOffset = 0
			if m.preview.enabled {
				return m, fetchPreviewCmdForSession(m.filtered[m.list.cursor])
			}
		}

	case km.IsMoveDown(key):
		if len(m.filtered) > 0 {
			m.list.cursor = wrapCursor(m.list.cursor, 1, len(m.filtered))
			m.preview.scrollOffset = 0
			if m.preview.enabled {
				return m, fetchPreviewCmdForSession(m.filtered[m.list.cursor])
			}
		}

	case km.IsScrollUp(key):
		if m.preview.enabled && len(m.filtered) > 0 && previewHeight(m) > 0 {
			m.preview.scrollOffset += previewScrollStep(m)
			return m, fetchPreviewCmdForSessionWithOffset(m.filtered[m.list.cursor], m.preview.scrollOffset, previewHeight(m))
		}

	case km.IsScrollDown(key):
		if m.preview.enabled && len(m.filtered) > 0 && previewHeight(m) > 0 {
			m.preview.scrollOffset -= previewScrollStep(m)
			if m.preview.scrollOffset < 0 {
				m.preview.scrollOffset = 0
			}
			return m, fetchPreviewCmdForSessionWithOffset(m.filtered[m.list.cursor], m.preview.scrollOffset, previewHeight(m))
		}

	case key == "enter":
		if len(m.filtered) > 0 {
			s := m.filtered[m.list.cursor]
			sessionName, paneIndex := s.ResolveTarget(tmux.SessionName)
			windowIndex := s.WindowIndex
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, windowIndex, paneIndex); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}

	case km.IsNewSession(key):
		return m, m.enterNewSessionMode(ModeList)

	case km.IsKill(key):
		if len(m.filtered) > 0 {
			s := m.filtered[m.list.cursor]
			_, confirmPaneIndex := s.ResolveTarget(tmux.SessionName)
			m.confirm.target = s.DisplayName()
			m.confirm.windowIndex = s.WindowIndex
			m.confirm.paneIndex = confirmPaneIndex
			m.confirm.sessionName = s.SessionName
			m.err = nil
			m.confirm.returnMode = ModeList
			m.mode = ModeConfirmKill
		}

	case km.IsTogglePreview(key):
		m.preview.enabled = !m.preview.enabled
		if m.preview.enabled && len(m.filtered) > 0 {
			return m, fetchPreviewCmdForSession(m.filtered[m.list.cursor])
		}
		if !m.preview.enabled {
			m.preview.content = ""
			m.preview.scrollOffset = 0
		}

	case km.IsDashboard(key):
		m.mode = ModeDashboard
		m.dashboard.cursor = 0
		m.dashboard.pageOffset = 0
		m.preview.scrollOffset = 0
		m.dashboard.previews = make(map[int]string)
		maxVisible := m.dashMaxVisible()
		pageItems := len(m.filtered)
		if pageItems > maxVisible {
			pageItems = maxVisible
		}
		return m, fetchDashboardPreviews(m.filtered[:pageItems])

	case km.IsBroadcast(key):
		return m, m.enterBroadcastSelectMode()

	case km.IsGroup(key):
		m.groupEnabled = !m.groupEnabled

	case km.IsInput(key):
		if len(m.filtered) > 0 {
			m.mode = ModeListPrompt
			m.dashboard.promptInput.SetValue("")
			return m, m.dashboard.promptInput.Focus()
		}

	case km.IsFilter(key):
		m.mode = ModeFilter
		return m, m.filterInput.Focus()

	case km.IsQuit(key), key == "esc":
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) updateListPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.dashboard.promptInput.Value())
		if text == "" {
			return m, nil
		}
		if m.list.cursor >= len(m.filtered) {
			m.dashboard.promptInput.Blur()
			m.mode = ModeList
			return m, nil
		}
		s := m.filtered[m.list.cursor]
		m.dashboard.promptInput.Blur()
		m.mode = ModeList
		sessionName, paneIndex := s.ResolveTarget(tmux.SessionName)
		windowIndex := s.WindowIndex
		return m, func() tea.Msg {
			if err := tmux.SendKeysLiteral(sessionName, windowIndex, paneIndex, text); err != nil {
				return errMsg(err)
			}
			return nil
		}
	case "esc":
		m.dashboard.promptInput.Blur()
		m.mode = ModeList
		return m, nil
	default:
		var cmd tea.Cmd
		m.dashboard.promptInput, cmd = m.dashboard.promptInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateConfirmKill(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		windowIndex := m.confirm.windowIndex
		sessionName := m.confirm.sessionName
		if sessionName == "" {
			sessionName = tmux.SessionName
		}
		m = m.clearConfirm()
		return m, func() tea.Msg {
			if err := tmux.KillWindowForSession(sessionName, windowIndex); err != nil {
				return errMsg(err)
			}
			return windowKilledMsg{}
		}
	case "n", "esc":
		m = m.clearConfirm()
	}
	return m, nil
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
		m.list.cursor = 0
		return m, nil

	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filtered = applyFilter(m.sessions, m.filterInput.Value())
		if m.list.cursor >= len(m.filtered) {
			m.list.cursor = 0
		}
		return m, cmd
	}
}

func (m Model) updateNewSession(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	km := m.cfg.Keymaps

	switch {
	case key == "enter":
		if len(m.filteredDirs) > 0 {
			dir := m.filteredDirs[m.newSess.cursor]
			if err := tmux.ValidateDir(dir); err != nil {
				m.err = err
				return m, nil
			}
			m.newSess.input.Blur()
			m.mode = m.newSess.returnMode
			m.newSess.returnMode = 0
			name := tmux.GenerateWindowName(dir)
			return m, func() tea.Msg {
				if err := tmux.CreateWindow(name, dir); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}
		return m, nil

	case key == "esc":
		m.mode = m.newSess.returnMode
		m.newSess.returnMode = 0
		m.newSess.input.Blur()
		return m, nil

	case km.IsMoveUpNonPrintable(key):
		if len(m.filteredDirs) > 0 {
			m.newSess.cursor = wrapCursor(m.newSess.cursor, -1, len(m.filteredDirs))
		}
		return m, nil

	case km.IsMoveDownNonPrintable(key):
		if len(m.filteredDirs) > 0 {
			m.newSess.cursor = wrapCursor(m.newSess.cursor, 1, len(m.filteredDirs))
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.newSess.input, cmd = m.newSess.input.Update(msg)
		m.filteredDirs = filterDirs(m.repoDirs, m.newSess.input.Value())
		if m.newSess.cursor >= len(m.filteredDirs) {
			m.newSess.cursor = 0
		}
		return m, cmd
	}
}

func (m Model) updateDashboard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cols := m.dashCols()
	maxVisible := m.dashMaxVisible()
	pageItems := len(m.filtered) - m.dashboard.pageOffset
	if pageItems > maxVisible {
		pageItems = maxVisible
	}
	if pageItems < 0 {
		pageItems = 0
	}

	key := msg.String()
	km := m.cfg.Keymaps
	switch {
	case key == "esc":
		if m.dashboardOnly {
			return m, tea.Quit
		}
		m.mode = ModeList
		m.preview.scrollOffset = 0
		return m, nil
	case km.IsDashboard(key):
		if m.dashboardOnly {
			return m, tea.Quit
		}
		m.mode = ModeList
		m.preview.scrollOffset = 0
		return m, nil
	case km.IsQuit(key):
		return m, tea.Quit
	case km.IsScrollUp(key):
		if pageItems > 0 && m.dashboard.pageOffset+m.dashboard.cursor < len(m.filtered) {
			wasZero := m.preview.scrollOffset == 0
			m.preview.scrollOffset += dashPreviewScrollStep()
			if !wasZero {
				// Clamp only after scrollback content has been fetched
				if maxOff := dashMaxScrollOffset(m); m.preview.scrollOffset > maxOff {
					m.preview.scrollOffset = maxOff
				}
			}
			if wasZero {
				s := m.filtered[m.dashboard.pageOffset+m.dashboard.cursor]
				return m, fetchDashboardPreviewWithScrollback(s, m.dashboard.cursor)
			}
		}
	case km.IsScrollDown(key):
		if pageItems > 0 && m.dashboard.pageOffset+m.dashboard.cursor < len(m.filtered) {
			m.preview.scrollOffset -= dashPreviewScrollStep()
			if m.preview.scrollOffset < 0 {
				m.preview.scrollOffset = 0
			}
		}
	case km.IsNextPage(key):
		if cmd := m.dashNavigateToNextPage(); cmd != nil {
			return m, cmd
		}
	case km.IsPrevPage(key):
		if cmd := m.dashNavigateToPrevPage(); cmd != nil {
			return m, cmd
		}
	case km.IsMoveLeft(key):
		m.dashMoveGrid("left", cols, pageItems)
	case km.IsMoveRight(key):
		m.dashMoveGrid("right", cols, pageItems)
	case km.IsMoveUp(key):
		if cmd := m.dashMoveGrid("up", cols, pageItems); cmd != nil {
			return m, cmd
		}
	case km.IsMoveDown(key):
		if cmd := m.dashMoveGrid("down", cols, pageItems); cmd != nil {
			return m, cmd
		}
	case key == "enter":
		if pageItems > 0 && m.dashboard.pageOffset+m.dashboard.cursor < len(m.filtered) {
			s := m.filtered[m.dashboard.pageOffset+m.dashboard.cursor]
			sessionName, paneIndex := s.ResolveTarget(tmux.SessionName)
			return m, func() tea.Msg {
				if err := tmux.SwitchToWindow(sessionName, s.WindowIndex, paneIndex); err != nil {
					return errMsg(err)
				}
				return tea.QuitMsg{}
			}
		}
	case km.IsKill(key):
		if len(m.filtered) > 0 && m.dashboard.pageOffset+m.dashboard.cursor < len(m.filtered) {
			s := m.filtered[m.dashboard.pageOffset+m.dashboard.cursor]
			_, confirmPaneIndex := s.ResolveTarget(tmux.SessionName)
			m.confirm.target = s.DisplayName()
			m.confirm.windowIndex = s.WindowIndex
			m.confirm.paneIndex = confirmPaneIndex
			m.confirm.sessionName = s.SessionName
			m.err = nil
			m.confirm.returnMode = ModeDashboard
			m.mode = ModeConfirmKill
		}

	case km.IsNewSession(key):
		return m, m.enterNewSessionMode(ModeDashboard)

	case km.IsFilter(key):
		m.mode = ModeFilter
		return m, m.filterInput.Focus()

	case km.IsBroadcast(key):
		return m, m.enterBroadcastSelectMode()

	case km.IsInput(key):
		if pageItems > 0 && m.dashboard.pageOffset+m.dashboard.cursor < len(m.filtered) {
			m.mode = ModeDashboardPrompt
			m.dashboard.promptInput.SetValue("")
			return m, m.dashboard.promptInput.Focus()
		}
	}
	return m, nil
}

func (m Model) updateDashboardPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.dashboard.promptInput.Value())
		if text == "" {
			return m, nil
		}
		idx := m.dashboard.pageOffset + m.dashboard.cursor
		if idx >= len(m.filtered) {
			m.dashboard.promptInput.Blur()
			m.mode = ModeDashboard
			return m, nil
		}
		s := m.filtered[idx]
		m.dashboard.promptInput.Blur()
		m.mode = ModeDashboard
		sessionName, paneIndex := s.ResolveTarget(tmux.SessionName)
		windowIndex := s.WindowIndex
		return m, func() tea.Msg {
			if err := tmux.SendKeysLiteral(sessionName, windowIndex, paneIndex, text); err != nil {
				return errMsg(err)
			}
			return nil
		}
	case "esc":
		m.dashboard.promptInput.Blur()
		m.mode = ModeDashboard
		return m, nil
	default:
		var cmd tea.Cmd
		m.dashboard.promptInput, cmd = m.dashboard.promptInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateBroadcastSelect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	km := m.cfg.Keymaps
	switch {
	case key == "enter":
		// Collect selected dirs; if nothing selected, treat cursor item as selected.
		var selectedDirs []string
		for dir, sel := range m.broadcast.selected {
			if sel {
				selectedDirs = append(selectedDirs, dir)
			}
		}
		if len(selectedDirs) == 0 && len(m.broadcast.filtered) > 0 {
			selectedDirs = append(selectedDirs, m.broadcast.filtered[m.broadcast.cursor])
		}
		if len(selectedDirs) == 0 {
			return m, nil
		}
		uniqueDirs := selectedDirs // already unique since map keys are unique
		// Store as a temporary field; transition to prompt.
		m.broadcast.targets = make([]broadcastTarget, len(uniqueDirs))
		for i, d := range uniqueDirs {
			m.broadcast.targets[i] = broadcastTarget{dir: d}
		}
		m.broadcast.input.Blur()
		m.mode = ModeBroadcastPrompt
		m.broadcast.promptInput.SetValue("")
		return m, m.broadcast.promptInput.Focus()

	case key == "esc":
		m.mode = ModeList
		m.broadcast.input.Blur()
		m.broadcast.selected = make(map[string]bool)
		return m, nil

	case km.IsToggleSelect(key):
		// Toggle selection of item at cursor.
		if len(m.broadcast.filtered) > 0 {
			dir := m.broadcast.filtered[m.broadcast.cursor]
			m.broadcast.selected[dir] = !m.broadcast.selected[dir]
		}
		return m, nil

	case km.IsMoveUpNonPrintable(key):
		if len(m.broadcast.filtered) > 0 {
			m.broadcast.cursor = wrapCursor(m.broadcast.cursor, -1, len(m.broadcast.filtered))
		}
		return m, nil

	case km.IsMoveDownNonPrintable(key):
		if len(m.broadcast.filtered) > 0 {
			m.broadcast.cursor = wrapCursor(m.broadcast.cursor, 1, len(m.broadcast.filtered))
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.broadcast.input, cmd = m.broadcast.input.Update(msg)
		m.broadcast.filtered = filterDirs(m.broadcast.dirs, m.broadcast.input.Value())
		if m.broadcast.cursor >= len(m.broadcast.filtered) {
			m.broadcast.cursor = 0
		}
		return m, cmd
	}
}

func (m Model) updateBroadcastPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := strings.TrimSpace(m.broadcast.promptInput.Value())
		if prompt == "" {
			return m, nil
		}
		m.broadcast.promptInput.Blur()
		targets := m.broadcast.targets
		m.broadcast.targets = nil
		m.broadcast.errors = nil
		return m, func() tea.Msg {
			var dirErrors []string
			for _, t := range targets {
				if err := tmux.ValidateDir(t.dir); err != nil {
					dirErrors = append(dirErrors, fmt.Sprintf("%s: %v", t.dir, err))
					continue
				}
				name := tmux.GenerateWindowName(t.dir)
				if _, err := tmux.CreateWindowSilent(name, t.dir, prompt); err != nil {
					dirErrors = append(dirErrors, fmt.Sprintf("%s: %v", t.dir, err))
				}
			}
			return broadcastCompletedMsg{errors: dirErrors}
		}

	case "esc":
		m.broadcast.promptInput.Blur()
		m.mode = ModeBroadcastSelect
		return m, m.broadcast.input.Focus()

	default:
		var cmd tea.Cmd
		m.broadcast.promptInput, cmd = m.broadcast.promptInput.Update(msg)
		return m, cmd
	}
}
