package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfig_SaveAndLoad(t *testing.T) {
	// Override the home dir for this test using a temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	expected := filepath.Join(tmpHome, configDir, configFile)
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("config file not created at %q: %v", expected, err)
	}

	// Load and verify contents.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config after load")
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Write invalid JSON to config file.
	dir := filepath.Join(tmpHome, configDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestNotificationConfig_NilReceiver(t *testing.T) {
	var n *NotificationConfig
	if !n.ShouldNotifyWorkingToIdle() {
		t.Error("expected nil receiver WorkingToIdle to default to true")
	}
	if !n.ShouldNotifyWorkingToWaiting() {
		t.Error("expected nil receiver WorkingToWaiting to default to true")
	}
}

func TestNotificationConfig_Defaults(t *testing.T) {
	n := &NotificationConfig{}
	if !n.ShouldNotifyWorkingToIdle() {
		t.Error("expected default WorkingToIdle to be true")
	}
	if !n.ShouldNotifyWorkingToWaiting() {
		t.Error("expected default WorkingToWaiting to be true")
	}
}

func TestNotificationConfig_ExplicitValues(t *testing.T) {
	f := false
	tr := true
	n := &NotificationConfig{WorkingToIdle: &f, WorkingToWaiting: &tr}
	if n.ShouldNotifyWorkingToIdle() {
		t.Error("expected WorkingToIdle=false")
	}
	if !n.ShouldNotifyWorkingToWaiting() {
		t.Error("expected WorkingToWaiting=true")
	}
}

func TestConfig_Notifications_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	f := false
	cfg := &Config{
		Notifications: &NotificationConfig{WorkingToIdle: &f},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Notifications.ShouldNotifyWorkingToIdle() {
		t.Error("expected WorkingToIdle=false after round-trip")
	}
	if !loaded.Notifications.ShouldNotifyWorkingToWaiting() {
		t.Error("expected WorkingToWaiting=true (default) after round-trip")
	}
}

func TestConfig_PreviewDefault_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{PreviewDefault: true}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.PreviewDefault {
		t.Error("expected PreviewDefault=true after round-trip, got false")
	}
}

func TestConfig_GroupDefault_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{GroupDefault: true}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.GroupDefault {
		t.Error("expected GroupDefault=true after round-trip, got false")
	}
}

func TestConfig_GroupDefault_DefaultFalse(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.GroupDefault {
		t.Error("expected GroupDefault=false by default, got true")
	}
}

func TestDefaultKeymap_ReturnsExpectedDefaults(t *testing.T) {
	km := DefaultKeymap()

	tests := []struct {
		name string
		keys []string
	}{
		{"MoveUp", km.MoveUp},
		{"MoveDown", km.MoveDown},
		{"MoveLeft", km.MoveLeft},
		{"MoveRight", km.MoveRight},
		{"Quit", km.Quit},
		{"Filter", km.Filter},
		{"NewSession", km.NewSession},
		{"Kill", km.Kill},
		{"TogglePreview", km.TogglePreview},
		{"ScrollUp", km.ScrollUp},
		{"ScrollDown", km.ScrollDown},
		{"Dashboard", km.Dashboard},
		{"Broadcast", km.Broadcast},
		{"Group", km.Group},
		{"Input", km.Input},
		{"NextPage", km.NextPage},
		{"PrevPage", km.PrevPage},
		{"ToggleSelect", km.ToggleSelect},
	}

	for _, tt := range tests {
		if len(tt.keys) == 0 {
			t.Errorf("DefaultKeymap().%s should not be empty", tt.name)
		}
	}

	// Spot-check specific defaults
	if !slices.Contains(km.MoveUp, "k") || !slices.Contains(km.MoveUp, "up") || !slices.Contains(km.MoveUp, "ctrl+p") {
		t.Errorf("MoveUp defaults missing expected keys, got %v", km.MoveUp)
	}
	if !slices.Contains(km.Quit, "q") {
		t.Errorf("Quit defaults missing 'q', got %v", km.Quit)
	}
	if !slices.Contains(km.ToggleSelect, "space") || !slices.Contains(km.ToggleSelect, " ") {
		t.Errorf("ToggleSelect defaults missing 'space' or ' ', got %v", km.ToggleSelect)
	}
}

