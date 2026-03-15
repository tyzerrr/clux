package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

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
		return fmt.Errorf("creating clux session: %w", err)
	}
	return nil
}

// windowInfo holds parsed window metadata from list-windows.
type windowInfo struct {
	index string
	name  string
	dir   string
}

// ListWindows returns all windows in the clux session that are running Claude Code, with their status.
// Pane captures are run concurrently to minimize latency.
func ListWindows() ([]session.Session, error) {
	out, err := exec.Command("tmux", "list-windows", "-t", SessionName, "-F", "#{window_index}\t#{window_name}\t#{pane_current_path}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing windows: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var windows []windowInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		idx := parts[0]
		if !validWindowIndex.MatchString(idx) {
			continue
		}
		windows = append(windows, windowInfo{index: idx, name: parts[1], dir: parts[2]})
	}

	if len(windows) == 0 {
		return nil, nil
	}

	// Capture pane content concurrently.
	type captureResult struct {
		content string
		err     error
	}
	results := make([]captureResult, len(windows))
	var wg sync.WaitGroup
	for i, w := range windows {
		wg.Add(1)
		go func(i int, idx string) {
			defer wg.Done()
			content, err := capturePaneContent(idx)
			results[i] = captureResult{content: content, err: err}
		}(i, w.index)
	}
	wg.Wait()

	var sessions []session.Session
	for i, w := range windows {
		if results[i].err != nil {
			continue
		}
		status, isClaudeCode := detectStatusWithHooks(results[i].content, w.index)
		if !isClaudeCode {
			continue
		}
		summary := getWindowSummary(w.index)

		sessions = append(sessions, session.Session{
			Name:        w.name,
			Summary:     summary,
			Dir:         w.dir,
			Status:      status,
			WindowIndex: w.index,
		})
	}

	return sessions, nil
}

// CreateWindow creates a new window in the clux session with the given name and directory,
// then starts Claude Code in it via send-keys so the shell persists after CC exits.
func CreateWindow(name, dir string) error {
	name = sanitizeWindowName(name)
	out, err := exec.Command("tmux", "new-window", "-t", SessionName, "-n", name, "-c", dir, "-P", "-F", "#{window_index}").Output()
	if err != nil {
		return fmt.Errorf("creating window %q: %w", name, err)
	}
	windowIndex := strings.TrimSpace(string(out))
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("unexpected window index %q from tmux", windowIndex)
	}
	if err := exec.Command("tmux", "send-keys", "-t", SessionName+":"+windowIndex, "claude", "Enter").Run(); err != nil {
		return fmt.Errorf("starting claude in window %q: %w", name, err)
	}
	return nil
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
	if !nameSet[base] {
		return base
	}
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !nameSet[candidate] {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, 101)
}

// sanitizeWindowName removes characters that could interfere with tmux target parsing.
func sanitizeWindowName(name string) string {
	s := safeWindowName.ReplaceAllString(name, "_")
	if s == "" {
		return "window"
	}
	return s
}

