package tmux

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tanaka0325/clux/internal/session"
)

// assert is a test helper that fails with a formatted message if the condition is false.
func assert(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Errorf(format, args...)
	}
}

// --- isWaiting ---

func TestIsWaiting(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"contains [Y/n]", "Delete file? [Y/n]", true},
		{"contains [y/n]", "Overwrite? [y/n]", true},
		{"contains [y/N]", "Continue? [y/N]", true},
		{"contains Do you want to proceed?", "Do you want to proceed?", true},
		{"contains (y/n)", "Proceed? (y/n)", true},
		{"contains (Y/n)", "Proceed? (Y/n)", true},
		{"contains (y)es / (n)o", "Choose: (y)es / (n)o", true},
		{"contains ? (yes/no)", "Overwrite? (yes/no)", true},
		{"Esc to cancel is not a waiting indicator", "Press Esc to cancel the operation", false},
		{"no waiting prompt", "-- INSERT --\nnormal output", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWaiting(tt.content)
			if got != tt.want {
				t.Errorf("isWaiting(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// --- bottomContent ---

func TestBottomContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{"fewer lines than n", "a\nb\nc", 5, "a\nb\nc"},
		{"exact lines", "a\nb\nc", 3, "a\nb\nc"},
		{"more lines than n", "a\nb\nc\nd\ne", 3, "c\nd\ne"},
		{"empty content", "", 5, ""},
		{"trailing blank lines trimmed", "a\nb\nc\n\n\n\n", 3, "a\nb\nc"},
		{"prompt followed by padding", "line1\nline2\nDo you want to proceed?\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n", 3, "line1\nline2\nDo you want to proceed?"},
		{"all blank lines", "\n\n\n\n", 3, ""},
		{"trailing whitespace-only lines", "a\nb\n   \n  \t\n", 3, "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bottomContent(tt.content, tt.n)
			if got != tt.want {
				t.Errorf("bottomContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- ValidateDir ---

func TestValidateDir_ValidDir(t *testing.T) {
	err := ValidateDir(os.TempDir())
	if err != nil {
		t.Errorf("expected nil error for valid dir %q, got %v", os.TempDir(), err)
	}
}

func TestValidateDir_NonExistentDir(t *testing.T) {
	err := ValidateDir("/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for non-existent dir, got nil")
	}
}

func TestValidateDir_FileNotDir(t *testing.T) {
	// Create a temporary file.
	f, err := os.CreateTemp("", "tmux_test_file_*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	err = ValidateDir(f.Name())
	if err == nil {
		t.Errorf("expected error when passing a file path %q, got nil", f.Name())
	}
}

// --- CheckTmux ---

func TestCheckTmux_WithTMUX(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	if err := CheckTmux(); err != nil {
		t.Errorf("expected no error with TMUX set, got %v", err)
	}
}

func TestCheckTmux_WithoutTMUX(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := CheckTmux(); err == nil {
		t.Error("expected error without TMUX, got nil")
	}
}

// --- validWindowIndex ---

func TestValidWindowIndex(t *testing.T) {
	matching := []string{"0", "1", "123"}
	for _, s := range matching {
		if !validWindowIndex.MatchString(s) {
			t.Errorf("expected %q to match validWindowIndex, but it did not", s)
		}
	}

	notMatching := []string{"abc", "1.2", "", "1\t2"}
	for _, s := range notMatching {
		if validWindowIndex.MatchString(s) {
			t.Errorf("expected %q to NOT match validWindowIndex, but it did", s)
		}
	}
}

// --- sanitizeWindowName ---

func TestSanitizeWindowName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal-name", "normal-name"},
		{"has:colon", "has_colon"},
		{"has.dot", "has.dot"},
		{"has space", "has_space"},
		{"foo:bar.baz", "foo_bar.baz"},
		{"", "window"},
		{"...", "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeWindowName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeWindowName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- parseClaudeStatus ---

func TestParseClaudeStatus(t *testing.T) {
	tests := []struct {
		input  string
		wantSt session.Status
		wantOK bool
	}{
		{"working", session.StatusWorking, true},
		{"idle", session.StatusIdle, true},
		{"waiting", session.StatusWaiting, true},
		{"", session.StatusUnknown, false},
		{"unknown", session.StatusUnknown, false},
		{"WORKING", session.StatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			st, ok := parseClaudeStatus(tt.input)
			if st != tt.wantSt || ok != tt.wantOK {
				t.Errorf("parseClaudeStatus(%q) = (%v, %v), want (%v, %v)", tt.input, st, ok, tt.wantSt, tt.wantOK)
			}
		})
	}
}

// --- SwitchWindow/KillWindow invalid index tests ---

func TestSwitchWindow_InvalidIndex(t *testing.T) {
	err := SwitchWindow("abc", "0")
	if err == nil {
		t.Error("expected error for invalid window index")
	}
}

func TestKillWindow_InvalidIndex(t *testing.T) {
	err := KillWindow("")
	if err == nil {
		t.Error("expected error for invalid window index")
	}
}

func TestSwitchWindow_InvalidPaneIndex(t *testing.T) {
	err := SwitchWindow("0", "abc")
	if err == nil {
		t.Error("expected error for invalid pane index")
	}
}

// --- validatePaneTarget ---

func TestValidatePaneTarget(t *testing.T) {
	tests := []struct {
		name        string
		windowIndex string
		paneIndex   string
		wantErr     bool
	}{
		{"both valid", "0", "1", false},
		{"invalid windowIndex", "abc", "0", true},
		{"invalid paneIndex", "0", "abc", true},
		{"both invalid", "abc", "xyz", true},
		{"empty windowIndex", "", "0", true},
		{"empty paneIndex", "0", "", true},
		{"both empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePaneTarget(tt.windowIndex, tt.paneIndex)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePaneTarget(%q, %q) error = %v, wantErr %v", tt.windowIndex, tt.paneIndex, err, tt.wantErr)
			}
		})
	}
}

// --- groupedSessionName ---

func TestGroupedSessionName(t *testing.T) {
	name := groupedSessionName()
	if !strings.HasPrefix(name, "clux-") {
		t.Errorf("groupedSessionName() = %q, want prefix 'clux-'", name)
	}
	// Verify the suffix is a numeric timestamp.
	suffix := strings.TrimPrefix(name, "clux-")
	if suffix == "" {
		t.Error("groupedSessionName() has empty suffix")
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			t.Errorf("groupedSessionName() suffix %q contains non-digit %q", suffix, string(r))
			break
		}
	}
}

// --- SendKeys validation ---

func TestSendKeys_InvalidWindowIndex(t *testing.T) {
	if err := SendKeys("sess", "abc", "0", "keys"); err == nil {
		t.Error("expected error for invalid windowIndex")
	}
}

func TestSendKeys_InvalidPaneIndex(t *testing.T) {
	if err := SendKeys("sess", "0", "abc", "keys"); err == nil {
		t.Error("expected error for invalid paneIndex")
	}
}

// --- SendKeysLiteral validation ---

func TestSendKeysLiteral_InvalidWindowIndex(t *testing.T) {
	if err := SendKeysLiteral("sess", "abc", "0", "text"); err == nil {
		t.Error("expected error for invalid windowIndex")
	}
}

func TestSendKeysLiteral_InvalidPaneIndex(t *testing.T) {
	if err := SendKeysLiteral("sess", "0", "abc", "text"); err == nil {
		t.Error("expected error for invalid paneIndex")
	}
}

// --- SwitchToWindow validation ---

func TestSwitchToWindow_InvalidWindowIndex(t *testing.T) {
	err := SwitchToWindow("sess", "abc", "0")
	if err == nil {
		t.Error("expected error for invalid window index")
	}
}

func TestSwitchToWindow_InvalidPaneIndex(t *testing.T) {
	err := SwitchToWindow("sess", "0", "abc")
	if err == nil {
		t.Error("expected error for invalid pane index")
	}
}

// --- CapturePaneForSession validation ---

func TestCapturePaneForSession_InvalidWindowIndex(t *testing.T) {
	_, err := CapturePaneForSession("sess", "abc", "0")
	if err == nil {
		t.Error("expected error for invalid window index")
	}
}

func TestCapturePaneForSession_InvalidPaneIndex(t *testing.T) {
	_, err := CapturePaneForSession("sess", "0", "abc")
	if err == nil {
		t.Error("expected error for invalid pane index")
	}
}

// --- parseListPanesOutput ---

func TestParseListPanesOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int // expected count
	}{
		{
			"normal output",
			"0\t0\teditor\t/home/user\n1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"with multiple panes",
			"0\t0\teditor\t/home\n0\t1\teditor\t/home\n1\t0\tshell\t/tmp\n",
			3,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"malformed line skipped",
			"0\t0\teditor\t/home\nbadline\n1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"invalid window index skipped",
			"abc\t0\teditor\t/home\n1\t0\tshell\t/tmp\n",
			1,
		},
		{
			"invalid pane index skipped",
			"0\tabc\teditor\t/home\n1\t0\tshell\t/tmp\n",
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListPanesOutput(tt.output)
			if len(got) != tt.want {
				t.Errorf("parseListPanesOutput() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseListPanesOutput_FieldValues(t *testing.T) {
	output := "3\t1\tmy-window\t/home/user/project\n"
	got := parseListPanesOutput(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	w := got[0]
	if w.index != "3" {
		t.Errorf("index = %q, want %q", w.index, "3")
	}
	if w.paneIndex != "1" {
		t.Errorf("paneIndex = %q, want %q", w.paneIndex, "1")
	}
	if w.name != "my-window" {
		t.Errorf("name = %q, want %q", w.name, "my-window")
	}
	if w.dir != "/home/user/project" {
		t.Errorf("dir = %q, want %q", w.dir, "/home/user/project")
	}
}

// --- deduplicateWindowName ---

func TestDeduplicateWindowName(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		nameSet map[string]bool
		want    string
	}{
		{
			"no conflict",
			"myrepo",
			map[string]bool{"other": true},
			"myrepo",
		},
		{
			"one conflict",
			"myrepo",
			map[string]bool{"myrepo": true},
			"myrepo-2",
		},
		{
			"two conflicts",
			"myrepo",
			map[string]bool{"myrepo": true, "myrepo-2": true},
			"myrepo-3",
		},
		{
			"empty nameSet",
			"myrepo",
			map[string]bool{},
			"myrepo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateWindowName(tt.base, tt.nameSet)
			if got != tt.want {
				t.Errorf("deduplicateWindowName(%q, ...) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// --- contentChanged ---

func resetPaneHashes(t *testing.T) {
	t.Helper()
	paneCacheMu.Lock()
	orig := paneContentHashes
	paneContentHashes = map[string]uint64{}
	origDetect := paneDetectCache
	paneDetectCache = map[string]paneDetectResult{}
	paneCacheMu.Unlock()
	t.Cleanup(func() {
		paneCacheMu.Lock()
		paneContentHashes = orig
		paneDetectCache = origDetect
		paneCacheMu.Unlock()
	})
}

func TestContentChanged(t *testing.T) {
	resetPaneHashes(t)

	key := "test:0.0"

	// First call — no previous hash, returns false
	if contentChanged(key, "hello") {
		t.Error("first call should return false")
	}

	// Same content — returns false
	if contentChanged(key, "hello") {
		t.Error("same content should return false")
	}

	// Different content — returns true
	if !contentChanged(key, "world") {
		t.Error("different content should return true")
	}

	// Same new content — returns false
	if contentChanged(key, "world") {
		t.Error("same content should return false")
	}
}

func TestClearPaneHash(t *testing.T) {
	resetPaneHashes(t)

	key := paneKey("test", "0", "0")
	contentChanged(key, "hello")
	ClearPaneHash("test", "0", "0")

	// After clear, first call returns false again
	if contentChanged(key, "hello") {
		t.Error("after clear, first call should return false")
	}

	// Clearing a non-existent key is a no-op (no panic).
	ClearPaneHash("nonexistent", "99", "99")
}

// --- detectStatusWithHooksForSession (mocked) ---

// withMockedDeps sets up mocked dependencies for a test,
// restoring the originals when the test completes.
func withMockedDeps(t *testing.T,
	statusFn func(string, string, string) string,
	childrenFn func(string, string, string) bool,
	hashChangedFn func(string, string) bool,
) {
	t.Helper()
	origStatus := getClaudeStatusFn
	origChildren := hasActiveChildrenFn
	origHash := contentChangedFn
	getClaudeStatusFn = statusFn
	hasActiveChildrenFn = childrenFn
	contentChangedFn = hashChangedFn
	t.Cleanup(func() {
		getClaudeStatusFn = origStatus
		hasActiveChildrenFn = origChildren
		contentChangedFn = origHash
	})
}

// Hash changed -> Working (regardless of content patterns)
func TestDetect_HashChanged_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return true },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
}

// Hash stable + waiting pattern -> Waiting
func TestDetect_HashStable_WaitingPattern(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nDo you want to proceed?\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
}

// Hash stable + active children -> Working (safety net)
func TestDetect_HashStable_ActiveChildren_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return true },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
}

// Hash stable + no waiting + no children -> Idle
func TestDetect_HashStable_NoWaiting_NoChildren_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle, got %v", st)
}

// Hook says "waiting", hash stable, no active children -> Waiting
func TestDetect_HookWaiting_HashStable_NoChildren(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
}

// Hook says "waiting" but active children exist -> Working (agents running)
func TestDetect_HookWaiting_ActiveChildren_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string) bool { return true },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (active children override hook waiting), got %v", st)
}

// Hash stable + "local agent still running" in body -> Working
func TestDetect_BackgroundAgent_StillRunning_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n✻ Cooked for 36s · 1 local agent still running\n❯ \n  [Opus 4.6 (1M context)]\n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (agent still running in body), got %v", st)
}

// Hash stable + "local agents still running" (plural) -> Working
func TestDetect_BackgroundAgents_Plural_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n✻ Cooked for 20s · 2 local agents still running\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (agents plural still running), got %v", st)
}

