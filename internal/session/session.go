package session

// Status represents the current state of a Claude Code session.
type Status int

const (
	// StatusUnknown is iota 0 intentionally so that zero-initialized Session defaults to Unknown, not Working.
	StatusUnknown Status = iota
	StatusWorking
	StatusIdle
	StatusWaiting
)

// Session holds information about a tmux window running Claude Code.
type Session struct {
	Name        string
	Summary     string // @clux-summary user option; empty if not set
	Dir         string // pane_current_path
	Branch      string // git branch name; empty if not a git repo
	Status      Status
	WindowIndex string // tmux window index
	External    bool   // true if registered from outside clux session
	SessionName string // tmux session name (for external sessions; empty means clux session)
}

// DisplayName returns Summary if set, otherwise Name.
// For external sessions with no summary, returns "session:window" format.
func (s Session) DisplayName() string {
	if s.Summary != "" {
		return s.Summary
	}
	if s.External && s.SessionName != "" {
		return s.SessionName + ":" + s.WindowIndex
	}
	return s.Name
}

// Icon returns an emoji icon representing the status.
func (s Status) Icon() string {
	switch s {
	case StatusWorking:
		return "🔄"
	case StatusIdle:
		return "✅"
	case StatusWaiting:
		return "⚠️"
	default:
		return "❓"
	}
}

// String returns a human-readable name for the status.
func (s Status) String() string {
	switch s {
	case StatusWorking:
		return "Working"
	case StatusIdle:
		return "Idle"
	case StatusWaiting:
		return "Waiting"
	default:
		return "Unknown"
	}
}
