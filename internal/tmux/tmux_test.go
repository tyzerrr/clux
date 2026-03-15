package tmux

import (
	"os"
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
		{"contains Esc to cancel", "Press Esc to cancel the operation", true},
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

// --- detectStatus ---

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotIsCC := detectStatus(tt.content)
			if gotStatus != tt.wantStatus {
				t.Errorf("detectStatus() status = %v, want %v", gotStatus, tt.wantStatus)
			}
			if gotIsCC != tt.wantIsClaudeCode {
				t.Errorf("detectStatus() isClaudeCode = %v, want %v", gotIsCC, tt.wantIsClaudeCode)
			}
		})
	}
}

// --- SwitchWindow/KillWindow invalid index tests ---

func TestSwitchWindow_InvalidIndex(t *testing.T) {
	err := SwitchWindow("abc")
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