func TestApplyDefaults_FillsMissingFields(t *testing.T) {
	km := &KeymapConfig{
		Quit: []string{"x"}, // custom quit key
	}
	km.applyDefaults()

	// Custom value should be preserved
	if !slices.Contains(km.Quit, "x") {
		t.Error("expected custom Quit key 'x' to be preserved")
	}
	if slices.Contains(km.Quit, "q") {
		t.Error("expected default Quit key 'q' NOT to be present when custom is set")
	}

	// Empty fields should get defaults
	if !slices.Contains(km.MoveUp, "k") {
		t.Error("expected MoveUp to get default 'k'")
	}
	if !slices.Contains(km.Dashboard, "d") {
		t.Error("expected Dashboard to get default 'd'")
	}
}

func TestApplyDefaults_PreservesAllSetFields(t *testing.T) {
	km := &KeymapConfig{
		MoveUp:        []string{"w"},
		MoveDown:      []string{"s"},
		MoveLeft:      []string{"a"},
		MoveRight:     []string{"d"},
		Quit:          []string{"x"},
		Filter:        []string{"f"},
		NewSession:    []string{"N"},
		Kill:          []string{"D"},
		TogglePreview: []string{"P"},
		ScrollUp:      []string{"ctrl+b"},
		ScrollDown:    []string{"ctrl+f"},
		Dashboard:     []string{"D"},
		Broadcast:     []string{"B"},
		Group:         []string{"G"},
		Input:         []string{"I"},
		NextPage:      []string{"."},
		PrevPage:      []string{","},
		ToggleSelect:  []string{"tab"},
	}
	km.applyDefaults()

	// All fields should retain custom values
	if km.MoveUp[0] != "w" {
		t.Errorf("expected MoveUp[0]='w', got %q", km.MoveUp[0])
	}
	if km.Quit[0] != "x" {
		t.Errorf("expected Quit[0]='x', got %q", km.Quit[0])
	}
}

func TestContainsKey(t *testing.T) {
	keys := []string{"a", "b", "ctrl+c"}

	if !slices.Contains(keys, "a") {
		t.Error("expected containsKey to find 'a'")
	}
	if !slices.Contains(keys, "ctrl+c") {
		t.Error("expected containsKey to find 'ctrl+c'")
	}
	if slices.Contains(keys, "d") {
		t.Error("expected containsKey NOT to find 'd'")
	}
	if slices.Contains([]string(nil), "a") {
		t.Error("expected slices.Contains to return false for nil slice")
	}
}

func TestIsMethodsMatchKeys(t *testing.T) {
	km := DefaultKeymap()

	if !km.IsMoveUp("k") {
		t.Error("expected IsMoveUp('k') to be true")
	}
	if !km.IsMoveUp("up") {
		t.Error("expected IsMoveUp('up') to be true")
	}
	if km.IsMoveUp("j") {
		t.Error("expected IsMoveUp('j') to be false")
	}

	if !km.IsQuit("q") {
		t.Error("expected IsQuit('q') to be true")
	}
	if km.IsQuit("x") {
		t.Error("expected IsQuit('x') to be false")
	}

	if !km.IsToggleSelect(" ") {
		t.Error("expected IsToggleSelect(' ') to be true")
	}
	if !km.IsToggleSelect("space") {
		t.Error("expected IsToggleSelect('space') to be true")
	}
}

func TestHintDisplay_FormatKeys(t *testing.T) {
	km := DefaultKeymap()

	// MoveUp hint should contain arrow symbol
	hint := km.HintMoveUp()
	if hint == "" {
		t.Error("HintMoveUp should not be empty")
	}
	// Should contain the up arrow for "up" key
	if !strings.Contains(hint, "\u2191") {
		t.Errorf("HintMoveUp should contain up arrow, got %q", hint)
	}

	// Quit hint should just be "q"
	if km.HintQuit() != "q" {
		t.Errorf("HintQuit expected 'q', got %q", km.HintQuit())
	}

	// ToggleSelect should deduplicate "space" and " " into single "Space"
	selectHint := km.HintToggleSelect()
	if selectHint != "Space" {
		t.Errorf("HintToggleSelect expected 'Space', got %q", selectHint)
	}
}