// Hash stable + "esc to interrupt" -> Working (tool execution)
func TestDetect_EscToInterrupt_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n❯ \n  [Opus 4.6 (1M context)]\n  ⏺ Bash(git status) esc to interrupt"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (esc to interrupt), got %v", st)
}

// Hash stable + spinner activity line -> Working
func TestDetect_SpinnerActivity_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n⏺ Reading file…\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (spinner activity), got %v", st)
}

// Hash stable + agent still running + hook=waiting -> Working (agent overrides hook)
func TestDetect_BackgroundAgent_OverridesHookWaiting(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n✻ Baked for 10s · 1 local agent still running\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (agent overrides hook waiting), got %v", st)
}

// "local agent" in scrollback (beyond bottomScanLines) -> Idle, not false positive
func TestDetect_OldLocalAgentInScrollback_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "1 local agent still running\n" + strings.Repeat("filler line\n", 20) + "some output\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (old agent text in scrollback), got %v", st)
}

// Truncated tool output line "⏺ Bash(long command…" must NOT trigger spinner detection
func TestDetect_TruncatedToolOutput_NotSpinner_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "some output\n⏺ Bash(git add internal/tmux/tmux.go internal/tmux/tmux_t…\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (truncated tool output, not spinner), got %v", st)
}

// Hook says "idle" -> ignored, hash takes priority
func TestDetect_HookIdle_Ignored_HashChanged(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "idle" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return true },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (hash changed despite hook idle), got %v", st)
}

