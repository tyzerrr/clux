package tmux

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tanaka0325/clux/internal/session"
)

// SessionName is the name of the dedicated clux tmux session.
const SessionName = "clux"

// claudeProcessName is the expected process name for Claude Code.
const claudeProcessName = "claude"

var (
	validWindowIndex = regexp.MustCompile(`^\d+$`)
	safeWindowName   = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)
	// spinnerPattern matches Claude Code's activity spinner lines like
	// "✻ Cooking…" or "⏺ Reading file…". The spinner character class covers
	// dingbats (U+2720-U+2767), ⏺ (U+23FA), and geometric shapes (U+25C9-U+25CF).
	// Requires a gerund word (\w+ing) to avoid false positives on truncated
	// tool output lines like "⏺ Bash(long command…".
	spinnerPattern = regexp.MustCompile(`(?m)^\s*[\x{2720}-\x{2767}\x{23FA}\x{25C9}-\x{25CF}] \S*[Ii]ng\b.*…`)
	// idleTimerPattern matches Claude Code's idle timer lines that update
	// every second (e.g., "◆ Baked for 3m 16s", "✻ Cogitated for 47s").
	// Claude Code uses many timer words (Baked, Cooked, Cogitated, Sauteed, …)
	// so we match the structural pattern: a non-ASCII bullet char, a
	// capitalized word, "for", and a duration with time units.
	idleTimerPattern = regexp.MustCompile(`(?m)^\s*[^\x00-\x7F] [A-Z]\w* for \d+[smh].*$`)
)

const (
	tmuxCmdTimeout = 5 * time.Second
	procCmdTimeout = 2 * time.Second
	gitCmdTimeout  = 5 * time.Second
)

// commandWithTimeout creates an exec.Cmd with a timeout context.
// The returned CancelFunc must be called by the caller (via defer or explicit call)
// to release the context resources and avoid goroutine leaks.
func commandWithTimeout(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = timeout
	return cmd, cancel
}

// tmuxCommand creates an exec.Cmd for a tmux subcommand with a timeout context.
func tmuxCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	return commandWithTimeout(tmuxCmdTimeout, "tmux", args...)
}

// procCommand creates an exec.Cmd for a process inspection command with a timeout context.
func procCommand(name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	return commandWithTimeout(procCmdTimeout, name, args...)
}

// gitCommand creates an exec.Cmd for a git subcommand with a timeout context.
func gitCommand(args ...string) (*exec.Cmd, context.CancelFunc) {
	return commandWithTimeout(gitCmdTimeout, "git", args...)
}

