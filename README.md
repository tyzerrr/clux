# Clux — Claude Code multiplexer for tmux

A lightweight TUI for managing multiple Claude Code instances as tmux windows. View all sessions, check their status (Working/Idle/Waiting), and switch between them with a single keystroke.

## What it does

Clux runs as a tmux popup and displays all Claude Code sessions across different directories. Each session shows:

- **Session name** and **working directory**
- **Status indicator**: ● (Working) · ◐ (Waiting) · ○ (Idle) · ? (Unknown)
- **Quick switching**: Select a session and press Enter to attach

Perfect for managing 2–3 Claude Code instances simultaneously without context-switching overhead.

## Install

```bash
go install github.com/tanaka0325/clux@latest
```

Optional shell alias:

```bash
alias clx=clux
```

## Usage

### Must run inside tmux

Clux requires tmux. It will check for the `$TMUX` environment variable at startup.

### Recommended: tmux key binding

Add this to your `~/.tmux.conf`:

```tmux
bind-key C-c display-popup -E -w80% -h80% "clux"
```

Then press `Ctrl-C` in any tmux window to launch the Clux session switcher.

## Keybindings

| Key | Action |
|-----|--------|
| `Enter` | Attach to selected session |
| `j` / `k` or `↑` / `↓` | Navigate |
| `n` | New session |
| `K` | Kill session |
| `/` | Filter |
| `R` | Refresh |
| `q` / `Esc` | Quit |

## Requirements

- **tmux** (1.8+)
- **Go** 1.21+ (for building)
- **ghq** (optional, for `n` new session feature)

## License

MIT
