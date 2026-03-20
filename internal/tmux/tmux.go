package tmux

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tanaka0325/clux/internal/config"
	"github.com/tanaka0325/clux/internal/session"
)

// SessionName is the name of the dedicated clux tmux session.
const SessionName = "clux"

var (
	validWindowIndex = regexp.MustCompile(`^\d+$`)
	safeWindowName   = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)
)

// CheckTmux verifies that the current process is running inside a tmux session.
func CheckTmux() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("clux must be run inside a tmux session")
	}
	return nil
}

// EnsureSession ensures the clux tmux session exists. If not, it creates one.
func EnsureSession() error {
	if err := exec.Command("tmux", "has-session", "-t", SessionName).Run(); err == nil {
		return nil
	}
	if err := exec.Command("tmux", "new-session", "-d", "-s", SessionName).Run(); err != nil {
		// Another process may have created it concurrently.
		if exec.Command("tmux", "has-session", "-t", SessionName).Run() == nil {
			return nil
		}
		return fmt.Errorf("creating clux session: %w", err)
	}
	return nil
}

// windowInfo holds parsed window metadata from list-panes.
type windowInfo struct {
	index     string
	paneIndex string
	name      string
	dir       string
}

// captureResult holds the result of a concurrent pane capture.
type captureResult struct {
	content string
	err     error
}

// paneContentHashes stores the FNV-1a hash of the last captured pane content.
// Key format: "sessionName:windowIndex.paneIndex"
var (
	paneContentHashes   = map[string]uint64{}
	paneContentHashesMu sync.Mutex
)

// paneKey returns the canonical map key for a pane's content hash.
func paneKey(sessionName, windowIndex, paneIndex string) string {
	return sessionName + ":" + windowIndex + "." + paneIndex
}

// contentChanged compares the current content hash with the stored hash.
// Returns true if content changed since the last call. Updates the stored hash.
// On the first call for a given key (no previous hash), returns false —
// we cannot assume Working just because we haven't seen the pane before.
func contentChanged(key string, content string) bool {
	h := fnv.New64a()
	h.Write([]byte(content))
	hash := h.Sum64()
	paneContentHashesMu.Lock()
	defer paneContentHashesMu.Unlock()
	prev, exists := paneContentHashes[key]
	paneContentHashes[key] = hash
	if !exists {
		return false // First time — no previous state to compare
	}
	return prev != hash
}

// ClearPaneHash removes the stored hash for a pane.
func ClearPaneHash(sessionName, windowIndex, paneIndex string) {
	key := paneKey(sessionName, windowIndex, paneIndex)
	paneContentHashesMu.Lock()
	defer paneContentHashesMu.Unlock()
	delete(paneContentHashes, key)
}

// clearPaneHashByPrefix removes all stored hashes whose key starts with the given prefix.
func clearPaneHashByPrefix(prefix string) {
	paneContentHashesMu.Lock()
	defer paneContentHashesMu.Unlock()
	for k := range paneContentHashes {
		if strings.HasPrefix(k, prefix) {
			delete(paneContentHashes, k)
		}
	}
}

// capturePanesConcurrently captures the plain content of all given panes concurrently.
// Each result corresponds to the pane at the same index in the input slice.
func capturePanesConcurrently(sessionName string, panes []windowInfo) []captureResult {
	results := make([]captureResult, len(panes))
	var wg sync.WaitGroup
	for i, p := range panes {
		wg.Add(1)
		go func(i int, idx, paneIdx string) {
			defer wg.Done()
			content, err := capturePaneContentForSession(sessionName, idx, paneIdx)
			results[i] = captureResult{content: content, err: err}
		}(i, p.index, p.paneIndex)
	}
	wg.Wait()
	return results
}

// parseListPanesOutput parses the tab-separated output of tmux list-panes
// with format "#{window_index}\t#{pane_index}\t#{window_name}\t#{pane_current_path}".
func parseListPanesOutput(output string) []windowInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var windows []windowInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		idx := parts[0]
		if !validWindowIndex.MatchString(idx) {
			continue
		}
		paneIdx := parts[1]
		if !validWindowIndex.MatchString(paneIdx) {
			continue
		}
		windows = append(windows, windowInfo{index: idx, paneIndex: paneIdx, name: parts[2], dir: parts[3]})
	}
	return windows
}