// EnsureSession ensures the clux tmux session exists. If not, it creates one.
func EnsureSession() error {
	cmd, cancel := tmuxCommand("has-session", "-t", SessionName)
	err := cmd.Run()
	cancel()
	if err == nil {
		return nil
	}
	cmd, cancel = tmuxCommand("new-session", "-d", "-s", SessionName)
	err = cmd.Run()
	cancel()
	if err != nil {
		// Another process may have created it concurrently.
		cmd, cancel = tmuxCommand("has-session", "-t", SessionName)
		err2 := cmd.Run()
		cancel()
		if err2 == nil {
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
// paneCacheMu guards both paneContentHashes and paneDetectCache so that
// clearing operations are atomic and no goroutine can observe a partially
// cleared state.
var (
	paneCacheMu       sync.Mutex
	paneContentHashes = map[string]uint64{}
	paneDetectCache   = map[string]paneDetectResult{}
)

// paneDetectCache stores the last detected status and branch for each pane,
// keyed by the same paneKey. When the content hash hasn't changed between
// ticks, these cached values are reused to skip expensive pgrep/git calls.
type paneDetectResult struct {
	status  session.Status
	branch  string
	summary string
}

// paneKey returns the canonical map key for a pane's content hash.
func paneKey(sessionName, windowIndex, paneIndex string) string {
	return sessionName + ":" + windowIndex + "." + paneIndex
}

// hashContent computes the FNV-1a hash of a string.
func hashContent(content string) uint64 {
	// Strip idle timer lines that update every second (e.g., "◆ Baked for 3m 16s")
	// so that timer ticks alone do not cause hash changes.
	normalized := idleTimerPattern.ReplaceAllString(content, "")
	h := fnv.New64a()
	_, _ = io.WriteString(h, normalized)
	return h.Sum64()
}

// contentChanged compares the current content hash with the stored hash.
// Returns true if content changed since the last call. Updates the stored hash.
// On the first call for a given key (no previous hash), returns false —
// we cannot assume Working just because we haven't seen the pane before.
func contentChanged(key string, content string) bool {
	hash := hashContent(content)
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	prev, exists := paneContentHashes[key]
	paneContentHashes[key] = hash
	if !exists {
		return false // First time — no previous state to compare
	}
	return prev != hash
}

// ClearPaneHash removes the stored hash and cached detection result for a pane.
func ClearPaneHash(sessionName, windowIndex, paneIndex string) {
	key := paneKey(sessionName, windowIndex, paneIndex)
	paneCacheMu.Lock()
	delete(paneContentHashes, key)
	delete(paneDetectCache, key)
	paneCacheMu.Unlock()
}

// clearPaneHashByPrefix removes all stored hashes and cached results whose key starts with the given prefix.
func clearPaneHashByPrefix(prefix string) {
	paneCacheMu.Lock()
	for k := range paneContentHashes {
		if strings.HasPrefix(k, prefix) {
			delete(paneContentHashes, k)
		}
	}
	for k := range paneDetectCache {
		if strings.HasPrefix(k, prefix) {
			delete(paneDetectCache, k)
		}
	}
	paneCacheMu.Unlock()
}

// ClearAllPaneCache removes all stored hashes and cached detection results.
func ClearAllPaneCache() {
	paneCacheMu.Lock()
	clear(paneContentHashes)
	clear(paneDetectCache)
	paneCacheMu.Unlock()
}

// contentHashUnchanged checks whether the content hash for the given key
// matches the stored hash without updating it. Returns true if unchanged.
// Returns false if content changed or there is no stored hash.
func contentHashUnchanged(key, content string) bool {
	hash := hashContent(content)
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	prev, exists := paneContentHashes[key]
	if !exists {
		return false
	}
	return prev == hash
}

// getCachedResult returns the cached detection result for a pane, if any.
func getCachedResult(key string) (paneDetectResult, bool) {
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	r, ok := paneDetectCache[key]
	return r, ok
}

// setCachedResult stores a detection result in the cache.
func setCachedResult(key string, r paneDetectResult) {
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	paneDetectCache[key] = r
}

// claudePaneInfo holds metadata for a pane identified as running Claude Code.
type claudePaneInfo struct {
	sessionName string
	windowIndex string
	paneIndex   string
	windowName  string
	dir         string
}

// findClaudePanesFn is the function used to find all Claude panes.
// It can be overridden in tests.
var findClaudePanesFn = findClaudePanes

// listAllPanesFn is the function used to get all tmux panes. Overridable for tests.
var listAllPanesFn = listAllPanes

// listProcessesFn is the function used to get all processes. Overridable for tests.
var listProcessesFn = listProcesses

// listAllPanes runs tmux list-panes -a and returns the raw output.
func listAllPanes() (string, error) {
	cmd, cancel := tmuxCommand("list-panes", "-a", "-F",
		"#{pane_pid}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{window_name}\t#{pane_current_path}")
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("listing all panes: %w", err)
	}
	return string(out), nil
}

// listProcesses runs ps -ax and returns the raw output.
func listProcesses() (string, error) {
	cmd, cancel := procCommand("ps", "-ax", "-o", "pid=,ppid=,comm=")
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("listing processes: %w", err)
	}
	return string(out), nil
}

// processInfo holds parsed process metadata from ps output.
type processInfo struct {
	pid  int
	ppid int
	comm string // base name of the command
}

// parseProcessList parses the output of `ps -ax -o pid=,ppid=,comm=`.
func parseProcessList(output string) []processInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var procs []processInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		comm := filepath.Base(strings.Join(fields[2:], " "))
		procs = append(procs, processInfo{pid: pid, ppid: ppid, comm: comm})
	}
	return procs
}