func TestHintNavigate(t *testing.T) {
	km := DefaultKeymap()
	hint := km.HintNavigate()
	if hint == "" {
		t.Error("HintNavigate should not be empty")
	}
	// Should contain up and down arrows
	if !strings.Contains(hint, "\u2191") || !strings.Contains(hint, "\u2193") {
		t.Errorf("HintNavigate should contain up/down arrows, got %q", hint)
	}
}

func TestHintScroll(t *testing.T) {
	km := DefaultKeymap()
	hint := km.HintScroll()
	if hint == "" {
		t.Error("HintScroll should not be empty")
	}
	if !strings.Contains(hint, "ctrl+u") || !strings.Contains(hint, "ctrl+d") {
		t.Errorf("HintScroll should contain ctrl+u/ctrl+d, got %q", hint)
	}
}

func TestFormatKey_SpecialKeys(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"up", "\u2191"},
		{"down", "\u2193"},
		{"left", "\u2190"},
		{"right", "\u2192"},
		{"space", "Space"},
		{" ", "Space"},
		{"enter", "\u21b5"},
		{"q", "q"},
		{"ctrl+u", "ctrl+u"},
	}

	for _, tt := range tests {
		got := formatKey(tt.input)
		if got != tt.expected {
			t.Errorf("formatKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatKeyGroupsMax(t *testing.T) {
	// max=2 limits each group to 2 keys
	got := formatKeyGroupsMax(2, []string{"a", "b", "c", "d"})
	if got != "a/b" {
		t.Errorf("expected 'a/b', got %q", got)
	}

	// max=0 means unlimited
	got = formatKeyGroupsMax(0, []string{"a", "b", "c"})
	if got != "a/b/c" {
		t.Errorf("expected 'a/b/c', got %q", got)
	}

	// multiple groups each capped at 2
	got = formatKeyGroupsMax(2, []string{"a", "b", "c"}, []string{"x", "y", "z"})
	if got != "a/b/x/y" {
		t.Errorf("expected 'a/b/x/y', got %q", got)
	}

	// deduplication: duplicate from group 1 does not count toward group 2 limit
	got = formatKeyGroupsMax(2, []string{"a", "b"}, []string{"a", "c", "d"})
	if got != "a/b/c/d" {
		t.Errorf("expected 'a/b/c/d', got %q", got)
	}
}

func TestFormatKeyGroups_CapsAtTwo(t *testing.T) {
	km := DefaultKeymap()
	// MoveUp has 4 default keys; hint should only show 2
	hint := km.HintMoveUp()
	parts := strings.Split(hint, "/")
	if len(parts) > 2 {
		t.Errorf("HintMoveUp should show at most 2 keys, got %d: %q", len(parts), hint)
	}
}

func TestConfig_Keymaps_LoadDefaults(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Keymaps == nil {
		t.Fatal("expected Keymaps to be initialized")
	}
	if !cfg.Keymaps.IsQuit("q") {
		t.Error("expected default quit key 'q' on fresh load")
	}
}

func TestConfig_Keymaps_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{
		Keymaps: &KeymapConfig{
			Quit: []string{"x"},
		},
	}
	cfg.ensureDefaults()
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !loaded.Keymaps.IsQuit("x") {
		t.Error("expected custom Quit key 'x' after round-trip")
	}
	if loaded.Keymaps.IsQuit("q") {
		t.Error("expected default Quit key 'q' NOT present after round-trip with custom")
	}
	// Other fields should have defaults
	if !loaded.Keymaps.IsMoveUp("k") {
		t.Error("expected default MoveUp 'k' after round-trip")
	}
}


