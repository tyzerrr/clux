package tmux

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tanaka0325/clux/internal/session"
)

// assert is a test helper that fails with a formatted message if the condition is false.
func assert(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Errorf(format, args...)
	}
}

// --- isWaiting ---

func TestIsWaiting(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"contains [Y/n]", "Delete file? [Y/n]", true},
		{"contains [y/n]", "Overwrite? [y/n]", true},
		{"contains [y/N]", "Continue? [y/N]", true},
		{"contains Do you want to proceed?", "Do you want to proceed?", true},
		{"contains (y/n)", "Proceed? (y/n)", true},
		{"contains (Y/n)", "Proceed? (Y/n)", true},
		{"contains (y)es / (n)o", "Choose: (y)es / (n)o", true},
		{"contains ? (yes/no)", "Overwrite? (yes/no)", true},
		{"Esc to cancel is not a waiting indicator", "Press Esc to cancel the operation", false},
		{"no waiting prompt", "-- INSERT --\nnormal output", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWaiting(tt.content)
			if got != tt.want {
				t.Errorf("isWaiting(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// --- bottomContent ---

func TestBottomContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{"fewer lines than n", "a\nb\nc", 5, "a\nb\nc"},
		{"exact lines", "a\nb\nc", 3, "a\nb\nc"},
		{"more lines than n", "a\nb\nc\nd\ne", 3, "c\nd\ne"},
		{"empty content", "", 5, ""},
		{"trailing blank lines trimmed", "a\nb\nc\n\n\n\n", 3, "a\nb\nc"},
		{"prompt followed by padding", "line1\nline2\nDo you want to proceed?\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n", 3, "line1\nline2\nDo you want to proceed?"},
		{"all blank lines", "\n\n\n\n", 3, ""},
		{"trailing whitespace-only lines", "a\nb\n   \n  \t\n", 3, "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bottomContent(tt.content, tt.n)
			if got != tt.want {
				t.Errorf("bottomContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- parseClaudeStatus ---

func TestParseClaudeStatus(t *testing.T) {
	tests := []struct {
		input  string
		wantSt session.Status
		wantOK bool
	}{
		{"working", session.StatusWorking, true},
		{"idle", session.StatusIdle, true},
		{"waiting", session.StatusWaiting, true},
		{"", session.StatusUnknown, false},
		{"unknown", session.StatusUnknown, false},
		{"WORKING", session.StatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			st, ok := parseClaudeStatus(tt.input)
			if st != tt.wantSt || ok != tt.wantOK {
				t.Errorf("parseClaudeStatus(%q) = (%v, %v), want (%v, %v)", tt.input, st, ok, tt.wantSt, tt.wantOK)
			}
		})
	}
}

// --- detectStatusWithHooksForSession (mocked) ---

// withMockedDeps sets up mocked dependencies for a test,
// restoring the originals when the test completes.
func withMockedDeps(t *testing.T,
	statusFn func(string, string, string) string,
	childrenFn func(string, string, string, processMaps) bool,
	hashChangedFn func(string, uint64) bool,
) {
	t.Helper()
	origStatus := getClaudeStatusFn
	origChildren := hasActiveChildrenFn
	origHash := contentChangedFn
	getClaudeStatusFn = statusFn
	hasActiveChildrenFn = childrenFn
	contentChangedFn = hashChangedFn
	t.Cleanup(func() {
		getClaudeStatusFn = origStatus
		hasActiveChildrenFn = origChildren
		contentChangedFn = origHash
	})
}

// Hash changed -> Working (regardless of content patterns)
func TestDetect_HashChanged_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return true },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
}

// Hash stable + waiting pattern -> Waiting
func TestDetect_HashStable_WaitingPattern(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nDo you want to proceed?\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
}

// Hash stable + active children -> Working (safety net)
func TestDetect_HashStable_ActiveChildren_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return true },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working, got %v", st)
}

// Hash stable + no waiting + no children -> Idle
func TestDetect_HashStable_NoWaiting_NoChildren_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle, got %v", st)
}

