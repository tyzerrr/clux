package tmux

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SessionName is the name of the dedicated clux tmux session.
const SessionName = "clux"

// claudeProcessName is the expected process name for Claude Code.
const claudeProcessName = "claude"

var (
	validWindowIndex = regexp.MustCompile(`^\d+$`)
	safeWindowName   = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)
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

// runTmux executes a tmux command and returns any error.
func runTmux(args ...string) error {
	cmd, cancel := tmuxCommand(args...)
	defer cancel()
	return cmd.Run()
}

// runTmuxOutput executes a tmux command and returns its trimmed output.
func runTmuxOutput(args ...string) (string, error) {
	cmd, cancel := tmuxCommand(args...)
	defer cancel()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// EnsureSession ensures the clux tmux session exists. If not, it creates one.
func EnsureSession() error {
	if err := runTmux("has-session", "-t", SessionName); err == nil {
		return nil
	}
	if err := runTmux("new-session", "-d", "-s", SessionName); err != nil {
		// Another process may have created it concurrently.
		if err2 := runTmux("has-session", "-t", SessionName); err2 == nil {
			return nil
		}
		return fmt.Errorf("creating clux session: %w", err)
	}
	return nil
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

// debugLog appends a log line to a UID-specific file in the OS temp directory
// when CLUX_DEBUG=1 is set. The file is opened once and kept open for the
// lifetime of the process.
func debugLog(msg string) {
	if !debugEnabled {
		return
	}
	debugFileOnce.Do(func() {
		f, err := os.OpenFile(filepath.Join(os.TempDir(), fmt.Sprintf("clux-debug-%d.log", os.Getuid())), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