// SwitchWindow selects a window in the clux session by its window index.
func SwitchWindow(windowIndex string) error {
	if !validWindowIndex.MatchString(windowIndex) {
		return fmt.Errorf("invalid window index %q", windowIndex)
	}
	if err := exec.Command("tmux", "select-window", "-t", SessionName+":"+windowIndex).Run(); err != nil {
		return fmt.Errorf("switching to window %q: %w", windowIndex, err)
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
	return nil
}

// capturePaneContent captures the content of the first pane in the window.
func capturePaneContent(windowIndex string) (string, error) {
	target := SessionName + ":" + windowIndex + ".0"
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p").Output()
	if err != nil {
		return "", fmt.Errorf("capturing pane for window %q: %w", windowIndex, err)
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

// getClaudeStatus reads the @claude-status tmux user option set by Claude Code hooks.
// Returns the status string ("working", "idle", "waiting") or empty if not set.
func getClaudeStatus(windowIndex string) string {
	target := SessionName + ":" + windowIndex + ".0"
	out, err := exec.Command("tmux", "display-message", "-t", target, "-p", "#{@claude-status}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

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

// hasActiveChildren checks whether the pane's process has grandchild processes,
// indicating that Claude Code is actively executing a tool (e.g., bash command).
// The pane PID is the shell, its child is Claude Code (node), and grandchildren
// are tool processes.
func hasActiveChildren(windowIndex string) bool {
	target := SessionName + ":" + windowIndex + ".0"
	out, err := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_pid}").Output()
	if err != nil {
		return false
	}
	panePID := strings.TrimSpace(string(out))
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

// detectStatus determines the status of a Claude Code session in a tmux window.
//
// It uses a two-tier approach:
//  1. Hooks-based: reads the @claude-status tmux option set by Claude Code hooks.
//     This is the most reliable method when configured.
//  2. Fallback: combines process tree inspection with pane content pattern matching.
//     Process tree detects active tool execution (Working). Pattern matching
//     distinguishes Waiting vs Idle from the bottom portion of the pane.
func detectStatus(content string) (status session.Status, isClaudeCode bool) {
	if !hasClaudeCode(content) {
		return session.StatusUnknown, false
	}
	bottom := bottomContent(content, bottomScanLines)
	if isWaiting(bottom) {
		return session.StatusWaiting, true
	}
	if isWorking(bottom) {
		return session.StatusWorking, true
	}
	if isIdle(bottom) {
		return session.StatusIdle, true
	}
	return session.StatusUnknown, true
}

// detectStatusWithHooks determines status using hooks (@claude-status) first,
// then falls back to process tree + pane content analysis.
func detectStatusWithHooks(content, windowIndex string) (status session.Status, isClaudeCode bool) {
	// Tier 1: Check @claude-status hook.
	if cs := getClaudeStatus(windowIndex); cs != "" {
		if st, ok := parseClaudeStatus(cs); ok {
			return st, true
		}
	}

	// Tier 2: Fallback — require Claude Code presence in pane content.
	if !hasClaudeCode(content) {
		return session.StatusUnknown, false
	}

	// Use process tree to detect active tool execution.
	if hasActiveChildren(windowIndex) {
		return session.StatusWorking, true
	}

	// Fall back to pattern matching on the bottom of the pane.
	bottom := bottomContent(content, bottomScanLines)
	if isWaiting(bottom) {
		return session.StatusWaiting, true
	}
	if isIdle(bottom) {
		return session.StatusIdle, true
	}
	return session.StatusUnknown, true
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
	}
	for _, p := range prompts {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

// isWorking returns true when the pane shows Claude Code actively processing.
func isWorking(content string) bool {
	indicators := []string{
		"✳ Fermenting",
		"✻ Baked",
		"✻ Cooked",
		"✻ Churned",
		"✻ Worked",
		"⏺ ",
	}
	for _, ind := range indicators {
		if strings.Contains(content, ind) {
			return true
		}
	}
	return false
}

// getWindowSummary retrieves the @clux-summary user option for a window.
// Returns empty string if not set or on error.
func getWindowSummary(windowIndex string) string {
	target := SessionName + ":" + windowIndex + ".0"
	out, err := exec.Command("tmux", "display-message", "-t", target, "-p", "#{@clux-summary}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isIdle returns true when the pane is at the Claude Code input prompt.
func isIdle(content string) bool {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "❯") || line == ">" {
			return true
		}
		if strings.HasPrefix(line, "-- INSERT --") {
			continue
		}
		if isSeparatorLine(line) {
			continue
		}
		break
	}
	return false
}

// isSeparatorLine returns true if the line consists entirely of box-drawing horizontal characters (─).
func isSeparatorLine(line string) bool {
	if line == "" {
		return false
	}
	for _, r := range line {
		if r != '─' {
			return false
		}
	}
	return true
}
