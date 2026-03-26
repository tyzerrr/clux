package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// NotificationConfig controls which status transitions trigger a bell notification.
type NotificationConfig struct {
	WorkingToIdle    *bool `json:"working_to_idle,omitempty"`
	WorkingToWaiting *bool `json:"working_to_waiting,omitempty"`
}

// ShouldNotifyWorkingToIdle returns true if a Working→Idle bell should fire. Default: true.
func (n *NotificationConfig) ShouldNotifyWorkingToIdle() bool {
	if n == nil || n.WorkingToIdle == nil {
		return true
	}
	return *n.WorkingToIdle
}

// ShouldNotifyWorkingToWaiting returns true if a Working→Waiting bell should fire. Default: true.
func (n *NotificationConfig) ShouldNotifyWorkingToWaiting() bool {
	if n == nil || n.WorkingToWaiting == nil {
		return true
	}
	return *n.WorkingToWaiting
}

// Config holds clux persistent configuration.
type Config struct {
	PreviewDefault bool                `json:"preview_default"`
	Notifications  *NotificationConfig `json:"notifications,omitempty"`
	GroupDefault   bool                `json:"group_default"`
}

const configDir = ".config/clux"
const configFile = "sessions.json"

// configPath returns the full path to the config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, configFile), nil
}

// Load reads the config from disk. Returns empty config if file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return err
	}
	return nil
}
