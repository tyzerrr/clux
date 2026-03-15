package tmux

import (
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

// --- detectStatus ---

func TestDetectStatus(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantStatus     session.Status
		wantIsClaudeCC bool
	}{
		{
			name:           "not Claude Code returns false",
			content:        "regular shell output without any indicators",
			wantStatus:     session.StatusUnknown,
			wantIsClaudeCC: false,
		},
		{
			name:           "Waiting takes priority over Working",
			content:        "-- INSERT --\n✻ Worked\nDo you want to proceed?",
			wantStatus:     session.StatusWaiting,
			wantIsClaudeCC: true,
		},
		{
			name:           "Waiting takes priority over Idle",
			content:        "-- INSERT --\n❯ \n[Y/n]",
			wantStatus:     session.StatusWaiting,
			wantIsClaudeCC: true,
		},
		{
			name:           "Working when no Waiting",
			content:        "-- INSERT --\n✻ Worked on task",
			wantStatus:     session.StatusWorking,
			wantIsClaudeCC: true,
		},
		{
			name:           "Working takes priority over Idle",
			content:        "-- INSERT --\n✻ Worked\n❯ ",
			wantStatus:     session.StatusWorking,
			wantIsClaudeCC: true,
		},
		{
			name:           "Idle when only idle prompt",
			content:        "-- INSERT --\nsome text\n❯ ",
			wantStatus:     session.StatusIdle,
			wantIsClaudeCC: true,
		},
		{
			name:           "Unknown when Claude Code but no recognisable state",
			content:        "-- INSERT --\nsome unrecognised output",
			wantStatus:     session.StatusUnknown,
			wantIsClaudeCC: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotIsCC := detectStatus(tt.content)
			if gotStatus != tt.wantStatus {
				t.Errorf("detectStatus() status = %v, want %v", gotStatus, tt.wantStatus)
			}
			if gotIsCC != tt.wantIsClaudeCC {
				t.Errorf("detectStatus() isClaudeCode = %v, want %v", gotIsCC, tt.wantIsClaudeCC)
			}
		})
	}
}
