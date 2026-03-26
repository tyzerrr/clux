package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// KeymapConfig holds customizable key bindings for each action.
// Each action maps to one or more key strings (as reported by bubbletea).
type KeymapConfig struct {
	MoveUp        []string `json:"move_up,omitempty"`
	MoveDown      []string `json:"move_down,omitempty"`
	MoveLeft      []string `json:"move_left,omitempty"`
	MoveRight     []string `json:"move_right,omitempty"`
	Quit          []string `json:"quit,omitempty"`
	Filter        []string `json:"filter,omitempty"`
	NewSession    []string `json:"new_session,omitempty"`
	Kill          []string `json:"kill,omitempty"`
	TogglePreview []string `json:"toggle_preview,omitempty"`
	ScrollUp      []string `json:"scroll_up,omitempty"`
	ScrollDown    []string `json:"scroll_down,omitempty"`
	Dashboard     []string `json:"dashboard,omitempty"`
	Broadcast     []string `json:"broadcast,omitempty"`
	Group         []string `json:"group,omitempty"`
	Input         []string `json:"input,omitempty"`
	NextPage      []string `json:"next_page,omitempty"`
	PrevPage      []string `json:"prev_page,omitempty"`
	ToggleSelect  []string `json:"toggle_select,omitempty"`
}

// DefaultKeymap returns the default key bindings matching the original hardcoded values.
func DefaultKeymap() *KeymapConfig {
	return &KeymapConfig{
		MoveUp:        []string{"k", "up", "ctrl+p", "ctrl+k"},
		MoveDown:      []string{"j", "down", "ctrl+n", "ctrl+j"},
		MoveLeft:      []string{"h", "left"},
		MoveRight:     []string{"l", "right"},
		Quit:          []string{"q"},
		Filter:        []string{"/"},
		NewSession:    []string{"n"},
		Kill:          []string{"K"},
		TogglePreview: []string{"p"},
		ScrollUp:      []string{"ctrl+u"},
		ScrollDown:    []string{"ctrl+d"},
		Dashboard:     []string{"d"},
		Broadcast:     []string{"b"},
		Group:         []string{"g"},
		Input:         []string{"i"},
		NextPage:      []string{"]"},
		PrevPage:      []string{"["},
		ToggleSelect:  []string{"space", " "},
	}
}

// applyDefaults fills in any nil/empty slices with defaults from DefaultKeymap.
func (k *KeymapConfig) applyDefaults() {
	d := DefaultKeymap()
	if len(k.MoveUp) == 0 {
		k.MoveUp = d.MoveUp
	}
	if len(k.MoveDown) == 0 {
		k.MoveDown = d.MoveDown
	}
	if len(k.MoveLeft) == 0 {
		k.MoveLeft = d.MoveLeft
	}
	if len(k.MoveRight) == 0 {
		k.MoveRight = d.MoveRight
	}
	if len(k.Quit) == 0 {
		k.Quit = d.Quit
	}
	if len(k.Filter) == 0 {
		k.Filter = d.Filter
	}
	if len(k.NewSession) == 0 {
		k.NewSession = d.NewSession
	}
	if len(k.Kill) == 0 {
		k.Kill = d.Kill
	}
	if len(k.TogglePreview) == 0 {
		k.TogglePreview = d.TogglePreview
	}
	if len(k.ScrollUp) == 0 {
		k.ScrollUp = d.ScrollUp
	}
	if len(k.ScrollDown) == 0 {
		k.ScrollDown = d.ScrollDown
	}
	if len(k.Dashboard) == 0 {
		k.Dashboard = d.Dashboard
	}
	if len(k.Broadcast) == 0 {
		k.Broadcast = d.Broadcast
	}
	if len(k.Group) == 0 {
		k.Group = d.Group
	}
	if len(k.Input) == 0 {
		k.Input = d.Input
	}
	if len(k.NextPage) == 0 {
		k.NextPage = d.NextPage
	}
	if len(k.PrevPage) == 0 {
		k.PrevPage = d.PrevPage
	}
	if len(k.ToggleSelect) == 0 {
		k.ToggleSelect = d.ToggleSelect
	}
}

// --- Is methods: check if a key matches an action ---

func (k *KeymapConfig) IsMoveUp(key string) bool      { return slices.Contains(k.MoveUp, key) }
func (k *KeymapConfig) IsMoveDown(key string) bool     { return slices.Contains(k.MoveDown, key) }
func (k *KeymapConfig) IsMoveLeft(key string) bool     { return slices.Contains(k.MoveLeft, key) }
func (k *KeymapConfig) IsMoveRight(key string) bool    { return slices.Contains(k.MoveRight, key) }
func (k *KeymapConfig) IsQuit(key string) bool          { return slices.Contains(k.Quit, key) }
func (k *KeymapConfig) IsFilter(key string) bool        { return slices.Contains(k.Filter, key) }
func (k *KeymapConfig) IsNewSession(key string) bool    { return slices.Contains(k.NewSession, key) }
func (k *KeymapConfig) IsKill(key string) bool          { return slices.Contains(k.Kill, key) }
func (k *KeymapConfig) IsTogglePreview(key string) bool { return slices.Contains(k.TogglePreview, key) }
func (k *KeymapConfig) IsScrollUp(key string) bool      { return slices.Contains(k.ScrollUp, key) }
func (k *KeymapConfig) IsScrollDown(key string) bool    { return slices.Contains(k.ScrollDown, key) }
func (k *KeymapConfig) IsDashboard(key string) bool     { return slices.Contains(k.Dashboard, key) }
func (k *KeymapConfig) IsBroadcast(key string) bool     { return slices.Contains(k.Broadcast, key) }
func (k *KeymapConfig) IsGroup(key string) bool         { return slices.Contains(k.Group, key) }
func (k *KeymapConfig) IsInput(key string) bool         { return slices.Contains(k.Input, key) }
func (k *KeymapConfig) IsNextPage(key string) bool      { return slices.Contains(k.NextPage, key) }
func (k *KeymapConfig) IsPrevPage(key string) bool      { return slices.Contains(k.PrevPage, key) }
func (k *KeymapConfig) IsToggleSelect(key string) bool  { return slices.Contains(k.ToggleSelect, key) }