// Hook says "working" -> not trusted, falls through to hash
func TestDetect_HookWorking_NotTrusted_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "working" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (hook working not trusted, hash stable), got %v", st)
}

// Hash stable + no waiting pattern + no children + Claude Code present -> Idle (not Unknown)
func TestDetect_HashStable_ClaudeCodeNoPattern_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome unrecognized output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (hash stable, Claude Code present), got %v", st)
}

// Old waiting indicator in scrollback does not affect hash-based detection
func TestDetect_OldWaitingInScrollback_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	// "Do you want to proceed?" is in scrollback (more than 15 lines up),
	// so bottomContent won't include it.
	content := "-- INSERT --\nDo you want to proceed?\n" + strings.Repeat("filler line\n", 20) + "some output\n"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (old waiting in scrollback), got %v", st)
}

// --- CapturePaneForSessionWithOffset validation ---

func TestCapturePaneForSessionWithOffset_InvalidWindowIndex(t *testing.T) {
	_, err := CapturePaneForSessionWithOffset("clux", "abc", "0", 0, 10)
	if err == nil {
		t.Error("expected error for invalid window index, got nil")
	}
}

func TestCapturePaneForSessionWithOffset_InvalidPaneIndex(t *testing.T) {
	_, err := CapturePaneForSessionWithOffset("clux", "0", "xyz", 0, 10)
	if err == nil {
		t.Error("expected error for invalid pane index, got nil")
	}
}

