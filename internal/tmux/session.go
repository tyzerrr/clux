package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tanaka0325/clux/internal/session"
)

// ListWindows returns all panes across all tmux sessions that are running Claude Code, with their status.
// Uses findClaudePanes() for global detection (2 external commands total),
// then runs status detection (capture-pane + detectPaneMetadata) only on matching panes.
func ListWindows() ([]session.Session, error) {
	found, err := findClaudePanesFn()
	if err != nil {
		return nil, fmt.Errorf("finding claude panes: %w", err)
	}

	if len(found.panes) == 0 {
		return nil, nil
	}

	pm := processMaps{
		commByPID:     found.commByPID,
		childrenByPID: found.childrenByPID,
	}

	results := capturePanesConcurrently(found.panes)

	var sessions []session.Session
	for i, p := range found.panes {
		if results[i].err != nil {
			continue
		}
		content := results[i].content
		key := paneKey(p.sessionName, p.windowIndex, p.paneIndex)

		// If content hasn't changed and we have a fresh cached result, reuse it.
		// The TTL ensures we periodically re-evaluate even when content is stable,
		// because external signals (hooks, process tree) can change independently.
		unchanged, contentHash := contentHashUnchanged(key, content)
		if unchanged {
			if cached, ok := getCachedResult(key); ok && time.Since(cached.detectedAt) < paneDetectCacheTTL {
				sessions = append(sessions, session.Session{
					Name:        p.windowName,
					Summary:     cached.summary,
					Dir:         p.dir,
					Branch:      cached.branch,
					Status:      cached.status,
					WindowIndex: p.windowIndex,
					PaneIndex:   p.paneIndex,
					SessionName: p.sessionName,
				})
				continue
			}
		}

		status, summary, branch := detectPaneMetadata(content, p.sessionName, p.windowIndex, p.paneIndex, p.dir, pm, contentHash)
		setCachedResult(key, paneDetectResult{
			status:  status,
			branch:  branch,
			summary: summary,
		})

		sessions = append(sessions, session.Session{
			Name:        p.windowName,
			Summary:     summary,
			Dir:         p.dir,
			Branch:      branch,
			Status:      status,
			WindowIndex: p.windowIndex,
			PaneIndex:   p.paneIndex,
			SessionName: p.sessionName,
		})
	}

	return sessions, nil
}

// groupedSessionName returns a unique name for a short-lived grouped session.
// The name is based on the current time in nanoseconds.
func groupedSessionName() string {
	return fmt.Sprintf("clux-%d", time.Now().UnixNano())
}

// currentClientSession returns the tmux session name that the current client is attached to.
// Returns empty string on error (e.g., not inside tmux).
func currentClientSession() string {
	out, err := runTmuxOutput("display-message", "-p", "#{client_session}")
	if err != nil {
		return ""
	}
	return out
}