// ListWindows returns all panes in the clux session that are running Claude Code, with their status.
// Pane captures are run concurrently to minimize latency.
func ListWindows() ([]session.Session, error) {
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", SessionName, "-F", "#{window_index}\t#{pane_index}\t#{window_name}\t#{pane_current_path}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing panes: %w", err)
	}

	windows := parseListPanesOutput(string(out))

	if len(windows) == 0 {
		return nil, nil
	}

	results := capturePanesConcurrently(SessionName, windows)

	var sessions []session.Session
	for i, w := range windows {
		if results[i].err != nil {
			continue
		}
		status, isClaudeCode := detectStatusWithHooksForSession(results[i].content, SessionName, w.index, w.paneIndex)
		if !isClaudeCode {
			continue
		}
		summary := getWindowSummaryForSession(SessionName, w.index, w.paneIndex)
		branch := getGitBranch(w.dir)

		sessions = append(sessions, session.Session{
			Name:        w.name,
			Summary:     summary,
			Dir:         w.dir,
			Branch:      branch,
			Status:      status,
			WindowIndex: w.index,
			PaneIndex:   w.paneIndex,
		})
	}

	return sessions, nil
}

// ListExternalWindows returns sessions for each registered external tmux session:window.
// All panes within each window are scanned; windows that no longer exist (stale registrations)
// are silently skipped. Scans are run concurrently.
func ListExternalWindows(externals []config.ExternalSession) []session.Session {
	if len(externals) == 0 {
		return nil
	}

	results := make([][]session.Session, len(externals))
	var wg sync.WaitGroup
	for i, ext := range externals {
		wg.Add(1)
		go func(i int, ext config.ExternalSession) {
			defer wg.Done()
			results[i] = ScanPanes(ext.Session, ext.Window)
		}(i, ext)
	}
	wg.Wait()

	var sessions []session.Session
	for _, ss := range results {
		sessions = append(sessions, ss...)
	}
	return sessions
}

// parseScanPanesOutput parses the tab-separated output of tmux list-panes for a single window
// with format "#{pane_index}\t#{window_name}\t#{pane_current_path}".
func parseScanPanesOutput(output, windowIndex string) []windowInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var panes []windowInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		paneIdx := parts[0]
		if !validWindowIndex.MatchString(paneIdx) {
			continue
		}
		panes = append(panes, windowInfo{index: windowIndex, paneIndex: paneIdx, name: parts[1], dir: parts[2]})
	}
	return panes
}

// ScanPanes checks all panes in a specific session:window and returns Sessions for those containing Claude Code.
// Returns an empty slice if the window doesn't exist or no panes contain Claude Code.
func ScanPanes(sessionName, windowIndex string) []session.Session {
	if sessionName == "" {
		return nil
	}
	if !validWindowIndex.MatchString(windowIndex) {
		return nil
	}
	// Get pane metadata via list-panes for this window.
	out, err := exec.Command("tmux", "list-panes", "-t", sessionName+":"+windowIndex, "-F", "#{pane_index}\t#{window_name}\t#{pane_current_path}").Output()
	if err != nil {
		return nil
	}
	panes := parseScanPanesOutput(string(out), windowIndex)
	if len(panes) == 0 {
		return nil
	}

	captures := capturePanesConcurrently(sessionName, panes)

	var result []session.Session
	for i, p := range panes {
		if captures[i].err != nil {
			continue
		}
		status, isClaudeCode := detectStatusWithHooksForSession(captures[i].content, sessionName, windowIndex, p.paneIndex)
		if !isClaudeCode {
			continue
		}
		summary := getWindowSummaryForSession(sessionName, windowIndex, p.paneIndex)
		branch := getGitBranch(p.dir)
		result = append(result, session.Session{
			Name:        p.name,
			Summary:     summary,
			Dir:         p.dir,
			Branch:      branch,
			Status:      status,
			WindowIndex: windowIndex,
			PaneIndex:   p.paneIndex,
			External:    true,
			SessionName: sessionName,
		})
	}
	return result
}

// ScanWindow checks if a specific session:window exists and returns a Session if it contains Claude Code.
// It scans all panes and returns the first Claude Code pane found.
// Returns nil if the window doesn't exist or no panes contain Claude Code.
func ScanWindow(sessionName, windowIndex string) *session.Session {
	results := ScanPanes(sessionName, windowIndex)
	if len(results) == 0 {
		return nil
	}
	return &results[0]
}