func TestCapturePaneForSessionWithOffset_EmptyWindowIndex(t *testing.T) {
	_, err := CapturePaneForSessionWithOffset("clux", "", "0", 5, 10)
	if err == nil {
		t.Error("expected error for empty window index, got nil")
	}
}

func TestCapturePaneForSessionWithOffset_EmptyPaneIndex(t *testing.T) {
	_, err := CapturePaneForSessionWithOffset("clux", "0", "", 5, 10)
	if err == nil {
		t.Error("expected error for empty pane index, got nil")
	}
}

func TestCapturePaneForSessionWithOffset_ZeroOffsetValidation(t *testing.T) {
	// With offset=0 and valid indices, validation should pass (tmux call will fail without tmux running, that's fine)
	// We just test that the error is NOT a validation error (it would be a tmux exec error)
	_, err := CapturePaneForSessionWithOffset("clux", "0", "0", 0, 10)
	if err != nil {
		// Error is expected (no real tmux session) but should not be a validation error
		if err.Error() == `invalid window index "0"` || err.Error() == `invalid pane index "0"` {
			t.Errorf("unexpected validation error with valid indices: %v", err)
		}
	}
}

// --- contentHashUnchanged ---

func TestContentHashUnchanged(t *testing.T) {
	resetPaneHashes(t)

	key := "test:0.0"

	// No stored hash — returns false
	if contentHashUnchanged(key, "hello") {
		t.Error("expected false when no stored hash exists")
	}

	// Store a hash via contentChanged
	contentChanged(key, "hello")

	// Same content — returns true
	if !contentHashUnchanged(key, "hello") {
		t.Error("expected true for unchanged content")
	}

	// Different content — returns false
	if contentHashUnchanged(key, "world") {
		t.Error("expected false for changed content")
	}

	// contentHashUnchanged does not update the stored hash,
	// so checking the original content still matches
	if !contentHashUnchanged(key, "hello") {
		t.Error("expected true: contentHashUnchanged should not update stored hash")
	}
}