// Hook says "waiting", hash stable, no active children -> Waiting
func TestDetect_HookWaiting_HashStable_NoChildren(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWaiting, "expected Waiting, got %v", st)
}

// Hook says "waiting" but active children exist -> Working (agents running)
func TestDetect_HookWaiting_ActiveChildren_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string, _ processMaps) bool { return true },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (active children override hook waiting), got %v", st)
}

// Hash stable + "local agent still running" in body -> Working
func TestDetect_BackgroundAgent_StillRunning_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n✻ Cooked for 36s · 1 local agent still running\n❯ \n  [Opus 4.6 (1M context)]\n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (agent still running in body), got %v", st)
}

// Hash stable + "local agents still running" (plural) -> Working
func TestDetect_BackgroundAgents_Plural_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n✻ Cooked for 20s · 2 local agents still running\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (agents plural still running), got %v", st)
}

// Hash stable + "esc to interrupt" -> Working (tool execution)
func TestDetect_EscToInterrupt_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n❯ \n  [Opus 4.6 (1M context)]\n  ⏺ Bash(git status) esc to interrupt"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (esc to interrupt), got %v", st)
}

// Hash stable + spinner activity line -> Working
func TestDetect_SpinnerActivity_Working(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n⏺ Reading file…\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (spinner activity), got %v", st)
}

// Hash stable + agent still running + hook=waiting -> Working (agent overrides hook)
func TestDetect_BackgroundAgent_OverridesHookWaiting(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "waiting" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n✻ Baked for 10s · 1 local agent still running\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (agent overrides hook waiting), got %v", st)
}

// "local agent" in scrollback (beyond bottomScanLines) -> Idle, not false positive
func TestDetect_OldLocalAgentInScrollback_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "1 local agent still running\n" + strings.Repeat("filler line\n", 20) + "some output\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle (old agent text in scrollback), got %v", st)
}

// Truncated tool output line "⏺ Bash(long command…" must NOT trigger spinner detection
func TestDetect_TruncatedToolOutput_NotSpinner_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "some output\n⏺ Bash(git add internal/tmux/tmux.go internal/tmux/tmux_t…\n❯ \n  -- INSERT --"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle (truncated tool output, not spinner), got %v", st)
}

// Hook says "idle" -> ignored, hash takes priority
func TestDetect_HookIdle_Ignored_HashChanged(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "idle" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return true },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusWorking, "expected Working (hash changed despite hook idle), got %v", st)
}

// Hook says "working" -> not trusted, falls through to hash
func TestDetect_HookWorking_NotTrusted_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "working" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome output\n❯ "
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle (hook working not trusted, hash stable), got %v", st)
}

// Hash stable + no waiting pattern + no children + Claude Code present -> Idle (not Unknown)
func TestDetect_HashStable_ClaudeCodeNoPattern_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	content := "-- INSERT --\nsome unrecognized output"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle (hash stable, Claude Code present), got %v", st)
}

// Old waiting indicator in scrollback does not affect hash-based detection
func TestDetect_OldWaitingInScrollback_HashStable_Idle(t *testing.T) {
	withMockedDeps(t,
		func(_, _, _ string) string { return "" },
		func(_, _, _ string, _ processMaps) bool { return false },
		func(_ string, _ uint64) bool { return false },
	)
	// "Do you want to proceed?" is in scrollback (more than 15 lines up),
	// so bottomContent won't include it.
	content := "-- INSERT --\nDo you want to proceed?\n" + strings.Repeat("filler line\n", 20) + "some output\n"
	st := detectStatusWithHooksForSession(content, "clux", "0", "0", processMaps{}, hashContent(content))
	assert(t, st == session.StatusIdle, "expected Idle (old waiting in scrollback), got %v", st)
}

// --- parseProcessList ---