// switchClientGrouped switches the current tmux client to display the given window (and pane)
// in the base clux session, using grouped sessions so multiple clients can independently track
// different windows.
//
// If the client is in the base clux session, it switches directly. If the client is in a
// stale grouped session (clux-*), the old session is killed and a fresh one is created.
// For clients outside the clux session group, a new grouped session is created with
// destroy-unattached so it is cleaned up automatically. All failure paths fall back to the
// base clux session.
func switchClientGrouped(windowIndex, paneIndex string) error {
	// Verify the window exists via display-message (has-session only checks sessions).
	baseTarget := SessionName + ":" + windowIndex
	out, err := runTmuxOutput("display-message", "-t", baseTarget, "-p", "#{window_index}")
	if err != nil || out != windowIndex {
		return fmt.Errorf("window %q does not exist in session %q", windowIndex, SessionName)
	}

	clientSession := currentClientSession()

	// If already in a clux grouped session, kill it — it may be stale.
	// We always create a fresh grouped session for reliability.
	if strings.HasPrefix(clientSession, SessionName+"-") {
		debugLog(fmt.Sprintf("killing stale grouped session %q", clientSession))
		if err := runTmux("kill-session", "-t", clientSession); err != nil {
			debugLogf("failed to kill stale grouped session %q: %v", clientSession, err)
		}
	}

	// If the client is in the base clux session, use it directly.
	if clientSession == SessionName {
		if err := runTmux("select-window", "-t", baseTarget); err != nil {
			return fmt.Errorf("selecting window %q: %w", windowIndex, err)
		}
		paneTarget := SessionName + ":" + windowIndex + "." + paneIndex
		if err := runTmux("select-pane", "-t", paneTarget); err != nil {
			debugLogf("failed to select pane %q: %v", paneTarget, err)
		}
		if err := runTmux("switch-client", "-t", baseTarget); err != nil {
			return fmt.Errorf("switching client to %q: %w", baseTarget, err)
		}
		return nil
	}

	// Create a new grouped session linked to the base clux session.
	newSession := groupedSessionName()
	if err := runTmux("new-session", "-d", "-t", SessionName, "-s", newSession); err != nil {
		// Fall back to base session.
		return switchToBase(baseTarget, windowIndex)
	}

	// Auto-destroy the grouped session when the client detaches.
	if err := runTmux("set-option", "-t", newSession, "destroy-unattached", "on"); err != nil {
		debugLogf("failed to set destroy-unattached on session %q: %v", newSession, err)
	}

	target := newSession + ":" + windowIndex
	if err := runTmux("select-window", "-t", target); err != nil {
		if killErr := runTmux("kill-session", "-t", newSession); killErr != nil {
			debugLogf("failed to kill grouped session %q during cleanup: %v", newSession, killErr)
		}
		return switchToBase(baseTarget, windowIndex)
	}
	paneTarget := newSession + ":" + windowIndex + "." + paneIndex
	if err := runTmux("select-pane", "-t", paneTarget); err != nil {
		debugLogf("failed to select pane %q in grouped session: %v", paneTarget, err)
	}
	if err := runTmux("switch-client", "-t", target); err != nil {
		if killErr := runTmux("kill-session", "-t", newSession); killErr != nil {
			debugLogf("failed to kill grouped session %q during cleanup: %v", newSession, killErr)
		}
		return switchToBase(baseTarget, windowIndex)
	}
	return nil
}

// switchToBase attempts to switch the client to a window in the base clux session.
func switchToBase(baseTarget, windowIndex string) error {
	if err := runTmux("select-window", "-t", baseTarget); err != nil {
		// Last resort: try to at least attach the client to the base session.
		if fallbackErr := runTmux("switch-client", "-t", SessionName); fallbackErr != nil {
			debugLogf("last-resort switch-client to %q also failed: %v", SessionName, fallbackErr)
		}
		return fmt.Errorf("selecting window %q: %w", windowIndex, err)
	}
	if err := runTmux("switch-client", "-t", baseTarget); err != nil {
		if fallbackErr := runTmux("switch-client", "-t", SessionName); fallbackErr != nil {
			debugLogf("last-resort switch-client to %q also failed: %v", SessionName, fallbackErr)
		}
		return fmt.Errorf("switching client to session %q: %w", SessionName, err)
	}
	return nil
}

// CreateWindow creates a new window in the clux session with the given name and directory,
// running Claude Code directly. The window closes automatically when Claude Code exits.
// After creation, it switches the client to a grouped session so multiple clients can
// independently view different windows.
func CreateWindow(name, dir string, args ...string) error {
	name = sanitizeWindowName(name)
	tmuxArgs := []string{"new-window", "-a", "-t", SessionName, "-n", name, "-c", dir, "-P", "-F", "#{window_index}"}
	tmuxArgs = append(tmuxArgs, append([]string{"claude"}, args...)...)
	newIndex, err := runTmuxOutput(tmuxArgs...)
	if err != nil {
		return fmt.Errorf("creating window %q: %w", name, err)
	}
	// Clear any stale @clux-summary on the new pane (best-effort).
	if err := runTmux("set-option", "-p", "-t", SessionName+":"+newIndex+".0", "@clux-summary", ""); err != nil {
		debugLogf("failed to clear @clux-summary on %s:%s.0: %v", SessionName, newIndex, err)
	}
	if !validWindowIndex.MatchString(newIndex) {
		debugLog(fmt.Sprintf("CreateWindow: unexpected window index %q from new-window, falling back to base session", newIndex))
		if err2 := runTmux("switch-client", "-t", SessionName); err2 != nil {
			return fmt.Errorf("switching client to session %q: %w", SessionName, err2)
		}
		return nil
	}
	return switchClientGrouped(newIndex, "0")
}