// buildProcessMaps creates lookup maps from process list:
// - commByPID: pid -> command name
// - childrenByPID: ppid -> list of child PIDs
func buildProcessMaps(procs []processInfo) (commByPID map[int]string, childrenByPID map[int][]int) {
	commByPID = make(map[int]string, len(procs))
	childrenByPID = make(map[int][]int)
	for _, p := range procs {
		commByPID[p.pid] = p.comm
		childrenByPID[p.ppid] = append(childrenByPID[p.ppid], p.pid)
	}
	return
}

// paneHasClaude checks if a pane has a claude process:
// either the pane_pid itself is "claude", or any direct child of pane_pid is "claude".
func paneHasClaude(panePID int, commByPID map[int]string, childrenByPID map[int][]int) bool {
	if commByPID[panePID] == claudeProcessName {
		return true
	}
	for _, childPID := range childrenByPID[panePID] {
		if commByPID[childPID] == claudeProcessName {
			return true
		}
	}
	return false
}

// parsePaneListOutput parses the tab-separated output of tmux list-panes -a.
// Format: "pane_pid\tsession_name\twindow_index\tpane_index\twindow_name\tpane_current_path"
type rawPaneInfo struct {
	panePID     int
	sessionName string
	windowIndex string
	paneIndex   string
	windowName  string
	dir         string
}

func parsePaneListOutput(output string) []rawPaneInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var panes []rawPaneInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if !validWindowIndex.MatchString(parts[2]) || !validWindowIndex.MatchString(parts[3]) {
			continue
		}
		panes = append(panes, rawPaneInfo{
			panePID:     pid,
			sessionName: parts[1],
			windowIndex: parts[2],
			paneIndex:   parts[3],
			windowName:  parts[4],
			dir:         parts[5],
		})
	}
	return panes
}

// findClaudePanes finds ALL panes across ALL tmux sessions that have a claude process.
// Uses only 2 external commands: tmux list-panes -a and ps -ax.
// Excludes the clux UI pane itself (detected by matching our own PID's pane).
func findClaudePanes() ([]claudePaneInfo, error) {
	paneOutput, err := listAllPanesFn()
	if err != nil {
		return nil, err
	}
	psOutput, err := listProcessesFn()
	if err != nil {
		return nil, err
	}

	panes := parsePaneListOutput(paneOutput)
	procs := parseProcessList(psOutput)
	commByPID, childrenByPID := buildProcessMaps(procs)

	// Find our own PID to exclude the clux UI pane.
	myPID := os.Getpid()

	var result []claudePaneInfo
	for _, p := range panes {
		// Exclude the pane running clux itself.
		if p.panePID == myPID || isAncestorOf(p.panePID, myPID, childrenByPID) {
			continue
		}
		if paneHasClaude(p.panePID, commByPID, childrenByPID) {
			result = append(result, claudePaneInfo{
				sessionName: p.sessionName,
				windowIndex: p.windowIndex,
				paneIndex:   p.paneIndex,
				windowName:  p.windowName,
				dir:         p.dir,
			})
		}
	}
	return result, nil
}

// isAncestorOf checks if ancestorPID is an ancestor of targetPID in the process tree.
func isAncestorOf(ancestorPID, targetPID int, childrenByPID map[int][]int) bool {
	// Walk all children of ancestorPID recursively (BFS).
	visited := make(map[int]bool)
	queue := []int{ancestorPID}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if visited[pid] {
			continue
		}
		visited[pid] = true
		for _, child := range childrenByPID[pid] {
			if child == targetPID {
				return true
			}
			queue = append(queue, child)
		}
	}
	return false
}

