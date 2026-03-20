package tmux

import (
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

// --- hasClaudeCode ---

func TestHasClaudeCode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"contains INSERT indicator", "some text\n-- INSERT --\nmore text", true},
		{"contains Do you want to proceed", "Please confirm\nDo you want to proceed?\n", true},
		{"contains Esc to cancel", "running...\nEsc to cancel\n", true},
		{"no indicators", "just some regular shell output", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasClaudeCode(tt.content)
			if got != tt.want {
				t.Errorf("hasClaudeCode(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
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
	f.Close()
	defer os.Remove(f.Name())

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

func TestScanPanes_EmptySessionName(t *testing.T) {
	result := ScanPanes("", "0")
	if result != nil {
		t.Error("expected nil for empty session name")
	}
}

func TestScanPanes_InvalidWindowIndex(t *testing.T) {
	result := ScanPanes("test-session", "abc")
	if result != nil {
		t.Error("expected nil for invalid window index")
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

// --- parseScanPanesOutput ---

func TestParseScanPanesOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		windowIndex string
		want        int
	}{
		{
			"normal output",
			"0\teditor\t/home\n1\tshell\t/tmp\n",
			"5",
			2,
		},
		{
			"empty output",
			"",
			"0",
			0,
		},
		{
			"invalid pane index skipped",
			"abc\teditor\t/home\n0\tshell\t/tmp\n",
			"0",
			1,
		},
		{
			"malformed line skipped",
			"0\teditor\n1\tshell\t/tmp\n",
			"0",
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseScanPanesOutput(tt.output, tt.windowIndex)
			if len(got) != tt.want {
				t.Errorf("parseScanPanesOutput() returned %d items, want %d", len(got), tt.want)
			}
			// All results should have the provided windowIndex
			for _, w := range got {
				if w.index != tt.windowIndex {
					t.Errorf("windowIndex = %q, want %q", w.index, tt.windowIndex)
				}
			}
		})
	}
}

// --- parseListAllWindowsOutput ---

func TestParseListAllWindowsOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			"normal output excludes clux session",
			"main\t0\teditor\t/home\nclux\t1\tshell\t/tmp\nwork\t2\tdev\t/src\n",
			2,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"only clux session",
			"clux\t0\teditor\t/home\nclux\t1\tshell\t/tmp\n",
			0,
		},
		{
			"invalid window index skipped",
			"main\tabc\teditor\t/home\nmain\t1\tshell\t/tmp\n",
			1,
		},
		{
			"valid line with malformed line",
			"main\t0\teditor\t/home\nwork\t1\tshell\t/tmp\n",
			2,
		},
		{
			"all malformed lines skipped",
			"main\t0\n1\tshell\t/tmp\n",
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListAllWindowsOutput(tt.output)
			if len(got) != tt.want {
				t.Errorf("parseListAllWindowsOutput() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseListAllWindowsOutput_FieldValues(t *testing.T) {
	output := "dev-session\t3\tmy-editor\t/home/user/code\n"
	got := parseListAllWindowsOutput(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	w := got[0]
	if w.Session != "dev-session" {
		t.Errorf("Session = %q, want %q", w.Session, "dev-session")
	}
	if w.WindowIndex != "3" {
		t.Errorf("WindowIndex = %q, want %q", w.WindowIndex, "3")
	}
	if w.WindowName != "my-editor" {
		t.Errorf("WindowName = %q, want %q", w.WindowName, "my-editor")
	}
	if w.Dir != "/home/user/code" {
		t.Errorf("Dir = %q, want %q", w.Dir, "/home/user/code")
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
	paneContentHashesMu.Lock()
	orig := paneContentHashes
	paneContentHashes = map[string]uint64{}
	paneContentHashesMu.Unlock()
	t.Cleanup(func() {
		paneContentHashesMu.Lock()
		paneContentHashes = orig
		paneContentHashesMu.Unlock()
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
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hash stable + waiting pattern -> Waiting
func TestDetect_HashStable_WaitingPattern(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nDo you want to proceed?\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hash stable + active children -> Working (safety net)
func TestDetect_HashStable_ActiveChildren_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return true },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hash stable + no waiting + no children -> Idle
func TestDetect_HashStable_NoWaiting_NoChildren_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle, got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hook says "waiting" -> Waiting immediately
func TestDetect_HookWaiting_Immediate(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string) bool { t.Error("should not be called"); return false },
		func(_ string, _ string) bool { t.Error("should not be called"); return false },
	)
	content := "-- INSERT --\nsome output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hook says "idle" -> ignored, hash takes priority
func TestDetect_HookIdle_Ignored_HashChanged(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "idle" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return true },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusWorking, "expected Working (hash changed despite hook idle), got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Not Claude Code -> Unknown
func TestDetect_NotClaudeCode(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "regular shell output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusUnknown, "expected Unknown, got %v", st)
	assert(t, !isCC, "expected isClaudeCode=false")
}

// Hook says "working" -> not trusted, falls through to hash
func TestDetect_HookWorking_NotTrusted_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "working" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (hook working not trusted, hash stable), got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}

// Hash stable + no waiting pattern + no children + Claude Code present -> Idle (not Unknown)
func TestDetect_HashStable_ClaudeCodeNoPattern_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
		func(_ string, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome unrecognized output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (hash stable, Claude Code present), got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
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
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	assert(t, st == session.StatusIdle, "expected Idle (old waiting in scrollback), got %v", st)
	assert(t, isCC, "expected isClaudeCode=true")
}