// --- paneDetectCache ---

func TestGetSetCachedResult(t *testing.T) {
	resetPaneHashes(t)

	key := "test:1.0"

	// No cached result initially
	_, ok := getCachedResult(key)
	if ok {
		t.Error("expected no cached result for new key")
	}

	// Set and retrieve
	expected := paneDetectResult{
		status:  session.StatusWorking,
		branch:  "main",
		summary: "test summary",
	}
	setCachedResult(key, expected)

	got, ok := getCachedResult(key)
	if !ok {
		t.Fatal("expected cached result to exist")
	}
	if got.status != expected.status {
		t.Errorf("status = %v, want %v", got.status, expected.status)
	}
	if got.branch != expected.branch {
		t.Errorf("branch = %q, want %q", got.branch, expected.branch)
	}
	if got.summary != expected.summary {
		t.Errorf("summary = %q, want %q", got.summary, expected.summary)
	}
}

// --- ClearAllPaneCache ---

func TestClearAllPaneCache(t *testing.T) {
	resetPaneHashes(t)

	// Populate both caches
	contentChanged("a:0.0", "content-a")
	contentChanged("b:1.0", "content-b")
	setCachedResult("a:0.0", paneDetectResult{status: session.StatusWorking})
	setCachedResult("b:1.0", paneDetectResult{status: session.StatusIdle})

	ClearAllPaneCache()

	// Hash cache should be empty
	if contentHashUnchanged("a:0.0", "content-a") {
		t.Error("expected hash cache to be cleared for a:0.0")
	}
	if contentHashUnchanged("b:1.0", "content-b") {
		t.Error("expected hash cache to be cleared for b:1.0")
	}

	// Detect cache should be empty
	if _, ok := getCachedResult("a:0.0"); ok {
		t.Error("expected detect cache to be cleared for a:0.0")
	}
	if _, ok := getCachedResult("b:1.0"); ok {
		t.Error("expected detect cache to be cleared for b:1.0")
	}
}

