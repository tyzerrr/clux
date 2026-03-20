package tmux

import (
	"os"
	"strings"
	"testing"

	"github.com/tanaka0325/clux/internal/session"
)

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

// --- isWorking ---

func TestIsWorking(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"contains Fermenting", "✳ Fermenting tokens...", true},
		{"contains Baked", "✻ Baked response", true},
		{"contains Cooked", "✻ Cooked result", true},
		{"contains Churned", "✻ Churned output", true},
		{"contains Worked", "✻ Worked on it", true},
		{"contains record indicator", "⏺ running task", true},
		{"no working indicator", "-- INSERT --\njust idle text", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWorking(tt.content)
			if got != tt.want {
				t.Errorf("isWorking(%q) = %v, want %v", tt.content, got, tt.want)
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

// --- isIdle ---

func TestIsIdle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			"last non-empty line starts with ❯",
			"some output\n❯ ",
			true,
		},
		{
			"last non-empty line is >",
			"some output\n>",
			true,
		},
		{
			"trailing empty lines before ❯ prompt",
			"some output\n❯ \n\n",
			true,
		},
		{
			"trailing empty lines before > prompt",
			"some output\n>\n\n",
			true,
		},
		{
			"idle with separator line and INSERT",
			"some output\n❯ \n────────────────────\n  -- INSERT --\n\n",
			true,
		},
		{
			"no idle prompt",
			"-- INSERT --\nsome text\n✻ Working",
			false,
		},
		{
			"empty string",
			"",
			false,
		},
		{
			"INSERT line followed by empty lines but no prompt",
			"output\n-- INSERT --\n\n",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIdle(tt.content)
			if got != tt.want {
				t.Errorf("isIdle(%q) = %v, want %v", tt.content, got, tt.want)
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
		input    string
		wantSt   session.Status
		wantOK   bool
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

// --- detectStatusFromContent ---

func TestDetectStatus(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantStatus     session.Status
		wantIsClaudeCode bool
	}{
		{
			name:           "not Claude Code returns false",
			content:        "regular shell output without any indicators",
			wantStatus:     session.StatusUnknown,
			wantIsClaudeCode: false,
		},
		{
			name:           "Waiting takes priority over Working",
			content:        "-- INSERT --\n✻ Worked\nDo you want to proceed?",
			wantStatus:     session.StatusWaiting,
			wantIsClaudeCode: true,
		},
		{
			name:           "Waiting takes priority over Idle",
			content:        "-- INSERT --\n❯ \n[Y/n]",
			wantStatus:     session.StatusWaiting,
			wantIsClaudeCode: true,
		},
		{
			name:           "Working when no Waiting",
			content:        "-- INSERT --\n✻ Worked on task",
			wantStatus:     session.StatusWorking,
			wantIsClaudeCode: true,
		},
		{
			name:           "Working takes priority over Idle",
			content:        "-- INSERT --\n✻ Worked\n❯ ",
			wantStatus:     session.StatusWorking,
			wantIsClaudeCode: true,
		},
		{
			name:           "Idle when only idle prompt",
			content:        "-- INSERT --\nsome text\n❯ ",
			wantStatus:     session.StatusIdle,
			wantIsClaudeCode: true,
		},
		{
			name:           "Unknown when Claude Code but no recognizable state",
			content:        "-- INSERT --\nsome unrecognized output",
			wantStatus:     session.StatusUnknown,
			wantIsClaudeCode: true,
		},
		{
			name:           "old Esc to cancel in scrollback does not trigger Waiting",
			content:        "-- INSERT --\nEsc to cancel\n" + strings.Repeat("filler line\n", 20) + "❯ \n",
			wantStatus:     session.StatusIdle,
			wantIsClaudeCode: true,
		},
		{
			name:           "old working indicator in scrollback does not trigger Working",
			content:        "-- INSERT --\n⏺ running task\n" + strings.Repeat("filler line\n", 20) + "❯ \n",
			wantStatus:     session.StatusIdle,
			wantIsClaudeCode: true,
		},
		{
			name:           "Esc to cancel excluded from isWaiting so working indicator wins",
			content:        "-- INSERT --\n⏺ running task\nEsc to cancel\n",
			wantStatus:     session.StatusWorking,
			wantIsClaudeCode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotIsCC := detectStatusFromContent(tt.content)
			if gotStatus != tt.wantStatus {
				t.Errorf("detectStatusFromContent() status = %v, want %v", gotStatus, tt.wantStatus)
			}
			if gotIsCC != tt.wantIsClaudeCode {
				t.Errorf("detectStatusFromContent() isClaudeCode = %v, want %v", gotIsCC, tt.wantIsClaudeCode)
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

// --- isSeparatorLine ---

func TestIsSeparatorLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"all box-drawing chars", "────────────", true},
		{"single box-drawing char", "─", true},
		{"empty string", "", false},
		{"mixed chars", "─a─", false},
		{"regular dashes", "------------", false},
		{"spaces", "   ", false},
		{"box-drawing with trailing space", "────── ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSeparatorLine(tt.line)
			if got != tt.want {
				t.Errorf("isSeparatorLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
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

// --- detectStatusWithHooksForSession (mocked) ---

// withMockedDeps sets up mocked getClaudeStatusFn and hasActiveChildrenFn for a test,
// restoring the originals when the test completes.
func withMockedDeps(t *testing.T, statusFn func(string, string, string) string, childrenFn func(string, string, string) bool) {
	t.Helper()
	origStatus := getClaudeStatusFn
	origChildren := hasActiveChildrenFn
	getClaudeStatusFn = statusFn
	hasActiveChildrenFn = childrenFn
	t.Cleanup(func() {
		getClaudeStatusFn = origStatus
		hasActiveChildrenFn = origChildren
	})
}

func TestDetectStatusWithHooks_Tier1IdleReturnsImmediately(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "idle" },
		func(_, _, _ string) bool { t.Error("hasActiveChildren should not be called"); return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusIdle {
		t.Errorf("expected StatusIdle, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier1WaitingReturnsImmediately(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string) bool { t.Error("hasActiveChildren should not be called"); return false },
	)
	content := "-- INSERT --\nDo you want to proceed?"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusWaiting {
		t.Errorf("expected StatusWaiting, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier1WorkingOverriddenByTier3Idle(t *testing.T) {
	// Tier1 says "working" but tier3 content shows idle → tier3 overrides to idle
	withMockedDeps(t,
		func(_, _, _ string) string { return "working" },
		func(_, _, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusIdle {
		t.Errorf("expected StatusIdle (tier3 override), got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier2ActiveChildrenReturnsWorking(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" }, // no hook
		func(_, _, _ string) bool { return true },  // has children
	)
	content := "-- INSERT --\nsome output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusWorking {
		t.Errorf("expected StatusWorking from tier2, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier3WaitingFromContent(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
	)
	content := "-- INSERT --\nDo you want to proceed?"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusWaiting {
		t.Errorf("expected StatusWaiting from tier3, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier3IdleFromContent(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome text\n❯ "
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusIdle {
		t.Errorf("expected StatusIdle from tier3, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_Tier1WorkingFallbackWhenTier3Unknown(t *testing.T) {
	// Tier1 says "working", tier3 can't determine → should use tier1 working
	withMockedDeps(t,
		func(_, _, _ string) string { return "working" },
		func(_, _, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome unrecognized output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusWorking {
		t.Errorf("expected StatusWorking (tier1 fallback), got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}

func TestDetectStatusWithHooks_NotClaudeCode(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
	)
	content := "regular shell output"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusUnknown {
		t.Errorf("expected StatusUnknown, got %v", st)
	}
	if isCC {
		t.Error("expected isClaudeCode=false")
	}
}

func TestDetectStatusWithHooks_NoHookNoChildrenUnknownContent(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string) bool { return false },
	)
	content := "-- INSERT --\nsome unrecognized stuff"
	st, isCC := detectStatusWithHooksForSession(content, "clux", "0", "0")
	if st != session.StatusUnknown {
		t.Errorf("expected StatusUnknown, got %v", st)
	}
	if !isCC {
		t.Error("expected isClaudeCode=true")
	}
}
