package config

import (
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, []byte("not valid json"), 0644); err != nil {
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
