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
	Dir         string // pane_current_path
	Status      Status
	WindowIndex string // tmux window index
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
