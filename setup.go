package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const tmuxConfBinding = `bind-key -T root C-. display-popup -E -w100% -h100% "clux"`

// Old variants that need migration to tmuxConfBinding.
// tmuxConfBindingOld is the original prefix-based binding (C-c with 80% size).
const tmuxConfBindingOld = `bind-key C-c display-popup -E -w80% -h80% "clux"`

// tmuxConfBindingOldNewKey is the new key (C-.) but with the old 80% size.
const tmuxConfBindingOldNewKey = `bind-key -T root C-. display-popup -E -w80% -h80% "clux"`

// tmuxConfBindingOldKeyNewSize is the old prefix-based key (C-c) with the new 100% size.
const tmuxConfBindingOldKeyNewSize = `bind-key C-c display-popup -E -w100% -h100% "clux"`

const tmuxConfBlock = "\n# clux\n" + tmuxConfBinding + "\n"

const cluxSummaryBlock = `
## clux

After understanding the first task in a session, run ` + "`" + `tmux set-option -p -t "$TMUX_PANE" @clux-summary "<summary>"` + "`" + ` to set a concise task summary (max 30 chars) as the pane's clux-summary metadata. When the topic or task direction changes, re-run the command with an updated summary.
`

const hookCommand = `tmux set-window-option @claude-status waiting 2>/dev/null || true`
const hookMatcher = "AskUserQuestion"

type setupResult int

const (
	setupSuccess setupResult = iota
	setupSkipped
	setupError
)

func cmdSetup() {
	fmt.Println("Setting up clux...")
	fmt.Println()

	r1 := setupPostToolUseHook()
	r2 := setupClaudeMD()
	r3 := setupTmuxConf()

	fmt.Println()

	if r1 == setupError || r2 == setupError || r3 == setupError {
		fmt.Println("Setup finished with errors.")
		os.Exit(1)
	}
	fmt.Println("Setup complete! Restart Claude Code sessions to apply.")
}

func setupPostToolUseHook() setupResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to determine home directory: %v\n", err)
		return setupError
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	return setupPostToolUseHookAt(settingsPath)
}

func setupPostToolUseHookAt(settingsPath string) setupResult {
	var settings map[string]any

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "[✗] Failed to read %s: %v\n", settingsPath, err)
			return setupError
		}
		settings = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			fmt.Fprintf(os.Stderr, "[✗] Failed to parse %s: %v\n", settingsPath, err)
			return setupError
		}
	}

	// Navigate/create: hooks -> PostToolUse
	hooks, ok := settings["hooks"]
	if !ok {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		fmt.Fprintf(os.Stderr, "[✗] hooks key in %s is not an object\n", settingsPath)
		return setupError
	}

	postToolUse, ok := hooksMap["PostToolUse"]
	if !ok {
		postToolUse = make([]any, 0)
		hooksMap["PostToolUse"] = postToolUse
	}
	postToolUseArr, ok := postToolUse.([]any)
	if !ok {
		fmt.Fprintf(os.Stderr, "[✗] hooks.PostToolUse in %s is not an array\n", settingsPath)
		return setupError
	}

	// Check if already configured
	if isHookConfigured(postToolUseArr, hookMatcher, hookCommand) {
		fmt.Printf("[-] PostToolUse hook already configured in %s\n", settingsPath)
		return setupSkipped
	}

	// Build the new hook entry
	newHookEntry := map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookCommand,
			},
		},
	}

	hooksMap["PostToolUse"] = append(postToolUseArr, newHookEntry)

	// Write back
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to marshal settings: %v\n", err)
		return setupError
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to create directory: %v\n", err)
		return setupError
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to write %s: %v\n", settingsPath, err)
		return setupError
	}

	fmt.Printf("[✓] PostToolUse hook added to %s\n", settingsPath)
	return setupSuccess
}

func setupClaudeMD() setupResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to determine home directory: %v\n", err)
		return setupError
	}
	return setupClaudeMDAt(filepath.Join(homeDir, ".claude", "CLAUDE.md"))
}

func setupClaudeMDAt(path string) setupResult {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "[✗] Failed to read %s: %v\n", path, err)
			return setupError
		}
		// File doesn't exist — create it
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "[✗] Failed to create directory for %s: %v\n", path, err)
			return setupError
		}
		if err := os.WriteFile(path, []byte(strings.TrimLeft(cluxSummaryBlock, "\n")), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[✗] Failed to create %s: %v\n", path, err)
			return setupError
		}
		fmt.Printf("[✓] @clux-summary instruction added to %s\n", path)
		return setupSuccess
	}

	if strings.Contains(string(data), "@clux-summary") {
		fmt.Printf("[-] @clux-summary instruction already configured in %s\n", path)
		return setupSkipped
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to open %s for appending: %v\n", path, err)
		return setupError
	}
	if _, err := f.WriteString(cluxSummaryBlock); err != nil {
		_ = f.Close()
		fmt.Fprintf(os.Stderr, "[✗] Failed to append to %s: %v\n", path, err)
		return setupError
	}

	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to close %s: %v\n", path, err)
		return setupError
	}

	fmt.Printf("[✓] @clux-summary instruction added to %s\n", path)
	return setupSuccess
}

func setupTmuxConf() setupResult {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to determine home directory: %v\n", err)
		return setupError
	}
	return setupTmuxConfAt(filepath.Join(homeDir, ".tmux.conf"))
}

func setupTmuxConfAt(path string) setupResult {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "[✗] Failed to read %s: %v\n", path, err)
			return setupError
		}
		if err := os.WriteFile(path, []byte(strings.TrimLeft(tmuxConfBlock, "\n")), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[✗] Failed to create %s: %v\n", path, err)
			return setupError
		}
		fmt.Printf("[✓] tmux key binding added to %s\n", path)
		return setupSuccess
	}

	if strings.Contains(string(data), tmuxConfBinding) {
		fmt.Printf("[-] tmux key binding already configured in %s\n", path)
		return setupSkipped
	}

	// Migrate any old variant to the current binding.
	content := string(data)
	for _, old := range []string{tmuxConfBindingOld, tmuxConfBindingOldNewKey, tmuxConfBindingOldKeyNewSize} {
		if strings.Contains(content, old) {
			updated := strings.ReplaceAll(content, old, tmuxConfBinding)
			if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "[✗] Failed to write %s: %v\n", path, err)
				return setupError
			}
			fmt.Printf("[✓] tmux key binding updated in %s\n", path)
			return setupSuccess
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to open %s for appending: %v\n", path, err)
		return setupError
	}
	if _, err := f.WriteString(tmuxConfBlock); err != nil {
		_ = f.Close()
		fmt.Fprintf(os.Stderr, "[✗] Failed to append to %s: %v\n", path, err)
		return setupError
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[✗] Failed to close %s: %v\n", path, err)
		return setupError
	}

	fmt.Printf("[✓] tmux key binding added to %s\n", path)
	return setupSuccess
}

func isHookConfigured(postToolUseArr []any, matcher, command string) bool {
	for _, entry := range postToolUseArr {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		m, _ := entryMap["matcher"].(string)
		if m != matcher {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if hMap["type"] == "command" && hMap["command"] == command {
				return true
			}
		}
	}
	return false
}