// ExternalWindowInfo holds metadata for a window in any tmux session.
type ExternalWindowInfo struct {
	Session     string // tmux session name
	WindowIndex string
	WindowName  string
	Dir         string // pane_current_path
}

// parseListAllWindowsOutput parses the tab-separated output of tmux list-windows -a
// with format "#{session_name}\t#{window_index}\t#{window_name}\t#{pane_current_path}".
// Windows belonging to the clux session are excluded.
func parseListAllWindowsOutput(output string) []ExternalWindowInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var windows []ExternalWindowInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		sessionName := parts[0]
		if sessionName == SessionName {
			continue
		}
		idx := parts[1]
		if !validWindowIndex.MatchString(idx) {
			continue
		}
		windows = append(windows, ExternalWindowInfo{
			Session:     sessionName,
			WindowIndex: idx,
			WindowName:  parts[2],
			Dir:         parts[3],
		})
	}
	return windows
}

// ListAllWindows returns all windows across all tmux sessions, excluding the clux session.
func ListAllWindows() ([]ExternalWindowInfo, error) {
	out, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{pane_current_path}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing all windows: %w", err)
	}
	return parseListAllWindowsOutput(string(out)), nil
}

// groupedSessionName returns a unique name for a short-lived grouped session.
// The name is based on the current time in nanoseconds.
func groupedSessionName() string {
	return fmt.Sprintf("clux-%d", time.Now().UnixNano())
}

// currentClientSession returns the tmux session name that the current client is attached to.
// Returns empty string on error (e.g., not inside tmux).
func currentClientSession() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_session}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	out, err := exec.Command("tmux", "display-message", "-t", baseTarget, "-p", "#{window_index}").Output()
	if err != nil || strings.TrimSpace(string(out)) != windowIndex {
		return fmt.Errorf("window %q does not exist in session %q", windowIndex, SessionName)
	}

	clientSession := currentClientSession()

	// If already in a clux grouped session, kill it — it may be stale.
	// We always create a fresh grouped session for reliability.
	if strings.HasPrefix(clientSession, SessionName+"-") {
		debugLog(fmt.Sprintf("killing stale grouped session %q", clientSession))
		_ = exec.Command("tmux", "kill-session", "-t", clientSession).Run()
	}

	// If the client is in the base clux session, use it directly.
	if clientSession == SessionName {
		if err := exec.Command("tmux", "select-window", "-t", baseTarget).Run(); err != nil {
			return fmt.Errorf("selecting window %q: %w", windowIndex, err)
		}
		paneTarget := SessionName + ":" + windowIndex + "." + paneIndex
		_ = exec.Command("tmux", "select-pane", "-t", paneTarget).Run()
		if err := exec.Command("tmux", "switch-client", "-t", baseTarget).Run(); err != nil {
			return fmt.Errorf("switching client to %q: %w", baseTarget, err)
		}
		return nil
	}

	// Create a new grouped session linked to the base clux session.
	newSession := groupedSessionName()
	if err := exec.Command("tmux", "new-session", "-d", "-t", SessionName, "-s", newSession).Run(); err != nil {
		// Fall back to base session.
		return switchToBase(baseTarget, windowIndex)
	}

	// Auto-destroy the grouped session when the client detaches.
	_ = exec.Command("tmux", "set-option", "-t", newSession, "destroy-unattached", "on").Run()

	target := newSession + ":" + windowIndex
	if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
		_ = exec.Command("tmux", "kill-session", "-t", newSession).Run()
		return switchToBase(baseTarget, windowIndex)
	}
	paneTarget := newSession + ":" + windowIndex + "." + paneIndex
	_ = exec.Command("tmux", "select-pane", "-t", paneTarget).Run()
	if err := exec.Command("tmux", "switch-client", "-t", target).Run(); err != nil {
		_ = exec.Command("tmux", "kill-session", "-t", newSession).Run()
		return switchToBase(baseTarget, windowIndex)
	}
	return nil
}