// CreateWindowSilent creates a new window in the clux session with the given name and directory,
// running Claude Code directly, without switching the client to the new window.
// Returns the new window's index.
func CreateWindowSilent(name, dir string, args ...string) (string, error) {
	name = sanitizeWindowName(name)
	tmuxArgs := []string{"new-window", "-d", "-a", "-t", SessionName, "-n", name, "-c", dir, "-P", "-F", "#{window_index}"}
	tmuxArgs = append(tmuxArgs, append([]string{"claude"}, args...)...)
	newIndex, err := runTmuxOutput(tmuxArgs...)
	if err != nil {
		return "", fmt.Errorf("creating window %q: %w", name, err)
	}
	// Clear any stale @clux-summary on the new pane (best-effort).
	if err := runTmux("set-option", "-p", "-t", SessionName+":"+newIndex+".0", "@clux-summary", ""); err != nil {
		debugLogf("failed to clear @clux-summary on %s:%s.0: %v", SessionName, newIndex, err)
	}
	if !validWindowIndex.MatchString(newIndex) {
		return "", fmt.Errorf("unexpected window index %q from new-window", newIndex)
	}
	return newIndex, nil
}

// ValidateDir checks that the given path exists and is a directory.
func ValidateDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory %q does not exist", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}

// deduplicateWindowName returns a unique name by appending -2, -3, etc. if base already exists in nameSet.
func deduplicateWindowName(base string, nameSet map[string]bool) string {
	if !nameSet[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !nameSet[candidate] {
			return candidate
		}
	}
}

// GenerateWindowName creates a unique window name based on the directory basename.
// Appends -2, -3, etc. if the name already exists.
func GenerateWindowName(dir string) string {
	base := sanitizeWindowName(filepath.Base(dir))
	out, err := runTmuxOutput("list-windows", "-t", SessionName, "-F", "#{window_name}")
	if err != nil {
		return base
	}
	nameSet := make(map[string]bool)
	for _, n := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			nameSet[trimmed] = true
		}
	}
	return deduplicateWindowName(base, nameSet)
}

// sanitizeWindowName removes characters that could interfere with tmux target parsing.
func sanitizeWindowName(name string) string {
	s := safeWindowName.ReplaceAllString(name, "_")
	if s == "" {
		return "window"
	}
	return s
}

// SwitchWindow selects a window in the clux session by its window index and pane index.
func SwitchWindow(windowIndex, paneIndex string) error {
	return SwitchToWindow(SessionName, windowIndex, paneIndex)
}

// SwitchToWindow selects a window (and pane) in the given tmux session by its window index,
// then switches the client to that session to ensure visibility.
// For the base clux session, a grouped session is used so multiple clients can
// independently view different windows. For other sessions the switch is direct.
func SwitchToWindow(sessionName, windowIndex, paneIndex string) error {
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	if !validWindowIndex.MatchString(paneIndex) {
		return fmt.Errorf("invalid pane index %q", paneIndex)
	}
	// For the base clux session use grouped-session switching.
	if sessionName == SessionName {
		return switchClientGrouped(windowIndex, paneIndex)
	}
	// For other sessions, switch directly.
	target := sessionName + ":" + windowIndex
	if err := runTmux("select-window", "-t", target); err != nil {
		return fmt.Errorf("switching to window %q in session %q: %w", windowIndex, sessionName, err)
	}
	paneTarget := sessionName + ":" + windowIndex + "." + paneIndex
	// Best-effort pane selection; window is already selected so this is non-critical.
	if err := runTmux("select-pane", "-t", paneTarget); err != nil {
		debugLogf("failed to select pane %q in session %q: %v", paneTarget, sessionName, err)
	}
	if err := runTmux("switch-client", "-t", target); err != nil {
		return fmt.Errorf("switching client to session %q: %w", sessionName, err)
	}
	return nil
}

