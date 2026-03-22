package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupPostToolUseHook_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings: %v", err)
	}

	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}
	entry := postToolUse[0].(map[string]any)
	if entry["matcher"] != hookMatcher {
		t.Errorf("expected matcher %q, got %q", hookMatcher, entry["matcher"])
	}
}

func TestSetupPostToolUseHook_ExistingSettingsWithOtherHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	existing := map[string]any{
		"someOtherKey": "value",
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{"matcher": "SomeTool"},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	// Check existing keys preserved
	if settings["someOtherKey"] != "value" {
		t.Error("existing key was not preserved")
	}
	hooks := settings["hooks"].(map[string]any)
	if hooks["PreToolUse"] == nil {
		t.Error("existing PreToolUse hook was not preserved")
	}
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}
}

func TestSetupPostToolUseHook_ExistingPostToolUseWithOtherMatchers(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"matcher": "OtherTool",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "echo other",
						},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 2 {
		t.Fatalf("expected 2 PostToolUse entries, got %d", len(postToolUse))
	}
}

func TestSetupPostToolUseHook_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"matcher": hookMatcher,
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": hookCommand,
						},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupSkipped {
		t.Fatalf("expected setupSkipped, got %d", result)
	}
}

func TestSetupPostToolUseHook_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupError {
		t.Fatalf("expected setupError, got %d", result)
	}
}

func TestSetupPostToolUseHook_HooksNotObject(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"hooks": "string"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupError {
		t.Fatalf("expected setupError, got %d", result)
	}
}

func TestSetupPostToolUseHook_PostToolUseNotArray(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"hooks": {"PostToolUse": "string"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupPostToolUseHookAt(settingsPath)
	if result != setupError {
		t.Fatalf("expected setupError, got %d", result)
	}
}

func TestSetupClaudeMD_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "CLAUDE.md")

	result := setupClaudeMDAt(mdPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(data), "@clux-summary") {
		t.Error("CLAUDE.md does not contain @clux-summary")
	}
	if !strings.Contains(string(data), "## clux") {
		t.Error("CLAUDE.md does not contain ## clux header")
	}
}

func TestSetupClaudeMD_AppendToExisting(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(mdPath, []byte("# My Project\n\nSome existing content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupClaudeMDAt(mdPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "# My Project") {
		t.Error("existing content was not preserved")
	}
	if !strings.Contains(content, "@clux-summary") {
		t.Error("@clux-summary was not appended")
	}
}

func TestSetupClaudeMD_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(mdPath, []byte("# Project\n\nHas @clux-summary already.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupClaudeMDAt(mdPath)
	if result != setupSkipped {
		t.Fatalf("expected setupSkipped, got %d", result)
	}
}

func TestSetupTmuxConf_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	tmuxConfPath := filepath.Join(dir, ".tmux.conf")

	result := setupTmuxConfAt(tmuxConfPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(tmuxConfPath)
	if err != nil {
		t.Fatalf("failed to read .tmux.conf: %v", err)
	}
	if !strings.Contains(string(data), tmuxConfBinding) {
		t.Error(".tmux.conf does not contain tmux key binding")
	}
}

func TestSetupTmuxConf_AppendToExisting(t *testing.T) {
	dir := t.TempDir()
	tmuxConfPath := filepath.Join(dir, ".tmux.conf")
	if err := os.WriteFile(tmuxConfPath, []byte("set -g mouse on\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupTmuxConfAt(tmuxConfPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(tmuxConfPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "set -g mouse on") {
		t.Error("existing content was not preserved")
	}
	if !strings.Contains(content, tmuxConfBinding) {
		t.Error("tmux key binding was not appended")
	}
}

func TestSetupTmuxConf_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	tmuxConfPath := filepath.Join(dir, ".tmux.conf")
	if err := os.WriteFile(tmuxConfPath, []byte("set -g mouse on\n"+tmuxConfBinding+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupTmuxConfAt(tmuxConfPath)
	if result != setupSkipped {
		t.Fatalf("expected setupSkipped, got %d", result)
	}
}

func TestSetupTmuxConf_MigratesOldBinding(t *testing.T) {
	dir := t.TempDir()
	tmuxConfPath := filepath.Join(dir, ".tmux.conf")
	if err := os.WriteFile(tmuxConfPath, []byte("set -g mouse on\n# clux\n"+tmuxConfBindingOld+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := setupTmuxConfAt(tmuxConfPath)
	if result != setupSuccess {
		t.Fatalf("expected setupSuccess, got %d", result)
	}

	data, err := os.ReadFile(tmuxConfPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, tmuxConfBindingOld) {
		t.Error("old binding should have been replaced")
	}
	if !strings.Contains(content, tmuxConfBinding) {
		t.Error("new binding should be present")
	}
	if !strings.HasPrefix(content, "set -g mouse on") {
		t.Error("existing content was not preserved")
	}
}
