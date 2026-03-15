package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tanaka0325/clux/internal/session"
)

// SessionName is the name of the dedicated clux tmux session.
const SessionName = "clux"

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

// ListWindows returns all windows in the clux session that are running Claude Code, with their status.
func ListWindows() ([]session.Session, error) {
	out, err := exec.Command("tmux", "list-windows", "-t", SessionName, "-F", "#{window_index} #{window_name}").Output()
	if err != nil {
		return nil, fmt.Errorf("listing windows: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var sessions []session.Session
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		windowIndex := parts[0]
		windowName := parts[1]

		paneContent, dir, err := getPaneInfo(windowIndex)
		if err != nil {
			continue
		}

		status, isClaudeCode := detectStatus(paneContent)
		if !isClaudeCode {
			continue
		}

		sessions = append(sessions, session.Session{
			Name:        windowName,
			Dir:         dir,
			Status:      status,
			WindowIndex: windowIndex,
		})
	}

	return sessions, nil
}

// CreateWindow creates a new window in the clux session with the given name and directory,
// then starts Claude Code in it via send-keys so the shell persists after CC exits.
func CreateWindow(name, dir string) error {
	if err := exec.Command("tmux", "new-window", "-t", SessionName, "-n", name, "-c", dir).Run(); err != nil {
		return fmt.Errorf("creating window %q: %w", name, err)
	}
	// Get the new window's index
	out, err := exec.Command("tmux", "display-message", "-t", SessionName+":", "-p", "#{window_index}").Output()
	if err != nil {
		return fmt.Errorf("getting window index: %w", err)
	}
	windowIndex := strings.TrimSpace(string(out))
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
	base := filepath.Base(dir)
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
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !nameSet[candidate] {
			return candidate
		}
	}
}

// SwitchWindow selects a window in the clux session by its window index.
func SwitchWindow(windowIndex string) error {
	if err := exec.Command("tmux", "select-window", "-t", SessionName+":"+windowIndex).Run(); err != nil {
		return fmt.Errorf("switching to window %q: %w", windowIndex, err)
	}
	return nil
}

// KillWindow kills a window in the clux session by its window index.
func KillWindow(windowIndex string) error {
	if err := exec.Command("tmux", "kill-window", "-t", SessionName+":"+windowIndex).Run(); err != nil {
		return fmt.Errorf("killing window %q: %w", windowIndex, err)
	}
	return nil
}

// getPaneInfo captures the content of the first pane in the window and its working directory.
func getPaneInfo(windowIndex string) (content, dir string, err error) {
	target := SessionName + ":" + windowIndex + ".0"

	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p").Output()
	if err != nil {
		return "", "", fmt.Errorf("capturing pane for window %q: %w", windowIndex, err)
	}
	content = string(out)

	dirOut, err := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return content, "", nil
	}
	dir = strings.TrimSpace(string(dirOut))
	return content, dir, nil
}

// detectStatus analyses captured pane content to determine whether Claude Code is
// running and, if so, what state it is in.
//
// Claude Code detection: look for "-- INSERT --" status line which is unique to Claude Code TUI.
// Priority: Waiting > Working > Idle > Unknown.
func detectStatus(content string) (status session.Status, isClaudeCode bool) {
	if !hasClaudeCode(content) {
		return session.StatusUnknown, false
	}
	if isWaiting(content) {
		return session.StatusWaiting, true
	}
	if isWorking(content) {
		return session.StatusWorking, true
	}
	if isIdle(content) {
		return session.StatusIdle, true
	}
	return session.StatusUnknown, true
}

// hasClaudeCode checks whether the pane content looks like Claude Code is running.
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
func isWaiting(content string) bool {
	prompts := []string{
		"Do you want to proceed?",
		"[Y/n]",
		"[y/n]",
		"[y/N]",
		"Esc to cancel",
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
		break
	}
	return false
}