// isNonPrintable returns true if the key is a non-printable key (arrows, ctrl+x, special keys).
func isNonPrintable(key string) bool {
	switch key {
	case "up", "down", "left", "right", "enter", "esc", "tab", "space", "backspace", "delete":
		return true
	}
	return strings.HasPrefix(key, "ctrl+") || strings.HasPrefix(key, "alt+")
}

// IsMoveUpNonPrintable matches MoveUp keys excluding single-char printable keys (for use in text input modes).
func (k *KeymapConfig) IsMoveUpNonPrintable(key string) bool {
	return isNonPrintable(key) && slices.Contains(k.MoveUp, key)
}

// IsMoveDownNonPrintable matches MoveDown keys excluding single-char printable keys (for use in text input modes).
func (k *KeymapConfig) IsMoveDownNonPrintable(key string) bool {
	return isNonPrintable(key) && slices.Contains(k.MoveDown, key)
}

// --- Hint methods: format keys for help bar display ---

// displayKeyMap maps internal key names to user-friendly display strings.
var displayKeyMap = map[string]string{
	"up":    "↑",
	"down":  "↓",
	"left":  "←",
	"right": "→",
	"space": "Space",
	" ":     "Space",
	"enter": "↵",
}

// formatKey returns the display string for a single key.
func formatKey(key string) string {
	if d, ok := displayKeyMap[key]; ok {
		return d
	}
	return key
}

// formatKeyGroups returns a slash-joined, deduplicated display string for multiple key lists.
func formatKeyGroups(keyGroups ...[]string) string {
	seen := make(map[string]bool)
	var parts []string
	for _, keys := range keyGroups {
		for _, key := range keys {
			d := formatKey(key)
			if !seen[d] {
				seen[d] = true
				parts = append(parts, d)
			}
		}
	}
	return strings.Join(parts, "/")
}

func (k *KeymapConfig) HintMoveUp() string      { return formatKeyGroups(k.MoveUp) }
func (k *KeymapConfig) HintMoveDown() string     { return formatKeyGroups(k.MoveDown) }
func (k *KeymapConfig) HintMoveLeft() string     { return formatKeyGroups(k.MoveLeft) }
func (k *KeymapConfig) HintMoveRight() string    { return formatKeyGroups(k.MoveRight) }
func (k *KeymapConfig) HintQuit() string          { return formatKeyGroups(k.Quit) }
func (k *KeymapConfig) HintFilter() string        { return formatKeyGroups(k.Filter) }
func (k *KeymapConfig) HintNewSession() string    { return formatKeyGroups(k.NewSession) }
func (k *KeymapConfig) HintKill() string          { return formatKeyGroups(k.Kill) }
func (k *KeymapConfig) HintTogglePreview() string { return formatKeyGroups(k.TogglePreview) }
func (k *KeymapConfig) HintScrollUp() string      { return formatKeyGroups(k.ScrollUp) }
func (k *KeymapConfig) HintScrollDown() string    { return formatKeyGroups(k.ScrollDown) }
func (k *KeymapConfig) HintDashboard() string     { return formatKeyGroups(k.Dashboard) }
func (k *KeymapConfig) HintBroadcast() string     { return formatKeyGroups(k.Broadcast) }
func (k *KeymapConfig) HintGroup() string         { return formatKeyGroups(k.Group) }
func (k *KeymapConfig) HintInput() string         { return formatKeyGroups(k.Input) }
func (k *KeymapConfig) HintNextPage() string      { return formatKeyGroups(k.NextPage) }
func (k *KeymapConfig) HintPrevPage() string      { return formatKeyGroups(k.PrevPage) }
func (k *KeymapConfig) HintToggleSelect() string  { return formatKeyGroups(k.ToggleSelect) }

func (k *KeymapConfig) HintNavigate() string     { return formatKeyGroups(k.MoveUp, k.MoveDown) }
func (k *KeymapConfig) HintNavigateFull() string  { return formatKeyGroups(k.MoveUp, k.MoveDown, k.MoveLeft, k.MoveRight) }
func (k *KeymapConfig) HintScroll() string        { return formatKeyGroups(k.ScrollUp, k.ScrollDown) }

// Config holds clux persistent configuration.
type Config struct {
	PreviewDefault bool                `json:"preview_default"`
	Notifications  *NotificationConfig `json:"notifications,omitempty"`
	GroupDefault   bool                `json:"group_default"`
	Keymaps        *KeymapConfig       `json:"keymaps,omitempty"`
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
			cfg := &Config{}
			cfg.ensureKeymaps()
			return cfg, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.ensureKeymaps()
	return &cfg, nil
}

// NewConfig returns a Config with all defaults initialized.
func NewConfig() *Config {
	c := &Config{}
	c.ensureKeymaps()
	return c
}

// ensureKeymaps initializes Keymaps if nil and applies defaults for any missing fields.
func (c *Config) ensureKeymaps() {
	if c.Keymaps == nil {
		c.Keymaps = DefaultKeymap()
	} else {
		c.Keymaps.applyDefaults()
	}
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
