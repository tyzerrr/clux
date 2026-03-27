package tmux

import (
	"os"
	"strings"
	"testing"
)

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

func TestKillWindowForSession_EmptySessionName(t *testing.T) {
	err := KillWindowForSession("", "0")
	if err == nil {
		t.Error("expected error for empty session name")
	}
}

func TestKillWindowForSession_InvalidIndex(t *testing.T) {
	err := KillWindowForSession("my-session", "abc")
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

// --- WindowExists ---

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
