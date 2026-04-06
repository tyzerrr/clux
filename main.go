package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/tanaka0325/clux/internal/model"
	"github.com/tanaka0325/clux/internal/tmux"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

type cliAction int

const (
	cliActionLaunch cliAction = iota
	cliActionDashboard
	cliActionInit
	cliActionExit
)

func main() {
	action, exitCode := dispatchCLI(os.Args[1:], os.Stdout, os.Stderr)
	switch action {
	case cliActionExit:
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	case cliActionDashboard:
		cmdDashboard()
		return
	case cliActionInit:
		cmdInit()
		return
	}

	if os.Getenv("TMUX") == "" {
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "clux: could not determine executable path: %v\n", err)
			os.Exit(1)
		}
		tmuxBin, err := exec.LookPath("tmux")
		if err != nil {
			fmt.Fprintf(os.Stderr, "clux: tmux not found in PATH: %v\n", err)
			os.Exit(1)
		}
		shellCmd := "'" + strings.ReplaceAll(self, "'", `'\''`) + "'"
		if err := syscall.Exec(tmuxBin, []string{"tmux", "new-session", "-A", "-s", tmux.SessionName, shellCmd}, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "clux: failed to exec tmux: %v\n", err)
			os.Exit(1)
		}
	}

	if err := tmux.EnsureSession(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func dispatchCLI(args []string, stdout, stderr io.Writer) (cliAction, int) {
	if len(args) == 0 {
		return cliActionLaunch, 0
	}

	switch args[0] {
	case "-h", "--help":
		if len(args) > 1 {
			return writeUnexpectedArgs(stderr, "clux", args[1:], writeRootHelp)
		}
		writeRootHelp(stdout)
		return cliActionExit, 0
	case "-v", "--version":
		if len(args) > 1 {
			return writeUnexpectedArgs(stderr, "clux", args[1:], writeRootHelp)
		}
		writeVersion(stdout)
		return cliActionExit, 0
	case "dashboard":
		return dispatchSubcommand(args[1:], "dashboard", writeDashboardHelp, cliActionDashboard, stdout, stderr)
	case "init":
		return dispatchSubcommand(args[1:], "init", writeInitHelp, cliActionInit, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "clux: unknown command %q\n\n", args[0])
		writeRootHelp(stderr)
		return cliActionExit, 1
	}
}

func dispatchSubcommand(args []string, name string, helpWriter func(io.Writer), action cliAction, stdout, stderr io.Writer) (cliAction, int) {
	if len(args) == 0 {
		return action, 0
	}
	if len(args) == 1 && isHelpFlag(args[0]) {
		helpWriter(stdout)
		return cliActionExit, 0
	}
	return writeUnexpectedArgs(stderr, "clux "+name, args, helpWriter)
}

func writeUnexpectedArgs(w io.Writer, command string, args []string, helpWriter func(io.Writer)) (cliAction, int) {
	_, _ = fmt.Fprintf(w, "%s: unexpected argument %q\n\n", command, args[0])
	helpWriter(w)
	return cliActionExit, 1
}

func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func writeVersion(w io.Writer) {
	if commit != "" && date != "" {
		_, _ = fmt.Fprintf(w, "clux %s (%s, built %s)\n", version, commit, date)
		return
	}
	_, _ = fmt.Fprintf(w, "clux %s\n", version)
}

func writeRootHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  clux [command]

Launch the clux TUI session switcher.

Commands:
  dashboard    Launch directly in dashboard mode
  init         Install hook, tmux binding, @clux-summary instructions, and default config

Options:
  -h, --help       Show help
  -v, --version    Show version information

Examples:
  clux
  clux dashboard
  clux init
`)
}

func writeDashboardHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  clux dashboard

Launch clux directly in dashboard mode.

Options:
  -h, --help    Show help
`)
}

func writeInitHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  clux init

Install the Claude Code hook, tmux binding, @clux-summary instructions, and default config.

Options:
  -h, --help    Show help
`)
}

func cmdDashboard() {
	if err := tmux.EnsureSession(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p := tea.NewProgram(model.NewDashboard())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
