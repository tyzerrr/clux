package main

import (
	"bytes"
	"testing"
)

func TestDispatchCLI_LaunchByDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	action, exitCode := dispatchCLI(nil, &stdout, &stderr)

	if action != cliActionLaunch {
		t.Fatalf("expected cliActionLaunch, got %v", action)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

func TestDispatchCLI_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	action, exitCode := dispatchCLI([]string{"--help"}, &stdout, &stderr)

	if action != cliActionExit {
		t.Fatalf("expected cliActionExit, got %v", action)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Launch the clux TUI session switcher.")) {
		t.Fatalf("expected root help in stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

func TestDispatchCLI_SubcommandHelpByFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []byte
	}{
		{
			name:     "dashboard",
			args:     []string{"dashboard", "--help"},
			expected: []byte("Usage:\n  clux dashboard"),
		},
		{
			name:     "init",
			args:     []string{"init", "-h"},
			expected: []byte("Usage:\n  clux init"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			action, exitCode := dispatchCLI(tt.args, &stdout, &stderr)

			if action != cliActionExit {
				t.Fatalf("expected cliActionExit, got %v", action)
			}
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d", exitCode)
			}
			if !bytes.Contains(stdout.Bytes(), tt.expected) {
				t.Fatalf("expected help output %q, got %q", string(tt.expected), stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected no stderr, got %q", stderr.String())
			}
		})
	}
}

func TestDispatchCLI_HelpCommandIsUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer

	action, exitCode := dispatchCLI([]string{"help"}, &stdout, &stderr)

	if action != cliActionExit {
		t.Fatalf("expected cliActionExit, got %v", action)
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`clux: unknown command "help"`)) {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Usage:\n  clux [command]")) {
		t.Fatalf("expected root usage in stderr, got %q", stderr.String())
	}
}

func TestDispatchCLI_UnexpectedSubcommandArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer

	action, exitCode := dispatchCLI([]string{"dashboard", "extra"}, &stdout, &stderr)

	if action != cliActionExit {
		t.Fatalf("expected cliActionExit, got %v", action)
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`clux dashboard: unexpected argument "extra"`)) {
		t.Fatalf("expected unexpected argument error, got %q", stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Usage:\n  clux dashboard")) {
		t.Fatalf("expected dashboard usage in stderr, got %q", stderr.String())
	}
}

func TestDispatchCLI_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	action, exitCode := dispatchCLI([]string{"unknown"}, &stdout, &stderr)

	if action != cliActionExit {
		t.Fatalf("expected cliActionExit, got %v", action)
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`clux: unknown command "unknown"`)) {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Usage:\n  clux [command]")) {
		t.Fatalf("expected root usage in stderr, got %q", stderr.String())
	}
}
