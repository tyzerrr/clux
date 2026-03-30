# Clux — Claude Code multiplexer for tmux

A lightweight TUI for managing multiple Claude Code instances as tmux windows. View all sessions at a glance, check their status, and switch between them with a single keystroke.

## Features

- **Session list** — See all Claude Code sessions with status, name, git branch, and working directory
- **Status detection** — Automatically detects whether each session is Working (🔄), Waiting for input (⚠️), or Idle (✅)
- **Preview panel** — Live ANSI-colored preview of the selected session's output with scrollback support
- **Dashboard** — Grid view showing live previews of all sessions simultaneously, with pagination support
- **Broadcast** — Send the same prompt to multiple repositories at once
- **Grouping** — Group sessions by repository for organized display
- **External sessions** — Automatically detect and monitor Claude Code running in other tmux sessions
- **Bell notifications** — Get notified when sessions transition from Working → Idle or Working → Waiting
- **`@clux-summary`** — Claude Code agents can set a short task summary displayed in the session list
- **Fuzzy filter** — Filter sessions by name, summary, directory, or branch

## Install

```bash
brew install tanaka0325/tap/clux
```

Or with Go:

```bash
go install github.com/tanaka0325/clux@latest
```

## Quick Start

1. Install clux (see above)
2. Run `clux init` to configure tmux binding and Claude Code hook
3. Press `Ctrl-.` in tmux to launch

If run outside tmux, clux will automatically start a tmux session.

## Usage

### Recommended: tmux key binding

Run `clux init` to automatically configure a tmux key binding (see below), or add this manually to your `~/.tmux.conf`:

```tmux
# No prefix needed — press Ctrl-. directly to launch clux
bind-key -T root C-. display-popup -E -w100% -h100% "clux"
```

Then press `Ctrl-.` in any tmux window to launch clux instantly (no prefix key required).

### Init

Run `clux init` to configure your environment:

- Add a Claude Code `PostToolUse` hook to `~/.claude/settings.json` for instant Waiting status detection
- Append `@clux-summary` instructions to `~/.claude/CLAUDE.md`
- Add a tmux key binding (`Ctrl-.`) to `~/.tmux.conf`
- Write a default config file to `~/.config/clux/config.toml`

## Keybindings

All keys are configurable via `~/.config/clux/config.toml`. Defaults shown below.

### List View

| Key | Action |
|-----|--------|
| `Enter` | Attach to selected session |
| `j` / `k` / `↑` / `↓` | Navigate (also `Ctrl+N`/`Ctrl+J`, `Ctrl+P`/`Ctrl+K`) |
| `n` | New session (select from `ghq list`) |
| `K` | Kill / unregister session |
| `i` | Send prompt to selected session |
| `p` | Toggle preview panel |
| `Ctrl+U` / `Ctrl+D` | Scroll preview up / down |
| `d` | Switch to dashboard view |
| `b` | Broadcast prompt to multiple repos |
| `g` | Toggle grouping by repository |
| `/` | Filter sessions |
| `q` / `Esc` | Quit |

### Dashboard

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / arrows | Navigate grid |
| `Enter` | Attach to focused session |
| `i` | Send prompt to focused session |
| `Ctrl+U` / `Ctrl+D` | Scroll focused tile up / down |
| `[` / `]` | Previous / next page |
| `K` / `n` / `b` / `/` | Same as list view |
| `d` / `Esc` | Return to list view |
| `q` | Quit |

### Broadcast Select (after pressing `b`)

| Key | Action |
|-----|--------|
| `Space` | Toggle repo selection |
| `Enter` | Confirm selection and enter prompt |
| `Esc` | Cancel |

Type to fuzzy-filter the repo list. If no repos are selected, the repo at the cursor is used.

### Global

| Key | Action |
|-----|--------|
| `Ctrl+.` | Quit immediately |

## CLI Subcommands

```
clux                  Launch the TUI session switcher
clux dashboard        Launch directly in dashboard mode
clux init             Install hook, tmux binding, @clux-summary instructions, and default config
clux --version, -v    Show version information
```

## Configuration

Config file: `~/.config/clux/config.toml`

```toml
# Show preview panel by default
preview_default = true

# Show grouped by repository by default
group_default = false

# Bell notifications on status changes
[notifications]
working_to_idle = true
working_to_waiting = true

# Key bindings (each action accepts multiple keys)
[keymaps]
move_up = ["k", "up"]
move_down = ["j", "down"]
quit = ["q"]
# ... (see full list in generated config)
```

| Field | Default | Description |
|-------|---------|-------------|
| `preview_default` | `true` | Start with preview panel open |
| `group_default` | `false` | Start with grouping enabled |
| `notifications.working_to_idle` | `true` | Bell on Working → Idle |
| `notifications.working_to_waiting` | `true` | Bell on Working → Waiting |
| `keymaps.*` | (see defaults) | Custom key bindings per action |

## Requirements

- **tmux** 3.2+ (for `display-popup` support)
- **Claude Code** (`claude` on PATH)
- **Go** 1.26+ (for building)
- **[ghq](https://github.com/x-motemen/ghq)** (optional — used by new session and broadcast features)

## Notes

- Status detection uses heuristics (content hashing, process tree inspection, pattern matching) and may occasionally misidentify states.

## License

MIT
