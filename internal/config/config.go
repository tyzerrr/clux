package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ExternalSession represents a registered external tmux session:window.
type ExternalSession struct {
	Session string `json:"session"`
	Window  string `json:"window"`
}

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
	ExternalSessions []ExternalSession  `json:"external_sessions"`
	PreviewDefault   bool               `json:"preview_default"`
	Notifications    *NotificationConfig `json:"notifications,omitempty"`
	GroupDefault     bool               `json:"group_default"`
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
		if os.IsNotExist(err) {
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return err
	}
	return nil
}

// Add adds an external session. Returns error if already registered.
func (c *Config) Add(session, window string) error {
	for _, es := range c.ExternalSessions {
		if es.Session == session && es.Window == window {
			return fmt.Errorf("%s:%s is already registered", session, window)
		}
	}
	c.ExternalSessions = append(c.ExternalSessions, ExternalSession{
		Session: session,
		Window:  window,
	})
	return nil
}

// Remove removes an external session. Returns error if not found.
func (c *Config) Remove(session, window string) error {
	for i, es := range c.ExternalSessions {
		if es.Session == session && es.Window == window {
			c.ExternalSessions = append(c.ExternalSessions[:i], c.ExternalSessions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%s:%s is not registered", session, window)
}
