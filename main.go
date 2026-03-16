package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/tanaka0325/clux/internal/config"
	"github.com/tanaka0325/clux/internal/model"
	"github.com/tanaka0325/clux/internal/tmux"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			cmdAdd(os.Args[2:])
		case "remove":
			cmdRemove(os.Args[2:])
		case "list":
			cmdList()
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: clux [add|remove|list]\n", os.Args[1])
			os.Exit(1)
		}
		return
	}

	if err := tmux.CheckTmux(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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

func parseSessionWindow(arg string) (session, window string, err error) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid format %q, expected <session>:<window>", arg)
	}
	return parts[0], parts[1], nil
}

func cmdAdd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: clux add <session>:<window>")
		os.Exit(1)
	}
	sess, win, err := parseSessionWindow(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Add(sess, win); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registered %s:%s\n", sess, win)
}

func cmdRemove(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: clux remove <session>:<window>")
		os.Exit(1)
	}
	sess, win, err := parseSessionWindow(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Remove(sess, win); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Unregistered %s:%s\n", sess, win)
}

func cmdList() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.ExternalSessions) == 0 {
		fmt.Println("No external sessions registered.")
		return
	}
	for _, es := range cfg.ExternalSessions {
		fmt.Printf("%s:%s\n", es.Session, es.Window)
	}
}