// --- ClearPaneHash clears detect cache ---

func TestClearPaneHash_AlsoClearsDetectCache(t *testing.T) {
	resetPaneHashes(t)

	contentChanged("sess:0.0", "content")
	setCachedResult("sess:0.0", paneDetectResult{status: session.StatusIdle, branch: "main"})

	ClearPaneHash("sess", "0", "0")

	if _, ok := getCachedResult("sess:0.0"); ok {
		t.Error("expected detect cache to be cleared after ClearPaneHash")
	}
}

// --- clearPaneHashByPrefix clears detect cache ---

func TestClearPaneHashByPrefix_AlsoClearsDetectCache(t *testing.T) {
	resetPaneHashes(t)

	contentChanged("clux:5.0", "content1")
	contentChanged("clux:5.1", "content2")
	contentChanged("clux:6.0", "content3")
	setCachedResult("clux:5.0", paneDetectResult{status: session.StatusWorking})
	setCachedResult("clux:5.1", paneDetectResult{status: session.StatusIdle})
	setCachedResult("clux:6.0", paneDetectResult{status: session.StatusWaiting})

	clearPaneHashByPrefix("clux:5.")

	// Entries with prefix "clux:5." should be cleared
	if _, ok := getCachedResult("clux:5.0"); ok {
		t.Error("expected clux:5.0 to be cleared")
	}
	if _, ok := getCachedResult("clux:5.1"); ok {
		t.Error("expected clux:5.1 to be cleared")
	}

	// Entry with different prefix should remain
	if _, ok := getCachedResult("clux:6.0"); !ok {
		t.Error("expected clux:6.0 to remain")
	}
}

func TestWindowExists_InvalidIndex(t *testing.T) {
	tests := []struct {
		name  string
		index string
	}{
		{"empty", ""},
		{"letters", "abc"},
		{"special chars", "1;rm"},
		{"negative", "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if WindowExists(tt.index) {
				t.Errorf("WindowExists(%q) = true, want false", tt.index)
			}
		})
	}
}

