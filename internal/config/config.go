package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
)

// NotificationConfig controls which status transitions trigger a bell notification.
type NotificationConfig struct {
	WorkingToIdle    *bool `toml:"working_to_idle"`
	WorkingToWaiting *bool `toml:"working_to_waiting"`
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
	MoveUp        []string `toml:"move_up"`
	MoveDown      []string `toml:"move_down"`
	MoveLeft      []string `toml:"move_left"`
	MoveRight     []string `toml:"move_right"`
	Quit          []string `toml:"quit"`
	Filter        []string `toml:"filter"`
	NewSession    []string `toml:"new_session"`
	Kill          []string `toml:"kill"`
	TogglePreview []string `toml:"toggle_preview"`
	ScrollUp      []string `toml:"scroll_up"`
	ScrollDown    []string `toml:"scroll_down"`
	Dashboard     []string `toml:"dashboard"`
	Broadcast     []string `toml:"broadcast"`
	Group         []string `toml:"group"`
	Input         []string `toml:"input"`
	NextPage      []string `toml:"next_page"`
	PrevPage      []string `toml:"prev_page"`
	ToggleSelect  []string `toml:"toggle_select"`
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

// formatKeyGroupsMax returns a slash-joined, deduplicated display string for multiple key lists,
// limiting each group to at most max keys (0 means unlimited).
func formatKeyGroupsMax(max int, keyGroups ...[]string) string {
	seen := make(map[string]bool)
	var parts []string
	for _, keys := range keyGroups {
		count := 0
		for _, key := range keys {
			if max > 0 && count >= max {
				break
			}
			d := formatKey(key)
			if !seen[d] {
				seen[d] = true
				parts = append(parts, d)
				count++
			}
		}
	}
	return strings.Join(parts, "/")
}

// formatKeyGroups returns a slash-joined, deduplicated display string for multiple key lists,
// showing at most 2 keys per group to keep the help bar concise.
func formatKeyGroups(keyGroups ...[]string) string {
	return formatKeyGroupsMax(2, keyGroups...)
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
	PreviewDefault bool                `toml:"preview_default"`
	Notifications  *NotificationConfig `toml:"notifications"`
	GroupDefault   bool                `toml:"group_default"`
	Keymaps        *KeymapConfig       `toml:"keymaps"`
}

const configDir = ".config/clux"
const configFile = "config.toml"

// Legacy file names for migration.
var configFilesOld = []string{"config.json", "sessions.json"}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, configFile), nil
}

// Load reads the config from disk. If old JSON config files exist but config.toml
// does not, it migrates by reading the old file, saving as config.toml, and removing
// the old file. Returns empty config if no file exists.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		// config.toml doesn't exist — try migrating from old JSON files
		cfg, migrated := tryMigrateFromJSON()
		if migrated {
			return cfg, nil
		}
		// No files exist — return defaults
		cfg = &Config{}
		cfg.ensureDefaults()
		return cfg, nil
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.ensureDefaults()
	return &cfg, nil
}

// tryMigrateFromJSON attempts to load config from legacy JSON files.
// Returns the config and true if migration succeeded, nil and false otherwise.
func tryMigrateFromJSON() (*Config, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, false
	}
	for _, oldFile := range configFilesOld {
		oldPath := filepath.Join(home, configDir, oldFile)
		oldData, err := os.ReadFile(oldPath)
		if err != nil {
			continue
		}
		var cfg Config
		if err := json.Unmarshal(oldData, &cfg); err != nil {
			continue
		}
		cfg.ensureDefaults()
		// Best-effort: save as TOML and remove old file
		if err := cfg.Save(); err == nil {
			_ = os.Remove(oldPath)
		}
		return &cfg, true
	}
	return nil, false
}

// DefaultNotifications returns the default notification settings.
func DefaultNotifications() *NotificationConfig {
	t := true
	return &NotificationConfig{
		WorkingToIdle:    &t,
		WorkingToWaiting: &t,
	}
}

// NewConfig returns a Config with all defaults initialized.
func NewConfig() *Config {
	c := &Config{}
	c.ensureDefaults()
	return c
}

// ensureDefaults initializes all config sections with defaults for any missing fields.
func (c *Config) ensureDefaults() {
	if c.Notifications == nil {
		c.Notifications = DefaultNotifications()
	} else {
		t := true
		if c.Notifications.WorkingToIdle == nil {
			c.Notifications.WorkingToIdle = &t
		}
		if c.Notifications.WorkingToWaiting == nil {
			c.Notifications.WorkingToWaiting = &t
		}
	}
	if c.Keymaps == nil {
		c.Keymaps = DefaultKeymap()
	} else {
		c.Keymaps.applyDefaults()
	}
}

// configTemplate is the TOML template with comments for the config file.
var configTemplate = template.Must(template.New("config").Funcs(template.FuncMap{
	"tomlArray": tomlArray,
	"deref": func(b *bool) bool {
		if b == nil {
			return true
		}
		return *b
	},
}).Parse(`# Show preview panel by default
preview_default = {{ .PreviewDefault }}

# Show grouped by repository by default
group_default = {{ .GroupDefault }}

# Bell notifications on status changes
[notifications]
# Notify when a session changes from Working to Idle
working_to_idle = {{ deref .Notifications.WorkingToIdle }}
# Notify when a session changes from Working to Waiting
working_to_waiting = {{ deref .Notifications.WorkingToWaiting }}

# Key bindings (each action accepts multiple keys)
# Key names follow bubbletea conventions: "ctrl+x", "alt+x", "up", "down", "space", etc.
[keymaps]
# Navigation
move_up = {{ tomlArray .Keymaps.MoveUp }}
move_down = {{ tomlArray .Keymaps.MoveDown }}
move_left = {{ tomlArray .Keymaps.MoveLeft }}
move_right = {{ tomlArray .Keymaps.MoveRight }}

# Actions
quit = {{ tomlArray .Keymaps.Quit }}
filter = {{ tomlArray .Keymaps.Filter }}
new_session = {{ tomlArray .Keymaps.NewSession }}
kill = {{ tomlArray .Keymaps.Kill }}
toggle_preview = {{ tomlArray .Keymaps.TogglePreview }}
dashboard = {{ tomlArray .Keymaps.Dashboard }}
broadcast = {{ tomlArray .Keymaps.Broadcast }}
group = {{ tomlArray .Keymaps.Group }}
input = {{ tomlArray .Keymaps.Input }}

# Scrolling and pagination
scroll_up = {{ tomlArray .Keymaps.ScrollUp }}
scroll_down = {{ tomlArray .Keymaps.ScrollDown }}
next_page = {{ tomlArray .Keymaps.NextPage }}
prev_page = {{ tomlArray .Keymaps.PrevPage }}

# Selection (used in broadcast select mode)
toggle_select = {{ tomlArray .Keymaps.ToggleSelect }}
`))

// tomlArray formats a string slice as a TOML inline array.
func tomlArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Save writes the config to disk as commented TOML.
func (c *Config) Save() error {
	c.ensureDefaults()
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, c); err != nil {
		return fmt.Errorf("rendering config template: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