// capturePanesConcurrently captures the plain content of all given panes concurrently.
// Each result corresponds to the pane at the same index in the input slice.
func capturePanesConcurrently(panes []claudePaneInfo) []captureResult {
	results := make([]captureResult, len(panes))
	var wg sync.WaitGroup
	for i, p := range panes {
		wg.Add(1)
		go func(i int, sessionName, idx, paneIdx string) {
			defer wg.Done()
			content, err := capturePaneContentForSession(sessionName, idx, paneIdx)
			results[i] = captureResult{content: content, err: err}
		}(i, p.sessionName, p.windowIndex, p.paneIndex)
	}
	wg.Wait()
	return results
}

// detectPaneMetadata detects status, then fetches summary and git branch concurrently.
func detectPaneMetadata(content, sessionName, windowIndex, paneIndex, dir string) (session.Status, string, string) {
	status := detectStatusWithHooksForSession(content, sessionName, windowIndex, paneIndex)
	var summary, branch string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		summary = getWindowSummaryForSession(sessionName, windowIndex, paneIndex)
	}()
	go func() {
		defer wg.Done()
		branch = getGitBranch(dir)
	}()
	wg.Wait()
	return status, summary, branch
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

// ListWindows returns all panes across all tmux sessions that are running Claude Code, with their status.
// Uses findClaudePanes() for global detection (2 external commands total),
// then runs status detection (capture-pane + detectPaneMetadata) only on matching panes.
func ListWindows() ([]session.Session, error) {
	claudePanes, err := findClaudePanesFn()
	if err != nil {
		return nil, fmt.Errorf("finding claude panes: %w", err)
	}

	if len(claudePanes) == 0 {
		return nil, nil
	}

	results := capturePanesConcurrently(claudePanes)

	var sessions []session.Session
	for i, p := range claudePanes {
		if results[i].err != nil {
			continue
		}
		content := results[i].content
		key := paneKey(p.sessionName, p.windowIndex, p.paneIndex)

		// If content hasn't changed and we have a cached result, reuse it.
		if contentHashUnchanged(key, content) {
			if cached, ok := getCachedResult(key); ok {
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

		status, summary, branch := detectPaneMetadata(content, p.sessionName, p.windowIndex, p.paneIndex, p.dir)
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
	cmd, cancel := tmuxCommand("display-message", "-p", "#{client_session}")
	out, err := cmd.Output()
	cancel()
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
	cmd, cancel := tmuxCommand("display-message", "-t", baseTarget, "-p", "#{window_index}")
	out, err := cmd.Output()
	cancel()
	if err != nil || strings.TrimSpace(string(out)) != windowIndex {
		return fmt.Errorf("window %q does not exist in session %q", windowIndex, SessionName)
	}

	clientSession := currentClientSession()

	// If already in a clux grouped session, kill it — it may be stale.
	// We always create a fresh grouped session for reliability.
	if strings.HasPrefix(clientSession, SessionName+"-") {
		debugLog(fmt.Sprintf("killing stale grouped session %q", clientSession))
		cmd, cancel = tmuxCommand("kill-session", "-t", clientSession)
		if err := cmd.Run(); err != nil {
			debugLogf("failed to kill stale grouped session %q: %v", clientSession, err)
		}
		cancel()
	}

	// If the client is in the base clux session, use it directly.
	if clientSession == SessionName {
		cmd, cancel = tmuxCommand("select-window", "-t", baseTarget)
		err = cmd.Run()
		cancel()
		if err != nil {
			return fmt.Errorf("selecting window %q: %w", windowIndex, err)
		}
		paneTarget := SessionName + ":" + windowIndex + "." + paneIndex
		cmd, cancel = tmuxCommand("select-pane", "-t", paneTarget)
		if err := cmd.Run(); err != nil {
			debugLogf("failed to select pane %q: %v", paneTarget, err)
		}
		cancel()
		cmd, cancel = tmuxCommand("switch-client", "-t", baseTarget)
		err = cmd.Run()
		cancel()
		if err != nil {
			return fmt.Errorf("switching client to %q: %w", baseTarget, err)
		}
		return nil
	}

	// Create a new grouped session linked to the base clux session.
	newSession := groupedSessionName()
	cmd, cancel = tmuxCommand("new-session", "-d", "-t", SessionName, "-s", newSession)
	err = cmd.Run()
	cancel()
	if err != nil {
		// Fall back to base session.
		return switchToBase(baseTarget, windowIndex)
	}

	// Auto-destroy the grouped session when the client detaches.
	cmd, cancel = tmuxCommand("set-option", "-t", newSession, "destroy-unattached", "on")
	if err := cmd.Run(); err != nil {
		debugLogf("failed to set destroy-unattached on session %q: %v", newSession, err)
	}
	cancel()

	target := newSession + ":" + windowIndex
	cmd, cancel = tmuxCommand("select-window", "-t", target)
	err = cmd.Run()
	cancel()
	if err != nil {
		cmd, cancel = tmuxCommand("kill-session", "-t", newSession)
		if killErr := cmd.Run(); killErr != nil {
			debugLogf("failed to kill grouped session %q during cleanup: %v", newSession, killErr)
		}
		cancel()
		return switchToBase(baseTarget, windowIndex)
	}
	paneTarget := newSession + ":" + windowIndex + "." + paneIndex
	cmd, cancel = tmuxCommand("select-pane", "-t", paneTarget)
	if err := cmd.Run(); err != nil {
		debugLogf("failed to select pane %q in grouped session: %v", paneTarget, err)
	}
	cancel()
	cmd, cancel = tmuxCommand("switch-client", "-t", target)
	err = cmd.Run()
	cancel()
	if err != nil {
		cmd, cancel = tmuxCommand("kill-session", "-t", newSession)
		if killErr := cmd.Run(); killErr != nil {
			debugLogf("failed to kill grouped session %q during cleanup: %v", newSession, killErr)
		}
		cancel()
		return switchToBase(baseTarget, windowIndex)
	}
	return nil
}

// switchToBase attempts to switch the client to a window in the base clux session.
func switchToBase(baseTarget, windowIndex string) error {
	cmd, cancel := tmuxCommand("select-window", "-t", baseTarget)
	err := cmd.Run()
	cancel()
	if err != nil {
		// Last resort: try to at least attach the client to the base session.
		cmd, cancel = tmuxCommand("switch-client", "-t", SessionName)
		if fallbackErr := cmd.Run(); fallbackErr != nil {
			debugLogf("last-resort switch-client to %q also failed: %v", SessionName, fallbackErr)
		}
		cancel()
		return fmt.Errorf("selecting window %q: %w", windowIndex, err)
	}
	cmd, cancel = tmuxCommand("switch-client", "-t", baseTarget)
	err = cmd.Run()
	cancel()
	if err != nil {
		cmd, cancel = tmuxCommand("switch-client", "-t", SessionName)
		if fallbackErr := cmd.Run(); fallbackErr != nil {
			debugLogf("last-resort switch-client to %q also failed: %v", SessionName, fallbackErr)
		}
		cancel()
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
	cmd, cancel := tmuxCommand(tmuxArgs...)
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return fmt.Errorf("creating window %q: %w", name, err)
	}
	newIndex := strings.TrimSpace(string(out))
	// Clear any stale @clux-summary on the new pane (best-effort).
	cmd, cancel = tmuxCommand("set-option", "-p", "-t", SessionName+":"+newIndex+".0", "@clux-summary", "")
	if err := cmd.Run(); err != nil {
		debugLogf("failed to clear @clux-summary on %s:%s.0: %v", SessionName, newIndex, err)
	}
	cancel()
	if !validWindowIndex.MatchString(newIndex) {
		debugLog(fmt.Sprintf("CreateWindow: unexpected window index %q from new-window, falling back to base session", newIndex))
		cmd, cancel = tmuxCommand("switch-client", "-t", SessionName)
		err2 := cmd.Run()
		cancel()
		if err2 != nil {
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
	cmd, cancel := tmuxCommand(tmuxArgs...)
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return "", fmt.Errorf("creating window %q: %w", name, err)
	}
	newIndex := strings.TrimSpace(string(out))
	// Clear any stale @clux-summary on the new pane (best-effort).
	cmd, cancel = tmuxCommand("set-option", "-p", "-t", SessionName+":"+newIndex+".0", "@clux-summary", "")
	if err := cmd.Run(); err != nil {
		debugLogf("failed to clear @clux-summary on %s:%s.0: %v", SessionName, newIndex, err)
	}
	cancel()
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
	cmd, cancel := tmuxCommand("list-windows", "-t", SessionName, "-F", "#{window_name}")
	out, err := cmd.Output()
	cancel()
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
	cmd, cancel := tmuxCommand("select-window", "-t", target)
	err := cmd.Run()
	cancel()
	if err != nil {
		return fmt.Errorf("switching to window %q in session %q: %w", windowIndex, sessionName, err)
	}
	paneTarget := sessionName + ":" + windowIndex + "." + paneIndex
	// Best-effort pane selection; window is already selected so this is non-critical.
	cmd, cancel = tmuxCommand("select-pane", "-t", paneTarget)
	if err := cmd.Run(); err != nil {
		debugLogf("failed to select pane %q in session %q: %v", paneTarget, sessionName, err)
	}
	cancel()
	cmd, cancel = tmuxCommand("switch-client", "-t", target)
	err = cmd.Run()
	cancel()
	if err != nil {
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
	cmd, cancel := tmuxCommand("send-keys", "-l", "-t", target, text)
	err := cmd.Run()
	cancel()
	if err != nil {
		return fmt.Errorf("sending literal keys to window %q pane %q in session %q: %w", windowIndex, paneIndex, sessionName, err)
	}
	// Send Enter separately so it is interpreted as the actual Enter key.
	cmd, cancel = tmuxCommand("send-keys", "-t", target, "Enter")
	err = cmd.Run()
	cancel()
	if err != nil {
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
	cmd, cancel := tmuxCommand("has-session", "-t", target)
	err := cmd.Run()
	cancel()
	return err == nil
}

// KillWindowForSession kills a window in the specified tmux session by its window index.
func KillWindowForSession(sessionName, windowIndex string) error {
	if sessionName == "" {
		return fmt.Errorf("empty session name")
	}
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	cmd, cancel := tmuxCommand("kill-window", "-t", sessionName+":"+windowIndex)
	err := cmd.Run()
	cancel()
	if err != nil {
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

// capturePaneContentForSession captures the plain (no ANSI) content of a specific pane in a window.
func capturePaneContentForSession(sessionName, windowIndex, paneIndex string) (string, error) {
	if err := validatePaneTarget(windowIndex, paneIndex); err != nil {
		return "", err
	}
	target := sessionName + ":" + windowIndex + "." + paneIndex
	cmd, cancel := tmuxCommand("capture-pane", "-t", target, "-p")
	out, err := cmd.Output()
	cancel()
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
	// Trim trailing blank lines so padding doesn't push real content out of the window.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
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
	cmd, cancel := tmuxCommand("display-message", "-t", target, "-p", format)
	out, err := cmd.Output()
	cancel()
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

// hasActiveChildrenForSession checks whether the pane's process has grandchild
// processes that indicate active tool execution (e.g., bash → gh, grep).
// Persistent child processes (MCP servers, LSP servers, caffeinate) are excluded
// since they always have grandchildren and would cause false positives.
func hasActiveChildrenForSession(sessionName, windowIndex, paneIndex string) bool {
	panePID := tmuxDisplayOption(sessionName, windowIndex, paneIndex, "#{pane_pid}")
	if panePID == "" {
		return false
	}
	cmd, cancel := procCommand("pgrep", "-P", panePID)
	childOut, err := cmd.Output()
	cancel()
	if err != nil {
		return false
	}
	for _, child := range strings.Split(strings.TrimSpace(string(childOut)), "\n") {
		if child = strings.TrimSpace(child); child == "" {
			continue
		}
		if isPersistentChild(child) {
			continue
		}
		cmd, cancel = procCommand("pgrep", "-P", child)
		err = cmd.Run()
		cancel()
		if err == nil {
			return true
		}
	}
	return false
}

// isPersistentChild returns true if the given PID is a long-lived background
// process (MCP server, LSP, etc.) that should be excluded from grandchild
// detection. These processes are always present and their grandchildren do not
// indicate active tool execution.
func isPersistentChild(pid string) bool {
	cmd, cancel := procCommand("ps", "-o", "comm=", "-p", pid)
	out, err := cmd.Output()
	cancel()
	if err != nil {
		return false
	}
	comm := strings.TrimSpace(string(out))
	name := filepath.Base(comm)
	// Known persistent processes spawned by Claude Code.
	switch name {
	case "node", "gopls", "caffeinate",
		"bigbrother-mcp-server", "tasq", "memq":
		return true
	}
	return false
}

var (
	debugEnabled  = os.Getenv("CLUX_DEBUG") == "1"
	debugLogger   *log.Logger
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
		f, err := os.OpenFile("/tmp/clux-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		debugLogger = log.New(f, "", log.Ldate|log.Ltime)
	})
	if debugLogger == nil {
		return
	}
	debugLogger.Println(msg)
}

// detectStatusWithHooksForSession determines status using a hash-based change detection
// approach: if pane content changed since last check → Working. If stable, use pattern
// matching and process tree to distinguish Waiting vs Idle.
// This function assumes the pane is already known to contain Claude Code.
func detectStatusWithHooksForSession(content, sessionName, windowIndex, paneIndex string) session.Status {
	key := paneKey(sessionName, windowIndex, paneIndex)

	// Primary signal: content hash comparison.
	if contentChangedFn(key, content) {
		debugLogf("[%s] hash=changed -> final=working", key)
		return session.StatusWorking
	}
	debugLogf("[%s] hash=stable", key)

	bottom := bottomContent(content, bottomScanLines)

	// Active-work detection via pane content patterns. Catches background
	// agents (which run inside the Claude Code process without grandchild
	// processes) and spinner-based activity indicators.
	if isWorking(bottom) {
		debugLogf("[%s] pattern=working -> final=working", key)
		return session.StatusWorking
	}

	// Process tree check for active tool execution (grandchild processes).
	if hasActiveChildrenFn(sessionName, windowIndex, paneIndex) {
		debugLogf("[%s] proctree=active -> final=working", key)
		return session.StatusWorking
	}

	// @claude-status hook — trust "waiting" only after ruling out active work.
	if cs := getClaudeStatusFn(sessionName, windowIndex, paneIndex); cs != "" {
		if st, ok := parseClaudeStatus(cs); ok && st == session.StatusWaiting {
			debugLogf("[%s] hook=waiting -> final=waiting", key)
			return session.StatusWaiting
		}
	}

	// Pattern matching for permission prompts.
	if isWaiting(bottom) {
		debugLogf("[%s] pattern=waiting -> final=waiting", key)
		return session.StatusWaiting
	}

	debugLogf("[%s] -> final=idle", key)
	return session.StatusIdle
}

// isWorking returns true when the pane content indicates active work.
// This covers cases that the process tree cannot always detect:
//  1. Background agents — they run inside the Claude Code process without
//     spawning grandchild processes.
//  2. Interrupt prompts — Claude Code shows "esc to interrupt" or
//     "ctrl+c to interrupt" while actively executing tools.
//  3. Spinner-based activity — Claude Code shows a spinner character (from
//     the dingbat/symbol range) followed by "…" while working.
func isWorking(content string) bool {
	// Exact-match patterns for active work indicators.
	patterns := []string{
		"local agent still running",
		"local agents still running",
		"esc to interrupt",
		"ctrl+c to interrupt",
	}
	for _, p := range patterns {
		if strings.Contains(content, p) {
			return true
		}
	}

	// Spinner activity: a line like "✻ Cooking…" or "⏺ Reading file…".
	// Must start with a spinner char followed by a gerund word (ending in
	// "ing") to avoid false positives on truncated tool output lines like
	// "⏺ Bash(very long command…".
	if spinnerPattern.MatchString(content) {
		return true
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
	cmd, cancel := gitCommand("-C", dir, "branch", "--show-current")
	out, err := cmd.Output()
	cancel()
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
