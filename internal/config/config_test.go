package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_AddAndRemove(t *testing.T) {
	cfg := &Config{}

	// Add first session.
	if err := cfg.Add("main", "1"); err != nil {
		t.Fatalf("unexpected error adding session: %v", err)
	}
	if len(cfg.ExternalSessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(cfg.ExternalSessions))
	}
	if cfg.ExternalSessions[0].Session != "main" || cfg.ExternalSessions[0].Window != "1" {
		t.Errorf("unexpected session: %+v", cfg.ExternalSessions[0])
	}

	// Add duplicate — should error.
	if err := cfg.Add("main", "1"); err == nil {
		t.Error("expected error when adding duplicate, got nil")
	}

	// Add a different session.
	if err := cfg.Add("work", "2"); err != nil {
		t.Fatalf("unexpected error adding second session: %v", err)
	}
	if len(cfg.ExternalSessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(cfg.ExternalSessions))
	}

	// Remove first session.
	if err := cfg.Remove("main", "1"); err != nil {
		t.Fatalf("unexpected error removing session: %v", err)
	}
	if len(cfg.ExternalSessions) != 1 {
		t.Fatalf("expected 1 session after remove, got %d", len(cfg.ExternalSessions))
	}
	if cfg.ExternalSessions[0].Session != "work" {
		t.Errorf("expected remaining session to be 'work', got %q", cfg.ExternalSessions[0].Session)
	}

	// Remove non-existent — should error.
	if err := cfg.Remove("main", "1"); err == nil {
		t.Error("expected error removing non-existent session, got nil")
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// Override the home dir for this test using a temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{}
	if err := cfg.Add("session-a", "3"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := cfg.Add("session-b", "5"); err != nil {
		t.Fatalf("Add: %v", err)
	}

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
	if len(loaded.ExternalSessions) != 2 {
		t.Fatalf("expected 2 sessions after load, got %d", len(loaded.ExternalSessions))
	}
	if loaded.ExternalSessions[0].Session != "session-a" || loaded.ExternalSessions[0].Window != "3" {
		t.Errorf("unexpected first session: %+v", loaded.ExternalSessions[0])
	}
	if loaded.ExternalSessions[1].Session != "session-b" || loaded.ExternalSessions[1].Window != "5" {
		t.Errorf("unexpected second session: %+v", loaded.ExternalSessions[1])
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
	if len(cfg.ExternalSessions) != 0 {
		t.Errorf("expected empty sessions, got %d", len(cfg.ExternalSessions))
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
