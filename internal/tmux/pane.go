package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tanaka0325/clux/internal/session"
)

var (
	// spinnerPattern matches Claude Code's activity spinner lines like
	// "✻ Cooking…" or "⏺ Reading file…". The spinner character class covers
	// dingbats (U+2720-U+2767), ⏺ (U+23FA), and geometric shapes (U+25C9-U+25CF).
	// Requires a gerund word (\w+ing) to avoid false positives on truncated
	// tool output lines like "⏺ Bash(long command…".
	spinnerPattern = regexp.MustCompile(`(?m)^\s*[\x{2720}-\x{2767}\x{23FA}\x{25C9}-\x{25CF}] \S*[Ii]ng\b.*…`)
)

// claudePaneInfo holds metadata for a pane identified as running Claude Code.
type claudePaneInfo struct {
	sessionName string
	windowIndex string
	paneIndex   string
	windowName  string
	dir         string
}

// captureResult holds the result of a concurrent pane capture.
type captureResult struct {
	content string
	err     error
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

// rawPaneInfo holds parsed pane data from tmux list-panes -a.
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
