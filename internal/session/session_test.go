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

func TestStatusString(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"Working", StatusWorking, "Working"},
		{"Idle", StatusIdle, "Done"},
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
