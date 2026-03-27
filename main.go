package main

import (
	"fmt"
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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			if commit != "" && date != "" {
				fmt.Printf("clux %s (%s, built %s)\n", version, commit, date)
			} else {
				fmt.Printf("clux %s\n", version)
			}
			return
		case "dashboard":
			cmdDashboard()
		case "init":
			cmdInit()
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: clux [dashboard|init]\n", os.Args[1])
			os.Exit(1)
		}
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