// SendKeysLiteral sends text literally (no key name interpretation) to a specific pane in a window,
// then sends Enter as a separate key press. This is safe for arbitrary prompt text that may
// contain characters tmux would otherwise interpret as key names (e.g., "Up", "C-c").
func SendKeysLiteral(sessionName, windowIndex, paneIndex, text string) error {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	// -l sends the text literally, preventing tmux from interpreting key names.
	if err := runTmux("send-keys", "-l", "-t", target, text); err != nil {
		return fmt.Errorf("sending literal keys to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	// Send Enter separately so it is interpreted as the actual Enter key.
	if err := runTmux("send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("sending Enter to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return nil
}

// WindowExists checks whether a window with the given index exists in the clux session.
func WindowExists(windowIndex string) bool {
	if !validWindowIndex.MatchString(windowIndex) {
		return false
	}
	target := SessionName + ":" + windowIndex
	return runTmux("has-session", "-t", target) == nil
}

// KillWindowForSession kills a window in the specified tmux session by its window index.
func KillWindowForSession(sessionName, windowIndex string) error {
	if sessionName == "" {
		return fmt.Errorf("empty session name")
	}
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	if err := runTmux("kill-window", "-t", sessionName+":"+windowIndex); err != nil {
		return fmt.Errorf("killing window %q in session %q: %w", windowIndex, sessionName, err)
	}
	clearPaneHashByPrefix(sessionName + ":" + windowIndex + ".")
	return nil
}

// KillWindow kills a window in the clux session by its window index.
func KillWindow(windowIndex string) error {
	return KillWindowForSession(SessionName, windowIndex)
}

// validatePaneTarget returns an error if either index is not a valid numeric string.
func validatePaneTarget(windowIndex, paneIndex string) error {
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	if !validWindowIndex.MatchString(paneIndex) {
		return fmt.Errorf("invalid pane index %q", paneIndex)
	}
	return nil
}

// CapturePaneForSession captures the visible content of a specific pane
// in the specified tmux session, with ANSI escape sequences preserved.
// windowIndex and paneIndex must be numeric strings.
func CapturePaneForSession(sessionName, windowIndex, paneIndex string) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	cmd, cancel := tmuxCommand("capture-pane", "-t", target, "-e", "-p")
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("capturing pane for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return string(out), nil
}

// CapturePaneForSessionWithOffset captures pane content at a specific scroll offset.
// scrollOffset is the number of lines above the bottom to start from.
// height is the number of lines to capture.
// When scrollOffset is 0, behaves identically to CapturePaneForSession.
func CapturePaneForSessionWithOffset(sessionName, windowIndex, paneIndex string, scrollOffset, height int) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	if scrollOffset <= 0 {
		// Live view — same as regular capture with ANSI
		cmd, cancel := tmuxCommand("capture-pane", "-t", target, "-e", "-p")
		out, err := cmd.Output()
		cancel()
		if err != nil {
			return "", fmt.Errorf("capturing pane for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
		}
		return string(out), nil
	}
	// Range capture into scrollback (tmux -S/-E are inclusive)
	if height < 1 {
		height = 1
	}
	startLine := -(scrollOffset + height - 1)
	endLine := -scrollOffset
	cmd, cancel := tmuxCommand("capture-pane", "-t", target, "-e", "-p",
		"-S", strconv.Itoa(startLine),
		"-E", strconv.Itoa(endLine),
	)
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("capturing pane with offset for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return string(out), nil
}

// CapturePaneWithScrollback captures the visible pane content plus scrollback history.
// historyLines specifies how many lines of scrollback to include (e.g. 500).
func CapturePaneWithScrollback(sessionName, windowIndex, paneIndex string, historyLines int) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	if historyLines < 1 {
		historyLines = 1
	}
	cmd, cancel := tmuxCommand("capture-pane", "-t", target, "-e", "-p",
		"-S", strconv.Itoa(-historyLines),
	)
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("capturing pane with scrollback for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return string(out), nil
}
