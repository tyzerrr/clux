package session

import "testing"

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"Working", StatusWorking, "🔄"},
		{"Idle", StatusIdle, "✅"},
		{"Waiting", StatusWaiting, "⚠️"},
		{"Unknown", StatusUnknown, "❓"},
		{"OutOfRange negative", Status(-1), "❓"},
		{"OutOfRange large", Status(100), "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.Icon()
			if got != tt.want {
				t.Errorf("Status(%d).Icon() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		session Session
		want    string
	}{
		{"Summary set", Session{Name: "win-name", Summary: "my task"}, "my task"},
		{"Summary empty", Session{Name: "win-name", Summary: ""}, "win-name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.DisplayName()
			if got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name               string
		session            Session
		defaultSessionName string
		wantSession        string
		wantPane           string
	}{
		{
			"both set",
			Session{SessionName: "my-session", PaneIndex: "2"},
			"default-session",
			"my-session", "2",
		},
		{
			"session empty uses default",
			Session{SessionName: "", PaneIndex: "1"},
			"default-session",
			"default-session", "1",
		},
		{
			"pane empty defaults to 0",
			Session{SessionName: "my-session", PaneIndex: ""},
			"default-session",
			"my-session", "0",
		},
		{
			"both empty",
			Session{SessionName: "", PaneIndex: ""},
			"default-session",
			"default-session", "0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSession, gotPane := tt.session.ResolveTarget(tt.defaultSessionName)
			if gotSession != tt.wantSession {
				t.Errorf("ResolveTarget() sessionName = %q, want %q", gotSession, tt.wantSession)
			}
			if gotPane != tt.wantPane {
				t.Errorf("ResolveTarget() paneIndex = %q, want %q", gotPane, tt.wantPane)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"Working", StatusWorking, "Working"},
		{"Idle", StatusIdle, "Idle"},
		{"Waiting", StatusWaiting, "Waiting"},
		{"Unknown", StatusUnknown, "Unknown"},
		{"OutOfRange negative", Status(-1), "Unknown"},
		{"OutOfRange large", Status(100), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