// switchToBase attempts to switch the client to a window in the base clux session.
func switchToBase(baseTarget, windowIndex string) error {
	if err := exec.Command("tmux", "select-window", "-t", baseTarget).Run(); err != nil {
		// Last resort: try to at least attach the client to the base session.
		_ = exec.Command("tmux", "switch-client", "-t", SessionName).Run()
		return fmt.Errorf("selecting window %q: %w", windowIndex, err)
	}
	if err := exec.Command("tmux", "switch-client", "-t", baseTarget).Run(); err != nil {
		_ = exec.Command("tmux", "switch-client", "-t", SessionName).Run()
		return fmt.Errorf("switching client to session %q: %w", SessionName, err)
	}
	return nil
}

// CreateWindow creates a new window in the clux session with the given name and directory,
// running Claude Code directly. The window closes automatically when Claude Code exits.
// After creation, it switches the client to a grouped session so multiple clients can
// independently view different windows.
func CreateWindow(name, dir string) error {
	name = sanitizeWindowName(name)
	out, err := exec.Command("tmux", "new-window", "-a", "-t", SessionName, "-n", name, "-c", dir, "-P", "-F", "#{window_index}", "claude").Output()
	if err != nil {
		return fmt.Errorf("creating window %q: %w", name, err)
	}
	newIndex := strings.TrimSpace(string(out))
	if !validWindowIndex.MatchString(newIndex) {
		debugLog(fmt.Sprintf("CreateWindow: unexpected window index %q from new-window, falling back to base session", newIndex))
		if err2 := exec.Command("tmux", "switch-client", "-t", SessionName).Run(); err2 != nil {
			return fmt.Errorf("switching client to session %q: %w", SessionName, err2)
		}
		return nil
	}
	return switchClientGrouped(newIndex, "0")
}

// CreateWindowSilent creates a new window in the clux session with the given name and directory,
// running Claude Code directly, without switching the client to the new window.
// Returns the new window's index.
func CreateWindowSilent(name, dir string) (string, error) {
	name = sanitizeWindowName(name)
	out, err := exec.Command("tmux", "new-window", "-d", "-a", "-t", SessionName, "-n", name, "-c", dir, "-P", "-F", "#{window_index}", "claude").Output()
	if err != nil {
		return "", fmt.Errorf("creating window %q: %w", name, err)
	}
	newIndex := strings.TrimSpace(string(out))
	if !validWindowIndex.MatchString(newIndex) {
		return "", fmt.Errorf("unexpected window index %q from new-window", newIndex)
	}
	return newIndex, nil
}

// GetWindowStatus captures the pane content for a given session:window.pane and returns its status.
// Returns (status, true) if the pane contains Claude Code, or (StatusUnknown, false) otherwise.
func GetWindowStatus(sessionName, windowIndex, paneIndex string) (session.Status, bool) {
	content, err := capturePaneContentForSession(sessionName, windowIndex, paneIndex)
	if err != nil {
		return session.StatusUnknown, false
	}
	status, isClaudeCode := detectStatusWithHooksForSession(content, sessionName, windowIndex, paneIndex)
	return status, isClaudeCode
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
	out, err := exec.Command("tmux", "list-windows", "-t", SessionName, "-F", "#{window_name}").Output()
	if err != nil {
		return base
	}
	existing := strings.Split(strings.TrimSpace(string(out)), "\n")
	nameSet := make(map[string]bool)
	for _, n := range existing {
		nameSet[strings.TrimSpace(n)] = true
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
// independently view different windows. For external sessions the switch is direct.
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
	// For external sessions, switch directly.
	target := sessionName + ":" + windowIndex
	if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("switching to window %q in session %q: %w", windowIndex, sessionName, err)
	}
	paneTarget := sessionName + ":" + windowIndex + "." + paneIndex
	_ = exec.Command("tmux", "select-pane", "-t", paneTarget).Run()
	if err := exec.Command("tmux", "switch-client", "-t", target).Run(); err != nil {
		return fmt.Errorf("switching client to session %q: %w", sessionName, err)
	}
	return nil
}

