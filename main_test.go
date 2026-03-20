package main

import "testing"

func TestParseSessionWindow(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSession string
		wantWindow  string
		wantErr     bool
	}{
		{"valid", "session:window", "session", "window", false},
		{"multiple colons", "a:b:c", "a", "b:c", false},
		{"missing colon", "sessionwindow", "", "", true},
		{"empty session", ":window", "", "", true},
		{"empty window", "session:", "", "", true},
		{"empty string", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, win, err := parseSessionWindow(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSessionWindow(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if sess != tt.wantSession {
				t.Errorf("session = %q, want %q", sess, tt.wantSession)
			}
			if win != tt.wantWindow {
				t.Errorf("window = %q, want %q", win, tt.wantWindow)
			}
		})
	}
}