func TestParseProcessList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			"normal output",
			"  123   1 /usr/bin/bash\n  456 123 /usr/local/bin/claude\n  789 456 /usr/bin/node\n",
			3,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"malformed line skipped",
			"  123   1 /usr/bin/bash\nabc\n  789 456 /usr/bin/node\n",
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProcessList(tt.output)
			if len(got) != tt.want {
				t.Errorf("parseProcessList() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseProcessList_FieldValues(t *testing.T) {
	output := "  456 123 /usr/local/bin/claude\n"
	got := parseProcessList(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	p := got[0]
	if p.pid != 456 {
		t.Errorf("pid = %d, want 456", p.pid)
	}
	if p.ppid != 123 {
		t.Errorf("ppid = %d, want 123", p.ppid)
	}
	if p.comm != "claude" {
		t.Errorf("comm = %q, want %q", p.comm, "claude")
	}
}

func TestParseProcessList_MultiWordComm(t *testing.T) {
	output := "  100   1 /Applications/Self Service.app/Contents/MacOS/Self Service\n  200   1 /usr/local/bin/claude\n"
	got := parseProcessList(output)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].comm != "Self Service" {
		t.Errorf("comm = %q, want %q", got[0].comm, "Self Service")
	}
	if got[1].comm != "claude" {
		t.Errorf("comm = %q, want %q", got[1].comm, "claude")
	}
}

// --- buildProcessMaps ---

func TestBuildProcessMaps(t *testing.T) {
	procs := []processInfo{
		{pid: 1, ppid: 0, comm: "init"},
		{pid: 100, ppid: 1, comm: "bash"},
		{pid: 200, ppid: 100, comm: "claude"},
		{pid: 300, ppid: 200, comm: "node"},
	}
	commByPID, childrenByPID := buildProcessMaps(procs)

	if commByPID[100] != "bash" {
		t.Errorf("commByPID[100] = %q, want %q", commByPID[100], "bash")
	}
	if commByPID[200] != "claude" {
		t.Errorf("commByPID[200] = %q, want %q", commByPID[200], "claude")
	}
	if len(childrenByPID[100]) != 1 || childrenByPID[100][0] != 200 {
		t.Errorf("childrenByPID[100] = %v, want [200]", childrenByPID[100])
	}
}

// --- paneHasClaude ---

func TestPaneHasClaude(t *testing.T) {
	procs := []processInfo{
		{pid: 100, ppid: 1, comm: "bash"},
		{pid: 200, ppid: 100, comm: "claude"},
		{pid: 300, ppid: 1, comm: "vim"},
		{pid: 400, ppid: 300, comm: "node"},
		{pid: 500, ppid: 0, comm: "claude"}, // claude is pane_pid itself
	}
	commByPID, childrenByPID := buildProcessMaps(procs)

	tests := []struct {
		name    string
		panePID int
		want    bool
	}{
		{"child is claude", 100, true},
		{"no claude child", 300, false},
		{"pane_pid is claude", 500, true},
		{"nonexistent pid", 999, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneHasClaude(tt.panePID, commByPID, childrenByPID)
			if got != tt.want {
				t.Errorf("paneHasClaude(%d) = %v, want %v", tt.panePID, got, tt.want)
			}
		})
	}
}

// --- parsePaneListOutput ---

func TestParsePaneListOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			"normal output",
			"12345\tdev\t0\t0\teditor\t/home/user\n67890\twork\t1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"empty output",
			"",
			0,
		},
		{
			"malformed line skipped",
			"12345\tdev\t0\t0\teditor\t/home\nbadline\n67890\twork\t1\t0\tshell\t/tmp\n",
			2,
		},
		{
			"invalid window index skipped",
			"12345\tdev\tabc\t0\teditor\t/home\n67890\twork\t1\t0\tshell\t/tmp\n",
			1,
		},
		{
			"invalid pane index skipped",
			"12345\tdev\t0\tabc\teditor\t/home\n67890\twork\t1\t0\tshell\t/tmp\n",
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaneListOutput(tt.output)
			if len(got) != tt.want {
				t.Errorf("parsePaneListOutput() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParsePaneListOutput_FieldValues(t *testing.T) {
	output := "12345\tdev-session\t3\t1\tmy-window\t/home/user/code\n"
	got := parsePaneListOutput(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	p := got[0]
	if p.panePID != 12345 {
		t.Errorf("panePID = %d, want 12345", p.panePID)
	}
	if p.sessionName != "dev-session" {
		t.Errorf("sessionName = %q, want %q", p.sessionName, "dev-session")
	}
	if p.windowIndex != "3" {
		t.Errorf("windowIndex = %q, want %q", p.windowIndex, "3")
	}
	if p.paneIndex != "1" {
		t.Errorf("paneIndex = %q, want %q", p.paneIndex, "1")
	}
	if p.windowName != "my-window" {
		t.Errorf("windowName = %q, want %q", p.windowName, "my-window")
	}
	if p.dir != "/home/user/code" {
		t.Errorf("dir = %q, want %q", p.dir, "/home/user/code")
	}
}

// --- isAncestorOf ---

func TestIsAncestorOf(t *testing.T) {
	// Process tree: 1 -> 100 -> 200 -> 300
	childrenByPID := map[int][]int{
		1:   {100},
		100: {200},
		200: {300},
	}

	tests := []struct {
		name     string
		ancestor int
		target   int
		want     bool
	}{
		{"direct child", 100, 200, true},
		{"grandchild", 100, 300, true},
		{"not ancestor", 200, 100, false},
		{"same pid", 100, 100, false},
		{"nonexistent", 999, 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAncestorOf(tt.ancestor, tt.target, childrenByPID)
			if got != tt.want {
				t.Errorf("isAncestorOf(%d, %d) = %v, want %v", tt.ancestor, tt.target, got, tt.want)
			}
		})
	}
}

// --- findClaudePanes integration ---

func TestFindClaudePanes(t *testing.T) {
	myPID := os.Getpid()

	origPanes := listAllPanesFn
	origProcs := listProcessesFn
	t.Cleanup(func() {
		listAllPanesFn = origPanes
		listProcessesFn = origProcs
	})

	// Pane layout:
	//   pane 1000: shell with claude child → should be found
	//   pane 2000: claude as root process → should be found
	//   pane 3000: no claude → should be excluded
	//   pane myPID: clux itself → should be excluded (self-exclusion)
	listAllPanesFn = func() (string, error) {
		lines := strings.Join([]string{
			"1000\twork\t0\t0\tdev\t/home/user/project",
			"2000\tother\t1\t0\teditor\t/tmp/editor",
			"3000\twork\t2\t0\tshell\t/home/user",
			fmt.Sprintf("%d\tclux\t0\t0\tclux\t/home/user/clux", myPID),
		}, "\n")
		return lines, nil
	}

	listProcessesFn = func() (string, error) {
		lines := strings.Join([]string{
			fmt.Sprintf("%d   1 zsh", myPID), // clux's own process
			"1000   1 zsh",
			"1001 1000 claude",  // claude is child of pane 1000
			"2000   1 claude",   // claude IS pane 2000
			"3000   1 zsh",
			"3001 3000 vim",     // no claude in pane 3000
		}, "\n")
		return lines, nil
	}

	result, err := findClaudePanes()
	if err != nil {
		t.Fatalf("findClaudePanes() error: %v", err)
	}

	if len(result.panes) != 2 {
		t.Fatalf("expected 2 panes, got %d: %+v", len(result.panes), result.panes)
	}

	// Verify the two found panes
	sessions := map[string]bool{}
	for _, p := range result.panes {
		sessions[p.sessionName+":"+p.windowIndex] = true
	}
	if !sessions["work:0"] {
		t.Error("expected work:0 (shell with claude child) to be found")
	}
	if !sessions["other:1"] {
		t.Error("expected other:1 (claude as root) to be found")
	}

	// Verify process maps are returned
	if result.commByPID == nil {
		t.Error("expected commByPID to be non-nil")
	}
	if result.childrenByPID == nil {
		t.Error("expected childrenByPID to be non-nil")
	}
}