// SendKeys sends a key sequence to a specific pane in a window.
func SendKeys(sessionName, windowIndex, paneIndex, keys string) error {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	if err := exec.Command("tmux", "send-keys", "-t", target, keys, "Enter").Run(); err != nil {
		return fmt.Errorf("sending keys to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
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
	if err := exec.Command("tmux", "send-keys", "-l", "-t", target, text).Run(); err != nil {
		return fmt.Errorf("sending literal keys to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	// Send Enter separately so it is interpreted as the actual Enter key.
	if err := exec.Command("tmux", "send-keys", "-t", target, "Enter").Run(); err != nil {
		return fmt.Errorf("sending Enter to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return nil
}

// KillWindow kills a window in the clux session by its window index.
func KillWindow(windowIndex string) error {
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	if err := exec.Command("tmux", "kill-window", "-t", SessionName+":"+windowIndex).Run(); err != nil {
		return fmt.Errorf("killing window %q: %w", windowIndex, err)
	}
	clearPaneHashByPrefix(SessionName + ":" + windowIndex + ".")
	return nil
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

// CapturePane captures the visible content of a specific pane in the clux session
// with ANSI escape sequences preserved for colored output.
// windowIndex and paneIndex must be numeric strings.
func CapturePane(windowIndex, paneIndex string) (string, error) {
	return CapturePaneForSession(SessionName, windowIndex, paneIndex)
}

// CapturePaneForSession captures the visible content of a specific pane
// in the specified tmux session, with ANSI escape sequences preserved.
// windowIndex and paneIndex must be numeric strings.
func CapturePaneForSession(sessionName, windowIndex, paneIndex string) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-e", "-p").Output()
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
		out, err := exec.Command("tmux", "capture-pane", "-t", target, "-e", "-p").Output()
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
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-e", "-p",
		"-S", strconv.Itoa(startLine),
		"-E", strconv.Itoa(endLine),
	).Output()
	if err != nil {
		return "", fmt.Errorf("capturing pane with offset for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return string(out), nil
}

// capturePaneContentForSession captures the plain (no ANSI) content of a specific pane in a window.
func capturePaneContentForSession(sessionName, windowIndex, paneIndex string) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p").Output()
	if err != nil {
		return "", fmt.Errorf("capturing pane for window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	return string(out), nil
}

// bottomScanLines is the number of lines from the bottom of the pane to use
// for state detection. This avoids false positives from indicators that remain
// visible in scrolled-up output from previous interactions.
const bottomScanLines = 15

// bottomContent returns the last n lines of content.
func bottomContent(content string, n int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= n {
		return content
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// tmuxDisplayOption reads a tmux user option for a given session:window.pane via display-message.
// Returns the trimmed value, or empty string on error or invalid indices.
func tmuxDisplayOption(sessionName, windowIndex, paneIndex, format string) string {
	if !validWindowIndex.MatchString(windowIndex) || !validWindowIndex.MatchString(paneIndex) {
		return ""
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	out, err := exec.Command("tmux", "display-message", "-t", target, "-p", format).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getClaudeStatusForSession reads the @claude-status tmux user option for a given session:window.pane.
// Returns the status string ("working", "idle", "waiting") or empty if not set.
func getClaudeStatusForSession(sessionName, windowIndex, paneIndex string) string {
	return tmuxDisplayOption(sessionName, windowIndex, paneIndex, "#{@claude-status}")
}

// Package-level function variables for dependency injection in tests.
var (
	getClaudeStatusFn   = getClaudeStatusForSession
	hasActiveChildrenFn = hasActiveChildrenForSession
	contentChangedFn    = contentChanged
)

// parseClaudeStatus converts a @claude-status string to a session.Status.
// Returns the status and true if the string was recognized, or StatusUnknown and false otherwise.
func parseClaudeStatus(s string) (session.Status, bool) {
	switch s {
	case "working":
		return session.StatusWorking, true
	case "idle":
		return session.StatusIdle, true
	case "waiting":
		return session.StatusWaiting, true
	default:
		return session.StatusUnknown, false
	}
}

// hasActiveChildrenForSession checks whether the pane's process has grandchild processes,
// indicating that Claude Code is actively executing a tool (e.g., bash command).
// The pane PID is the shell, its child is Claude Code (node), and grandchildren
// are tool processes.
func hasActiveChildrenForSession(sessionName, windowIndex, paneIndex string) bool {
	panePID := tmuxDisplayOption(sessionName, windowIndex, paneIndex, "#{pane_pid}")
	if panePID == "" {
		return false
	}
	// Find direct children of the pane shell (e.g., the node process).
	childOut, err := exec.Command("pgrep", "-P", panePID).Output()
	if err != nil {
		return false
	}
	children := strings.Split(strings.TrimSpace(string(childOut)), "\n")
	for _, child := range children {
		child = strings.TrimSpace(child)
		if child == "" {
			continue
		}
		// Check if this child has its own children (tool processes).
		if err := exec.Command("pgrep", "-P", child).Run(); err == nil {
			return true
		}
	}
	return false
}

var (
	debugEnabled  = os.Getenv("CLUX_DEBUG") == "1"
	debugFile     *os.File
	debugFileOnce sync.Once
)

// debugLogf appends a formatted log line to /tmp/clux-debug.log when CLUX_DEBUG=1.
// Formatting is deferred so no allocation occurs when debug is disabled.
func debugLogf(format string, args ...any) {
	if !debugEnabled {
		return
	}
	debugLog(fmt.Sprintf(format, args...))
}

// debugLog appends a log line to /tmp/clux-debug.log when CLUX_DEBUG=1 is set.
// The file is opened once and kept open for the lifetime of the process.
func debugLog(msg string) {
	if !debugEnabled {
		return
	}
	debugFileOnce.Do(func() {
		f, err := os.OpenFile("/tmp/clux-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		debugFile = f
	})
	if debugFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(debugFile, "%s %s\n", ts, msg)
}

// detectStatusWithHooksForSession determines status using a hash-based change detection
// approach: if pane content changed since last check → Working. If stable, use pattern
// matching and process tree to distinguish Waiting vs Idle.
func detectStatusWithHooksForSession(content, sessionName, windowIndex, paneIndex string) (status session.Status, isClaudeCode bool) {
	key := paneKey(sessionName, windowIndex, paneIndex)

	// @claude-status hook — only trust "waiting" immediately.
	if cs := getClaudeStatusFn(sessionName, windowIndex, paneIndex); cs != "" {
		if st, ok := parseClaudeStatus(cs); ok && st == session.StatusWaiting {
			debugLogf("[%s] hook=waiting -> final=waiting", key)
			return session.StatusWaiting, true
		}
	}

	// Require Claude Code presence in pane content.
	if !hasClaudeCode(content) {
		return session.StatusUnknown, false
	}

	// Primary signal: content hash comparison.
	if contentChangedFn(key, content) {
		debugLogf("[%s] hash=changed -> final=working", key)
		return session.StatusWorking, true
	}
	debugLogf("[%s] hash=stable", key)

	// Hash stable — determine Waiting vs Idle.
	bottom := bottomContent(content, bottomScanLines)
	if isWaiting(bottom) {
		debugLogf("[%s] pattern=waiting -> final=waiting", key)
		return session.StatusWaiting, true
	}

	// Safety net: process tree check for active tool execution.
	if hasActiveChildrenFn(sessionName, windowIndex, paneIndex) {
		debugLogf("[%s] proctree=active -> final=working", key)
		return session.StatusWorking, true
	}

	debugLogf("[%s] -> final=idle", key)
	return session.StatusIdle, true
}

// hasClaudeCode checks whether the pane content looks like Claude Code is running.
// Some indicators intentionally overlap with isWaiting — this is by design so that
// waiting prompts also serve as Claude Code detection signals.
func hasClaudeCode(content string) bool {
	indicators := []string{
		"-- INSERT --",
		"Do you want to proceed?",
		"Esc to cancel",
	}
	for _, ind := range indicators {
		if strings.Contains(content, ind) {
			return true
		}
	}
	return false
}

// isWaiting returns true when the pane appears to be showing a permission prompt.
// Note: "Esc to cancel" is intentionally excluded here because it also appears
// during active tool execution (working state), causing false Waiting detection.
func isWaiting(content string) bool {
	prompts := []string{
		"Do you want to proceed?",
		"[Y/n]",
		"[y/n]",
		"[y/N]",
		"(y/n)",
		"(Y/n)",
		"(y)es / (n)o",
		"? (yes/no)",
	}
	for _, p := range prompts {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

// getGitBranch returns the current git branch for the given directory.
// Returns empty string if not a git repo or on error.
func getGitBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getWindowSummaryForSession retrieves the @clux-summary user option for a window in a given session.
// Returns empty string if not set or on error.
func getWindowSummaryForSession(sessionName, windowIndex, paneIndex string) string {
	return tmuxDisplayOption(sessionName, windowIndex, paneIndex, "#{@clux-summary}")
}

