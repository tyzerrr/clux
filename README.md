# Clux — Claude Code multiplexer for tmux

A lightweight TUI for managing multiple Claude Code instances as tmux windows. View all sessions at a glance, check their status, and switch between them with a single keystroke.

## Features

- **Session list** — See all Claude Code sessions with status, name, git branch, and working directory
- **Status detection** — Three-tier detection (tmux hook → process tree → content hash) accurately identifies Working (🔄), Waiting (⚠️), Idle (✅), and Unknown (❓) states
- **Preview panel** — Live ANSI-colored preview of the selected session's output with scrollback support
- **Dashboard** — Grid view showing live previews of all sessions simultaneously, with pagination support
- **Broadcast** — Send the same prompt to multiple repositories at once
- **Grouping** — Group sessions by repository for organized display
- **External sessions** — Monitor Claude Code running in other tmux sessions
- **Bell notifications** — Get notified when sessions transition from Working → Idle or Working → Waiting
- **`@clux-summary`** — Claude Code agents can set a short task summary displayed in the session list
- **Fuzzy filter** — Filter sessions by name, summary, directory, or branch

## Install

```bash
go install github.com/tanaka0325/clux@latest
```

## Usage

### Recommended: tmux key binding

Add this to your `~/.tmux.conf`:

```tmux
bind-key C-c display-popup -E -w80% -h80% "clux"
```

Then press `Prefix` + `Ctrl-C` in any tmux window to launch clux.

### Setup (optional)

Run `clux setup` to configure your environment:

- Add a Claude Code `PostToolUse` hook to `~/.claude/settings.json` for instant Waiting status detection
- Append `@clux-summary` instructions to `~/.claude/CLAUDE.md`

## Keybindings

### List View

| Key | Action |
|-----|--------|
| `Enter` | Attach to selected session |
| `j` / `k` / `↑` / `↓` | Navigate |
| `n` | New session (select from `ghq list`) |
| `K` | Kill / unregister session |
| `p` | Toggle preview panel |
| `Ctrl+U` / `Ctrl+D` | Scroll preview up / down |
| `d` | Switch to dashboard view |
| `b` | Broadcast prompt to multiple repos |
| `g` | Toggle grouping by repository |
| `a` | Add external session |
| `/` | Filter sessions |
| `q` / `Esc` | Quit |

### Dashboard

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / arrows | Navigate grid |
| `Enter` | Attach to focused session |
| `F` | Open dashboard as full-screen tmux popup |
| `[` / `]` | Previous / next page |
| `K` / `n` / `b` / `/` | Same as list view |
| `d` / `Esc` | Return to list view |
| `q` | Quit |

### Global

| Key | Action |
|-----|--------|
| `Ctrl+.` | Quit immediately |

## CLI Subcommands

```
clux                  Launch the TUI session switcher
clux dashboard        Launch directly in dashboard mode
clux setup            Install Claude Code hook and @clux-summary instructions
clux add <sess:win>   Register an external tmux session:window
clux remove <sess:win> Unregister an external session
clux list             List registered external sessions
```

## Configuration

Config file: `~/.config/clux/sessions.json`

```json
{
  "preview_default": false,
  "group_default": false,
  "notifications": {
    "working_to_idle": true,
    "working_to_waiting": true
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `preview_default` | `false` | Start with preview panel open |
| `group_default` | `false` | Start with grouping enabled |
| `notifications.working_to_idle` | `true` | Bell on Working → Idle |
| `notifications.working_to_waiting` | `true` | Bell on Working → Waiting |

## Requirements

- **tmux** 3.2+ (for `display-popup` support)
- **Claude Code** (`claude` on PATH)
- **Go** 1.25+ (for building)
- **ghq** (optional — used by new session and broadcast features)

## License

MIT