// --- parseProcessList ---

func TestParseProcessList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			"normal output",
			"  123   1 /usr/bin/bash\n  456 123 /usr/local/bin/claude\n  789 456 /usr/bin/node\n",
			3,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"malformed line skipped",
			"  123   1 /usr/bin/bash\nabc\n  789 456 /usr/bin/node\n",
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProcessList(tt.output)
			if len(got) != tt.want {
				t.Errorf("parseProcessList() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseProcessList_FieldValues(t *testing.T) {
	output := "  456 123 /usr/local/bin/claude\n"
	got := parseProcessList(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	p := got[0]
	if p.pid != 456 {
		t.Errorf("pid = %d, want 456", p.pid)
	}
	if p.ppid != 123 {
		t.Errorf("ppid = %d, want 123", p.ppid)
	}
	if p.comm != "claude" {
		t.Errorf("comm = %q, want %q", p.comm, "claude")
	}
}

func TestParseProcessList_MultiWordComm(t *testing.T) {
	output := "  100   1 /Applications/Self Service.app/Contents/MacOS/Self Service\n  200   1 /usr/local/bin/claude\n"
	got := parseProcessList(output)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].comm != "Self Service" {
		t.Errorf("comm = %q, want %q", got[0].comm, "Self Service")
	}
	if got[1].comm != "claude" {
		t.Errorf("comm = %q, want %q", got[1].comm, "claude")
	}
}

// --- buildProcessMaps ---

func TestBuildProcessMaps(t *testing.T) {
	procs := []processInfo{
		{pid: 1, ppid: 0, comm: "init"},
		{pid: 100, ppid: 1, comm: "bash"},
		{pid: 200, ppid: 100, comm: "claude"},
		{pid: 300, ppid: 200, comm: "node"},
	}
	commByPID, childrenByPID := buildProcessMaps(procs)

	if commByPID[100] != "bash" {
		t.Errorf("commByPID[100] = %q, want %q", commByPID[100], "bash")
	}
	if commByPID[200] != "claude" {
		t.Errorf("commByPID[200] = %q, want %q", commByPID[200], "claude")
	}
	if len(childrenByPID[100]) != 1 || childrenByPID[100][0] != 200 {
		t.Errorf("childrenByPID[100] = %v, want [200]", childrenByPID[100])
	}
}

// --- paneHasClaude ---

func TestPaneHasClaude(t *testing.T) {
	procs := []processInfo{
		{pid: 100, ppid: 1, comm: "bash"},
		{pid: 200, ppid: 100, comm: "claude"},
		{pid: 300, ppid: 1, comm: "vim"},
		{pid: 400, ppid: 300, comm: "node"},
		{pid: 500, ppid: 0, comm: "claude"}, // claude is pane_pid itself
	}
	commByPID, childrenByPID := buildProcessMaps(procs)

	tests := []struct {
		name    string
		panePID int
		want    bool
	}{
		{"child is claude", 100, true},
		{"no claude child", 300, false},
		{"pane_pid is claude", 500, true},
		{"nonexistent pid", 999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneHasClaude(tt.panePID, commByPID, childrenByPID)
			if got != tt.want {
				t.Errorf("paneHasClaude(%d) = %v, want %v", tt.panePID, got, tt.want)
			}
		})
	}
}

// --- parsePaneListOutput ---

func TestParsePaneListOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			"normal output",
			"12345\tdev\t0\t0\teditor\t/home/user\n67890\twork\t1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"malformed line skipped",
			"12345\tdev\t0\t0\teditor\t/home\nbadline\n67890\twork\t1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"invalid window index skipped",
			"12345\tdev\tabc\t0\teditor\t/home\n67890\twork\t1\t0\tshell\t/tmp\n",
			1,
		},
		{
			"invalid pane index skipped",
			"12345\tdev\t0\tabc\teditor\t/home\n67890\twork\t1\t0\tshell\t/tmp\n",
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaneListOutput(tt.output)
			if len(got) != tt.want {
				t.Errorf("parsePaneListOutput() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParsePaneListOutput_FieldValues(t *testing.T) {
	output := "12345\tdev-session\t3\t1\tmy-window\t/home/user/code\n"
	got := parsePaneListOutput(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	p := got[0]
	if p.panePID != 12345 {
		t.Errorf("panePID = %d, want 12345", p.panePID)
	}
	if p.sessionName != "dev-session" {
		t.Errorf("sessionName = %q, want %q", p.sessionName, "dev-session")
	}
	if p.windowIndex != "3" {
		t.Errorf("windowIndex = %q, want %q", p.windowIndex, "3")
	}
	if p.paneIndex != "1" {
		t.Errorf("paneIndex = %q, want %q", p.paneIndex, "1")
	}
	if p.windowName != "my-window" {
		t.Errorf("windowName = %q, want %q", p.windowName, "my-window")
	}
	if p.dir != "/home/user/code" {
		t.Errorf("dir = %q, want %q", p.dir, "/home/user/code")
	}
}

// --- isAncestorOf ---

func TestIsAncestorOf(t *testing.T) {
	// Process tree: 1 -> 100 -> 200 -> 300
	childrenByPID := map[int][]int{
		1:   {100},
		100: {200},
		200: {300},
	}

	tests := []struct {
		name      string
		ancestor  int
		target    int
		want      bool
	}{
		{"direct child", 100, 200, true},
		{"grandchild", 100, 300, true},
		{"not ancestor", 200, 100, false},
		{"same pid", 100, 100, false},
		{"nonexistent", 999, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAncestorOf(tt.ancestor, tt.target, childrenByPID)
			if got != tt.want {
				t.Errorf("isAncestorOf(%d, %d) = %v, want %v", tt.ancestor, tt.target, got, tt.want)
			}
		})
	}
}

// --- findClaudePanes integration ---

func TestFindClaudePanes(t *testing.T) {
	myPID := os.Getpid()

	origPanes := listAllPanesFn
	origProcs := listProcessesFn
	t.Cleanup(func() {
		listAllPanesFn = origPanes
		listProcessesFn = origProcs
	})

	// Pane layout:
	//   pane 1000: shell with claude child → should be found
	//   pane 2000: claude as root process → should be found
	//   pane 3000: no claude → should be excluded
	//   pane myPID: clux itself → should be excluded (self-exclusion)
	listAllPanesFn = func() (string, error) {
		lines := strings.Join([]string{
			"1000\twork\t0\t0\tdev\t/home/user/project",
			"2000\tother\t1\t0\teditor\t/tmp/editor",
			"3000\twork\t2\t0\tshell\t/home/user",
			fmt.Sprintf("%d\tclux\t0\t0\tclux\t/home/user/clux", myPID),
		}, "\n")
		return lines, nil
	}

	listProcessesFn = func() (string, error) {
		lines := strings.Join([]string{
			fmt.Sprintf("%d   1 zsh", myPID), // clux's own process
			"1000   1 zsh",
			"1001 1000 claude",  // claude is child of pane 1000
			"2000   1 claude",   // claude IS pane 2000
			"3000   1 zsh",
			"3001 3000 vim",     // no claude in pane 3000
		}, "\n")
		return lines, nil
	}

	got, err := findClaudePanes()
	if err != nil {
		t.Fatalf("findClaudePanes() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 panes, got %d: %+v", len(got), got)
	}

	// Verify the two found panes
	sessions := map[string]bool{}
	for _, p := range got {
		sessions[p.sessionName+":"+p.windowIndex] = true
	}
	if !sessions["work:0"] {
		t.Error("expected work:0 (shell with claude child) to be found")
	}
	if !sessions["other:1"] {
		t.Error("expected other:1 (claude as root) to be found")
	}
}
