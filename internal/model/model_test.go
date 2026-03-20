package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tanaka0325/clux/internal/config"
	"github.com/tanaka0325/clux/internal/session"
	"github.com/tanaka0325/clux/internal/tmux"
)

// testModel creates a Model with test sessions set in both sessions and filtered.
func testModel(sessions []session.Session) Model {
	m := New()
	m.sessions = sessions
	m.filtered = sessions
	return m
}

var testSessions = []session.Session{
	{Name: "alpha-session", Summary: "auth refactor", Dir: "/home/user/projects/alpha", Branch: "feature/auth", Status: session.StatusWorking, WindowIndex: "0", PaneIndex: "0"},
	{Name: "beta-session", Dir: "/home/user/projects/beta", Branch: "main", Status: session.StatusIdle, WindowIndex: "1", PaneIndex: "0"},
	{Name: "gamma-session", Summary: "fix bug #42", Dir: "/home/user/projects/gamma", Branch: "fix/bug-42", Status: session.StatusWaiting, WindowIndex: "2", PaneIndex: "0"},
}

// --- Helper function tests ---

func TestApplyFilter_EmptyQuery(t *testing.T) {
	result := applyFilter(testSessions, "")
	if len(result) != len(testSessions) {
		t.Errorf("expected %d sessions, got %d", len(testSessions), len(result))
	}
}

func TestApplyFilter_MatchByName(t *testing.T) {
	result := applyFilter(testSessions, "alpha")
	if len(result) == 0 {
		t.Fatal("expected at least one match by name, got none")
	}
	found := false
	for _, s := range result {
		if s.Name == "alpha-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'alpha-session' in results")
	}
}

func TestApplyFilter_MatchByDir(t *testing.T) {
	result := applyFilter(testSessions, "gamma")
	if len(result) == 0 {
		t.Fatal("expected at least one match by dir, got none")
	}
	found := false
	for _, s := range result {
		if s.Name == "gamma-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'gamma-session' in results")
	}
}

func TestApplyFilter_MatchByBranch(t *testing.T) {
	result := applyFilter(testSessions, "feature/auth")
	if len(result) == 0 {
		t.Fatal("expected at least one match by branch, got none")
	}
	found := false
	for _, s := range result {
		if s.Name == "alpha-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'alpha-session' in results when filtering by branch")
	}
}

func TestApplyFilter_MatchByBranchPrefix(t *testing.T) {
	result := applyFilter(testSessions, "fix/")
	if len(result) == 0 {
		t.Fatal("expected at least one match by branch prefix, got none")
	}
	found := false
	for _, s := range result {
		if s.Name == "gamma-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'gamma-session' in results when filtering by branch prefix")
	}
}

func TestApplyFilter_NoMatch(t *testing.T) {
	result := applyFilter(testSessions, "zzznomatchzzz")
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestFilterDirs_EmptyQuery(t *testing.T) {
	dirs := []string{"/home/user/proj1", "/home/user/proj2", "/home/user/proj3"}
	result := filterDirs(dirs, "")
	if len(result) != len(dirs) {
		t.Errorf("expected %d dirs, got %d", len(dirs), len(result))
	}
}

func TestFilterDirs_FilterCorrectly(t *testing.T) {
	dirs := []string{"/home/user/alpha", "/home/user/beta", "/home/user/gamma"}
	result := filterDirs(dirs, "alpha")
	if len(result) == 0 {
		t.Fatal("expected at least one match, got none")
	}
	found := false
	for _, d := range result {
		if d == "/home/user/alpha" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected '/home/user/alpha' in results")
	}
}

func TestShortenDir_ShowsLastTwoSegments(t *testing.T) {
	dir := "/home/user/projects/myrepo"
	result := shortenDir(dir)
	expected := "projects/myrepo"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestShortenDir_ShortPath(t *testing.T) {
	dir := "/etc/something"
	result := shortenDir(dir)
	// Only 2 segments after splitting on "/" (["", "etc", "something"]) -> last 2 = "etc/something"
	expected := "etc/something"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- ModeList key handling tests ---

func TestModeList_JMovesDown(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: 'j', Text: "j"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", rm.cursor)
	}
}

func TestModeList_DownMovesDown(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", rm.cursor)
	}
}

func TestModeList_JWrapsAround(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = len(testSessions) - 1
	msg := tea.KeyPressMsg{Code: 'j', Text: "j"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != 0 {
		t.Errorf("expected cursor to wrap to 0, got %d", rm.cursor)
	}
}

func TestModeList_KMovesUp(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 1
	msg := tea.KeyPressMsg{Code: 'k', Text: "k"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", rm.cursor)
	}
}

func TestModeList_UpMovesUp(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 1
	msg := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", rm.cursor)
	}
}

func TestModeList_KWrapsAround(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: 'k', Text: "k"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.cursor != len(testSessions)-1 {
		t.Errorf("expected cursor to wrap to %d, got %d", len(testSessions)-1, rm.cursor)
	}
}

func TestModeList_ShiftKSetsConfirmKill(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: 'K', Text: "K", ShiftedCode: 'K', Mod: tea.ModShift}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeConfirmKill {
		t.Errorf("expected ModeConfirmKill, got %v", rm.mode)
	}
	if rm.confirmTarget != testSessions[0].DisplayName() {
		t.Errorf("expected confirmTarget=%q, got %q", testSessions[0].DisplayName(), rm.confirmTarget)
	}
	if rm.confirmWindowIndex != testSessions[0].WindowIndex {
		t.Errorf("expected confirmWindowIndex=%q, got %q", testSessions[0].WindowIndex, rm.confirmWindowIndex)
	}
}

func TestModeList_ShiftKWithEmptyList(t *testing.T) {
	m := testModel([]session.Session{})
	msg := tea.KeyPressMsg{Code: 'K', Text: "K", ShiftedCode: 'K', Mod: tea.ModShift}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeList_NSetsNewSession(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeNewSession {
		t.Errorf("expected ModeNewSession, got %v", rm.mode)
	}
}

func TestModeList_RReturnsCmd(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: 'R', Text: "R", ShiftedCode: 'R', Mod: tea.ModShift}
	_, cmd := m.updateList(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from R key")
	}
}

func TestModeList_SlashSetsFilterMode(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: '/', Text: "/"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeFilter {
		t.Errorf("expected ModeFilter, got %v", rm.mode)
	}
}

func TestModeList_EnterWithSessionsReturnsCmd(t *testing.T) {
	m := testModel(testSessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := m.updateList(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from enter key with sessions")
	}
}

func TestModeList_EnterWithEmptyListReturnsNilCmd(t *testing.T) {
	m := testModel([]session.Session{})
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := m.updateList(msg)
	if cmd != nil {
		t.Error("expected nil cmd from enter key with empty list")
	}
}

// --- ModeConfirmKill key handling tests ---

func TestModeConfirmKill_YGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeConfirmKill
	m.confirmTarget = "test-session"
	m.confirmWindowIndex = "0"
	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	result, cmd := m.updateConfirmKill(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.confirmTarget != "" {
		t.Errorf("expected confirmTarget cleared, got %q", rm.confirmTarget)
	}
	if rm.confirmWindowIndex != "" {
		t.Errorf("expected confirmWindowIndex cleared, got %q", rm.confirmWindowIndex)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from y key in ConfirmKill mode")
	}
}

func TestModeConfirmKill_NGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeConfirmKill
	m.confirmTarget = "test-session"
	m.confirmWindowIndex = "0"
	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	result, cmd := m.updateConfirmKill(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.confirmTarget != "" {
		t.Errorf("expected confirmTarget cleared, got %q", rm.confirmTarget)
	}
	if rm.confirmWindowIndex != "" {
		t.Errorf("expected confirmWindowIndex cleared, got %q", rm.confirmWindowIndex)
	}
	if cmd != nil {
		t.Error("expected nil cmd from n key in ConfirmKill mode")
	}
}

func TestModeConfirmKill_EscGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeConfirmKill
	m.confirmTarget = "test-session"
	m.confirmWindowIndex = "0"
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateConfirmKill(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.confirmTarget != "" {
		t.Errorf("expected confirmTarget cleared, got %q", rm.confirmTarget)
	}
	if rm.confirmWindowIndex != "" {
		t.Errorf("expected confirmWindowIndex cleared, got %q", rm.confirmWindowIndex)
	}
}

func TestModeConfirmKill_OtherKeyStays(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeConfirmKill
	m.confirmTarget = "test-session"
	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
	result, _ := m.updateConfirmKill(msg)
	rm := result.(Model)
	if rm.mode != ModeConfirmKill {
		t.Errorf("expected ModeConfirmKill, got %v", rm.mode)
	}
}

// --- ModeFilter key handling tests ---

func TestModeFilter_EnterGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeFilter
	_ = m.filterInput.Focus()
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result, _ := m.updateFilter(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeFilter_EscGoesBackToListAndClearsFilter(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeFilter
	_ = m.filterInput.Focus()
	m.filterInput.SetValue("somefilter")
	// filter down so filtered is a subset
	m.filtered = testSessions[:1]
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateFilter(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.filterInput.Value() != "" {
		t.Errorf("expected filter cleared, got %q", rm.filterInput.Value())
	}
	if len(rm.filtered) != len(testSessions) {
		t.Errorf("expected filtered reset to full sessions (%d), got %d", len(testSessions), len(rm.filtered))
	}
}

// --- ModeNewSession key handling tests ---

func TestModeNewSession_EscGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeNewSession_UpMovesCursorUp(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	m.newSessionCursor = 1
	msg := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != 0 {
		t.Errorf("expected newSessionCursor=0, got %d", rm.newSessionCursor)
	}
}

func TestModeNewSession_CtrlKMovesCursorUp(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	m.newSessionCursor = 2
	msg := tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != 1 {
		t.Errorf("expected newSessionCursor=1, got %d", rm.newSessionCursor)
	}
}

func TestModeNewSession_UpWrapsAround(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	m.newSessionCursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != len(m.filteredDirs)-1 {
		t.Errorf("expected newSessionCursor to wrap to %d, got %d", len(m.filteredDirs)-1, rm.newSessionCursor)
	}
}

func TestModeNewSession_DownMovesCursorDown(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	m.newSessionCursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != 1 {
		t.Errorf("expected newSessionCursor=1, got %d", rm.newSessionCursor)
	}
}

func TestModeNewSession_CtrlJMovesCursorDown(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/a", "/repo/b", "/repo/c"}
	m.newSessionCursor = 1
	msg := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != 2 {
		t.Errorf("expected newSessionCursor=2, got %d", rm.newSessionCursor)
	}
}

func TestModeNewSession_DownWrapsAround(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	dirs := []string{"/repo/a", "/repo/b", "/repo/c"}
	m.filteredDirs = dirs
	m.newSessionCursor = len(dirs) - 1
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.newSessionCursor != 0 {
		t.Errorf("expected newSessionCursor to wrap to 0, got %d", rm.newSessionCursor)
	}
}

// --- View output tests ---

func TestView_ModeListWithSessions(t *testing.T) {
	m := testModel(testSessions)
	m.width = 120
	m.height = 40
	view := m.View().Content
	if !strings.Contains(view, "Clux") {
		t.Error("expected view to contain 'Clux'")
	}
	for _, s := range testSessions {
		if !strings.Contains(view, s.DisplayName()) {
			t.Errorf("expected view to contain session display name %q", s.DisplayName())
		}
	}
	if !strings.Contains(view, "K:kill") {
		t.Error("expected view to contain 'K:kill' in help bar")
	}
}

func TestView_ModeListNoSessions(t *testing.T) {
	m := testModel([]session.Session{})
	view := m.View().Content
	if !strings.Contains(view, "No Claude Code sessions found") {
		t.Error("expected view to contain 'No Claude Code sessions found'")
	}
}

func TestView_ModeConfirmKill(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeConfirmKill
	m.confirmTarget = "alpha-session"
	view := m.View().Content
	if !strings.Contains(view, "Kill session") {
		t.Error("expected view to contain 'Kill session'")
	}
	if !strings.Contains(view, "y:kill") {
		t.Error("expected view to contain 'y:kill'")
	}
	if !strings.Contains(view, "n/Esc:cancel") {
		t.Error("expected view to contain 'n/Esc:cancel'")
	}
}

func TestView_ModeFilter(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeFilter
	_ = m.filterInput.Focus()
	view := m.View().Content
	if !strings.Contains(view, "Enter:apply") {
		t.Error("expected view to contain 'Enter:apply'")
	}
	if !strings.Contains(view, "Esc:clear") {
		t.Error("expected view to contain 'Esc:clear'")
	}
}

// --- Update top-level dispatch tests (Issue 13) ---

func TestUpdate_SessionsMsg(t *testing.T) {
	m := New()
	sessions := []session.Session{
		{Name: "test", Dir: "/tmp", Status: session.StatusIdle, WindowIndex: "0"},
	}
	msg := sessionsMsg(sessions)
	result, _ := m.Update(msg)
	rm := result.(Model)
	if len(rm.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(rm.sessions))
	}
	if rm.sessions[0].Name != "test" {
		t.Errorf("expected session name 'test', got %q", rm.sessions[0].Name)
	}
	if len(rm.filtered) != 1 {
		t.Errorf("expected 1 filtered session, got %d", len(rm.filtered))
	}
}

func TestUpdate_SessionsMsg_ClampsCursor(t *testing.T) {
	m := New()
	// Set cursor beyond the new sessions length
	m.cursor = 5
	sessions := []session.Session{
		{Name: "only-one", Dir: "/tmp", Status: session.StatusIdle, WindowIndex: "0"},
	}
	msg := sessionsMsg(sessions)
	result, _ := m.Update(msg)
	rm := result.(Model)
	if rm.cursor != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", rm.cursor)
	}
}

func TestUpdate_ErrMsg(t *testing.T) {
	m := New()
	msg := errMsg(fmt.Errorf("test error"))
	result, cmd := m.Update(msg)
	rm := result.(Model)
	if cmd != nil {
		t.Error("expected nil cmd from errMsg")
	}
	if rm.err == nil {
		t.Fatal("expected err to be set")
	}
	if rm.err.Error() != "test error" {
		t.Errorf("expected err='test error', got %q", rm.err.Error())
	}
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := New()
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	result, cmd := m.Update(msg)
	rm := result.(Model)
	if cmd != nil {
		t.Error("expected nil cmd from WindowSizeMsg")
	}
	if rm.width != 120 {
		t.Errorf("expected width=120, got %d", rm.width)
	}
	if rm.height != 40 {
		t.Errorf("expected height=40, got %d", rm.height)
	}
}

func TestUpdate_TickMsg_InListMode(t *testing.T) {
	m := New()
	m.mode = ModeList
	msg := tickMsg(time.Now())
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from tickMsg in ModeList")
	}
}

func TestUpdate_TickMsg_InConfirmKillMode(t *testing.T) {
	m := New()
	m.mode = ModeConfirmKill
	msg := tickMsg(time.Now())
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from tickMsg in ModeConfirmKill")
	}
}

// --- Improved quit tests (Issue 15) ---

func TestModeList_QReturnsQuitMsg(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: 'q', Text: "q"}
	_, cmd := m.updateList(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from q key")
	}
	// Execute the cmd and verify it returns a tea.QuitMsg.
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected cmd() to return tea.QuitMsg, got %T", result)
	}
}

func TestModeList_EscReturnsQuitMsg(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, cmd := m.updateList(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from esc key")
	}
	// Execute the cmd and verify it returns a tea.QuitMsg.
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected cmd() to return tea.QuitMsg, got %T", result)
	}
}

func TestUpdate_WindowKilledMsg(t *testing.T) {
	m := New()
	msg := windowKilledMsg{}
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from windowKilledMsg (should dispatch fetchSessionsCmd)")
	}
}

func TestUpdate_GhqDirsMsg(t *testing.T) {
	m := New()
	m.mode = ModeNewSession
	msg := ghqDirsMsg([]string{"/repo/a", "/repo/b"})
	result, _ := m.Update(msg)
	rm := result.(Model)
	if len(rm.repoDirs) != 2 {
		t.Fatalf("expected 2 repoDirs, got %d", len(rm.repoDirs))
	}
	if rm.repoDirs[0] != "/repo/a" {
		t.Errorf("expected repoDirs[0]='/repo/a', got %q", rm.repoDirs[0])
	}
	if rm.repoDirs[1] != "/repo/b" {
		t.Errorf("expected repoDirs[1]='/repo/b', got %q", rm.repoDirs[1])
	}
}

func TestUpdate_TickMsg_InModeFilter(t *testing.T) {
	m := New()
	m.mode = ModeFilter
	msg := tickMsg(time.Now())
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from tickMsg in ModeFilter (fetch should happen)")
	}
}

func TestUpdate_CtrlDotQuit(t *testing.T) {
	m := New()
	msg := tea.KeyPressMsg{Code: '.', Mod: tea.ModCtrl}
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd from ctrl+. key")
	}
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected cmd() to return tea.QuitMsg, got %T", result)
	}
}

func TestView_ModeNewSession(t *testing.T) {
	m := New()
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/repo/alpha", "/repo/beta", "/repo/charlie"}
	m.newSessionCursor = 1
	view := m.View().Content
	if !strings.Contains(view, "New Session") {
		t.Error("expected view to contain 'New Session'")
	}
	if !strings.Contains(view, "Enter:select") {
		t.Error("expected view to contain 'Enter:select'")
	}
	if !strings.Contains(view, "beta") {
		t.Error("expected view to contain 'beta' (the selected item)")
	}
}

func TestView_ModeNewSession_WithError(t *testing.T) {
	m := New()
	m.mode = ModeNewSession
	m.err = fmt.Errorf("test error")
	view := m.View().Content
	if !strings.Contains(view, "Error:") {
		t.Error("expected view to contain 'Error:'")
	}
}

func TestModeNewSession_EnterWithValidDir(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{os.TempDir()}
	m.newSessionCursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result, _ := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.mode != ModeNewSessionPrompt {
		t.Errorf("expected ModeNewSessionPrompt, got %v", rm.mode)
	}
	if rm.selectedRepoDir != os.TempDir() {
		t.Errorf("expected selectedRepoDir %q, got %q", os.TempDir(), rm.selectedRepoDir)
	}
}

func TestModeNewSession_EnterWithInvalidDir(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSession
	m.filteredDirs = []string{"/nonexistent/path/xyz"}
	m.newSessionCursor = 0
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result, cmd := m.updateNewSession(msg)
	rm := result.(Model)
	if rm.err == nil {
		t.Error("expected err to be set for invalid dir")
	}
	if cmd != nil {
		t.Error("expected nil cmd for invalid dir")
	}
}

// --- ModeNewSessionPrompt tests ---

func TestModeNewSessionPrompt_EmptyPromptCreatesWindowDirectly(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSessionPrompt
	m.selectedRepoDir = os.TempDir()
	m.newSessionPromptInput.SetValue("")
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := m.updateNewSessionPrompt(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd for empty prompt (direct window creation)")
	}
}

func TestModeNewSessionPrompt_WithPromptCreatesWindowSilently(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSessionPrompt
	m.selectedRepoDir = os.TempDir()
	m.newSessionPromptInput.SetValue("fix the bug")
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result, cmd := m.updateNewSessionPrompt(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd for prompt (silent window creation)")
	}
}

func TestModeNewSessionPrompt_EscGoesToModeList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSessionPrompt
	m.selectedRepoDir = os.TempDir()
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateNewSessionPrompt(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestView_ModeNewSessionPrompt(t *testing.T) {
	m := New()
	m.mode = ModeNewSessionPrompt
	m.selectedRepoDir = "/tmp/test-repo"
	view := m.View().Content
	if !strings.Contains(view, "Initial Prompt") {
		t.Error("expected view to contain 'Initial Prompt'")
	}
	if !strings.Contains(view, "skip prompt") {
		t.Error("expected view to contain 'skip prompt'")
	}
}

func TestNewSessionCreatedWithPromptMsg_SetsPendingPrompt(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeList
	msg := newSessionCreatedWithPromptMsg{windowIndex: "5", prompt: "fix the bug"}
	result, cmd := m.Update(msg)
	rm := result.(Model)
	if rm.pendingPrompt != "fix the bug" {
		t.Errorf("expected pendingPrompt 'fix the bug', got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "5" {
		t.Errorf("expected pendingPromptTarget '5', got %q", rm.pendingPromptTarget)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (fetch sessions)")
	}
}

func TestSessionsMsg_SendsPendingPromptWhenIdle(t *testing.T) {
	m := testModel(nil)
	m.pendingPrompt = "fix the bug"
	m.pendingPromptTarget = "5"
	m.pendingPromptDeadline = time.Now().Add(120 * time.Second)
	m.prevStatuses = make(map[string]session.Status)
	msg := sessionsMsg([]session.Session{
		{Name: "test", WindowIndex: "5", PaneIndex: "0", Status: session.StatusIdle, Dir: "/tmp"},
	})
	result, cmd := m.Update(msg)
	rm := result.(Model)
	if rm.pendingPrompt != "" {
		t.Errorf("expected pendingPrompt to be cleared, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "" {
		t.Errorf("expected pendingPromptTarget to be cleared, got %q", rm.pendingPromptTarget)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (send keys)")
	}
}

func TestSessionsMsg_DoesNotSendPendingPromptWhenNotIdle(t *testing.T) {
	m := testModel(nil)
	m.pendingPrompt = "fix the bug"
	m.pendingPromptTarget = "5"
	m.pendingPromptDeadline = time.Now().Add(120 * time.Second)
	m.prevStatuses = make(map[string]session.Status)
	msg := sessionsMsg([]session.Session{
		{Name: "test", WindowIndex: "5", PaneIndex: "0", Status: session.StatusWorking, Dir: "/tmp"},
	})
	result, _ := m.Update(msg)
	rm := result.(Model)
	if rm.pendingPrompt != "fix the bug" {
		t.Errorf("expected pendingPrompt to remain, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "5" {
		t.Errorf("expected pendingPromptTarget to remain, got %q", rm.pendingPromptTarget)
	}
}

func TestUpdate_SessionsMsg_PendingPromptTimeout(t *testing.T) {
	m := New()
	m.pendingPrompt = "test prompt"
	m.pendingPromptTarget = "5"
	// Set deadline in the past to simulate timeout.
	m.pendingPromptDeadline = time.Now().Add(-1 * time.Second)

	sessions := []session.Session{
		{Name: "target", Dir: "/tmp", Status: session.StatusWorking, WindowIndex: "5"},
	}
	msg := sessionsMsg(sessions)
	result, _ := m.Update(msg)
	rm := result.(Model)

	if rm.pendingPrompt != "" {
		t.Errorf("expected pendingPrompt cleared, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "" {
		t.Errorf("expected pendingPromptTarget cleared, got %q", rm.pendingPromptTarget)
	}
	if rm.pendingPromptDeadline != (time.Time{}) {
		t.Errorf("expected pendingPromptDeadline zeroed, got %v", rm.pendingPromptDeadline)
	}
	if rm.err == nil {
		t.Fatal("expected err to be set for timeout")
	}
	if !strings.Contains(rm.err.Error(), "timed out") {
		t.Errorf("expected timeout error message, got %q", rm.err.Error())
	}
}

func TestUpdate_SessionsMsg_PendingPromptTargetDisappeared(t *testing.T) {
	m := New()
	m.pendingPrompt = "test prompt"
	m.pendingPromptTarget = "5"
	m.pendingPromptDeadline = time.Now().Add(120 * time.Second)

	// Sessions list does NOT contain window index "5".
	sessions := []session.Session{
		{Name: "other", Dir: "/tmp", Status: session.StatusIdle, WindowIndex: "1"},
	}
	msg := sessionsMsg(sessions)
	result, _ := m.Update(msg)
	rm := result.(Model)

	if rm.pendingPrompt != "" {
		t.Errorf("expected pendingPrompt cleared, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "" {
		t.Errorf("expected pendingPromptTarget cleared, got %q", rm.pendingPromptTarget)
	}
	if rm.pendingPromptDeadline != (time.Time{}) {
		t.Errorf("expected pendingPromptDeadline zeroed, got %v", rm.pendingPromptDeadline)
	}
	if rm.err == nil {
		t.Fatal("expected err to be set for disappeared target")
	}
	if !strings.Contains(rm.err.Error(), "no longer exists") {
		t.Errorf("expected target disappeared error message, got %q", rm.err.Error())
	}
}

func TestUpdate_SessionsMsg_PendingPromptSentWhenIdle(t *testing.T) {
	m := New()
	m.pendingPrompt = "test prompt"
	m.pendingPromptTarget = "5"
	m.pendingPromptDeadline = time.Now().Add(120 * time.Second)

	sessions := []session.Session{
		{Name: "target", Dir: "/tmp", Status: session.StatusIdle, WindowIndex: "5"},
	}
	msg := sessionsMsg(sessions)
	result, cmd := m.Update(msg)
	rm := result.(Model)

	if rm.pendingPrompt != "" {
		t.Errorf("expected pendingPrompt cleared after send, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "" {
		t.Errorf("expected pendingPromptTarget cleared after send, got %q", rm.pendingPromptTarget)
	}
	if rm.pendingPromptDeadline != (time.Time{}) {
		t.Errorf("expected pendingPromptDeadline zeroed after send, got %v", rm.pendingPromptDeadline)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd when prompt is sent")
	}
}

func TestUpdate_SessionsMsg_PendingPromptWaitsWhileWorking(t *testing.T) {
	m := New()
	m.pendingPrompt = "test prompt"
	m.pendingPromptTarget = "5"
	m.pendingPromptDeadline = time.Now().Add(120 * time.Second)

	sessions := []session.Session{
		{Name: "target", Dir: "/tmp", Status: session.StatusWorking, WindowIndex: "5"},
	}
	msg := sessionsMsg(sessions)
	result, _ := m.Update(msg)
	rm := result.(Model)

	// Pending prompt should still be set (waiting for idle).
	if rm.pendingPrompt != "test prompt" {
		t.Errorf("expected pendingPrompt still set, got %q", rm.pendingPrompt)
	}
	if rm.pendingPromptTarget != "5" {
		t.Errorf("expected pendingPromptTarget still set, got %q", rm.pendingPromptTarget)
	}
	if rm.err != nil {
		t.Errorf("expected no error while waiting, got %v", rm.err)
	}
}

func TestView_ModeConfirmKill_WithError(t *testing.T) {
	m := New()
	m.mode = ModeConfirmKill
	m.confirmTarget = "test-session"
	m.err = fmt.Errorf("test error")
	view := m.View().Content
	if !strings.Contains(view, "Error:") {
		t.Error("expected view to contain 'Error:'")
	}
}

func TestResolvePaneIndex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "0"},
		{"0", "0"},
		{"1", "1"},
		{"42", "42"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got := resolvePaneIndex(tt.input)
			if got != tt.want {
				t.Errorf("resolvePaneIndex(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrevStatuses_KeyIncludesPaneIndex(t *testing.T) {
	m := testModel([]session.Session{
		{Name: "s1", WindowIndex: "0", PaneIndex: "0", Status: session.StatusWorking},
		{Name: "s2", WindowIndex: "0", PaneIndex: "1", Status: session.StatusIdle},
	})

	// Trigger sessionsMsg to populate prevStatuses.
	result, _ := m.Update(sessionsMsg(m.sessions))
	rm := result.(Model)

	// Verify that different panes of the same window get distinct keys.
	key0 := "clux:0.0"
	key1 := "clux:0.1"
	if _, ok := rm.prevStatuses[key0]; !ok {
		t.Errorf("expected prevStatuses to contain key %q", key0)
	}
	if _, ok := rm.prevStatuses[key1]; !ok {
		t.Errorf("expected prevStatuses to contain key %q", key1)
	}
}

func TestModeList_ShiftKSetsConfirmPaneIndex(t *testing.T) {
	sessions := []session.Session{
		{Name: "test", WindowIndex: "3", PaneIndex: "2", Status: session.StatusIdle},
	}
	m := testModel(sessions)
	m.mode = ModeList
	msg := tea.KeyPressMsg{Code: 'K', Text: "K", ShiftedCode: 'K', Mod: tea.ModShift}
	result, _ := m.updateList(msg)
	updated := result.(Model)
	if updated.confirmWindowIndex != "3" {
		t.Errorf("confirmWindowIndex = %q, want %q", updated.confirmWindowIndex, "3")
	}
	if updated.confirmPaneIndex != "2" {
		t.Errorf("confirmPaneIndex = %q, want %q", updated.confirmPaneIndex, "2")
	}
}
// --- sortByStatus tests ---

func TestSortByStatus(t *testing.T) {
	sessions := []session.Session{
		{Name: "idle", Status: session.StatusIdle},
		{Name: "unknown", Status: session.StatusUnknown},
		{Name: "waiting", Status: session.StatusWaiting},
		{Name: "working", Status: session.StatusWorking},
	}
	sortByStatus(sessions)
	expected := []string{"waiting", "working", "idle", "unknown"}
	for i, name := range expected {
		if sessions[i].Name != name {
			t.Errorf("position %d: expected %q, got %q", i, name, sessions[i].Name)
		}
	}
}

func TestSortByStatus_Empty(t *testing.T) {
	var sessions []session.Session
	sortByStatus(sessions) // should not panic
}

func TestSortByStatus_StableOrder(t *testing.T) {
	sessions := []session.Session{
		{Name: "a", Status: session.StatusWorking},
		{Name: "b", Status: session.StatusWorking},
		{Name: "c", Status: session.StatusWorking},
	}
	sortByStatus(sessions)
	if sessions[0].Name != "a" || sessions[1].Name != "b" || sessions[2].Name != "c" {
		t.Errorf("stable sort violated: got %s, %s, %s", sessions[0].Name, sessions[1].Name, sessions[2].Name)
	}
}

// --- statusPriority tests ---

func TestStatusPriority(t *testing.T) {
	tests := []struct {
		status session.Status
		want   int
	}{
		{session.StatusWaiting, 3},
		{session.StatusWorking, 2},
		{session.StatusIdle, 1},
		{session.StatusUnknown, 0},
	}
	for _, tt := range tests {
		got := statusPriority(tt.status)
		if got != tt.want {
			t.Errorf("statusPriority(%v) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

// --- statusSummary tests ---

func TestStatusSummary_Mixed(t *testing.T) {
	sessions := []session.Session{
		{Status: session.StatusWaiting},
		{Status: session.StatusWorking},
		{Status: session.StatusIdle},
	}
	got := statusSummary(sessions)
	if got != "1 Waiting, 1 Working, 1 Idle" {
		t.Errorf("expected %q, got %q", "1 Waiting, 1 Working, 1 Idle", got)
	}
}

func TestStatusSummary_Empty(t *testing.T) {
	got := statusSummary(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestStatusSummary_AllSame(t *testing.T) {
	sessions := []session.Session{
		{Status: session.StatusWorking},
		{Status: session.StatusWorking},
		{Status: session.StatusWorking},
	}
	got := statusSummary(sessions)
	if got != "3 Working" {
		t.Errorf("expected %q, got %q", "3 Working", got)
	}
}

// --- excludeRegistered tests ---

func TestExcludeRegistered_NilConfig(t *testing.T) {
	windows := []tmux.ExternalWindowInfo{
		{Session: "s1", WindowIndex: "1"},
	}
	result := excludeRegistered(windows, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 window, got %d", len(result))
	}
}

func TestExcludeRegistered_EmptyConfig(t *testing.T) {
	windows := []tmux.ExternalWindowInfo{
		{Session: "s1", WindowIndex: "1"},
	}
	result := excludeRegistered(windows, &config.Config{})
	if len(result) != 1 {
		t.Errorf("expected 1 window, got %d", len(result))
	}
}

func TestExcludeRegistered_FiltersRegistered(t *testing.T) {
	windows := []tmux.ExternalWindowInfo{
		{Session: "s1", WindowIndex: "1"},
		{Session: "s2", WindowIndex: "2"},
		{Session: "s3", WindowIndex: "3"},
	}
	cfg := &config.Config{
		ExternalSessions: []config.ExternalSession{
			{Session: "s1", Window: "1"},
			{Session: "s3", Window: "3"},
		},
	}
	result := excludeRegistered(windows, cfg)
	if len(result) != 1 {
		t.Fatalf("expected 1 window, got %d", len(result))
	}
	if result[0].Session != "s2" {
		t.Errorf("expected s2, got %s", result[0].Session)
	}
}

// --- filterExtWindows tests ---

func TestFilterExtWindows_EmptyQuery(t *testing.T) {
	windows := []tmux.ExternalWindowInfo{
		{Session: "s1", WindowIndex: "1", WindowName: "win1", Dir: "/tmp"},
		{Session: "s2", WindowIndex: "2", WindowName: "win2", Dir: "/home"},
	}
	result := filterExtWindows(windows, "")
	if len(result) != 2 {
		t.Errorf("expected 2 windows, got %d", len(result))
	}
}

func TestFilterExtWindows_MatchesQuery(t *testing.T) {
	windows := []tmux.ExternalWindowInfo{
		{Session: "alpha", WindowIndex: "1", WindowName: "win1", Dir: "/tmp"},
		{Session: "beta", WindowIndex: "2", WindowName: "win2", Dir: "/home"},
	}
	result := filterExtWindows(windows, "alpha")
	if len(result) == 0 {
		t.Fatal("expected at least one match")
	}
	if result[0].Session != "alpha" {
		t.Errorf("expected alpha, got %s", result[0].Session)
	}
}

// --- dashCols tests ---

func TestDashCols(t *testing.T) {
	// Build sessions to ensure n >= maxColsByWidth in all cases.
	manySessions := make([]session.Session, 10)
	for i := range manySessions {
		manySessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	tests := []struct {
		width int
		want  int
	}{
		{200, 4},  // >= 180 → maxColsByWidth=4
		{180, 4},  // >= 180 → maxColsByWidth=4
		{179, 3},  // >= 120 → maxColsByWidth=3
		{120, 3},  // >= 120 → maxColsByWidth=3
		{119, 2},  // >= 80 → maxColsByWidth=2
		{80, 2},   // >= 80 → maxColsByWidth=2
		{79, 1},   // < 80 → maxColsByWidth=1
		{50, 1},
	}
	for _, tt := range tests {
		m := testModel(manySessions)
		m.width = tt.width
		got := m.dashCols()
		if got != tt.want {
			t.Errorf("dashCols() with width=%d: got %d, want %d", tt.width, got, tt.want)
		}
	}
}

// --- applyFilter matching by summary ---

func TestApplyFilter_MatchBySummary(t *testing.T) {
	result := applyFilter(testSessions, "auth")
	found := false
	for _, s := range result {
		if s.Name == "alpha-session" && s.Summary == "auth refactor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected applyFilter to match 'alpha-session' by summary 'auth refactor'")
	}
}

// --- ModeList key 'p' (preview toggle) ---

func TestModeList_PTogglesPreview(t *testing.T) {
	m := testModel(testSessions)
	m.previewEnabled = false
	msg := tea.KeyPressMsg{Code: 'p', Text: "p"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if !rm.previewEnabled {
		t.Error("expected previewEnabled=true after first p")
	}

	// Toggle back
	result2, _ := rm.updateList(msg)
	rm2 := result2.(Model)
	if rm2.previewEnabled {
		t.Error("expected previewEnabled=false after second p")
	}
	if rm2.previewContent != "" {
		t.Error("expected previewContent cleared")
	}
}

// --- ModeList key 'd' (dashboard) ---

func TestModeList_DSwitchesToDashboard(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: 'd', Text: "d"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeDashboard {
		t.Errorf("expected ModeDashboard, got %v", rm.mode)
	}
	if rm.dashCursor != 0 {
		t.Errorf("expected dashCursor=0, got %d", rm.dashCursor)
	}
}

// --- ModeList key 'a' (add external) ---

func TestModeList_ASwitchesToAddExternal(t *testing.T) {
	m := testModel(testSessions)
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	result, _ := m.updateList(msg)
	rm := result.(Model)
	if rm.mode != ModeAddExternal {
		t.Errorf("expected ModeAddExternal, got %v", rm.mode)
	}
}

// --- ModeList key 'y' (approve waiting) ---

func TestModeList_YApproveWaiting(t *testing.T) {
	sessions := []session.Session{
		{Name: "waiting-session", Status: session.StatusWaiting, WindowIndex: "0"},
	}
	m := testModel(sessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	_, cmd := m.updateList(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd when approving waiting session")
	}
}

func TestModeList_YOnNonWaitingDoesNothing(t *testing.T) {
	sessions := []session.Session{
		{Name: "idle-session", Status: session.StatusIdle, WindowIndex: "0"},
	}
	m := testModel(sessions)
	m.cursor = 0
	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	_, cmd := m.updateList(msg)
	if cmd != nil {
		t.Error("expected nil cmd when y pressed on non-waiting session")
	}
}

// --- ModeAddExternal tests ---

func TestModeAddExternal_EscGoesBackToList(t *testing.T) {
	m := New()
	m.mode = ModeAddExternal
	m.cfg = &config.Config{}
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateAddExternal(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeAddExternal_UpDownMovesCursor(t *testing.T) {
	m := New()
	m.mode = ModeAddExternal
	m.cfg = &config.Config{}
	m.filteredExtWindows = []tmux.ExternalWindowInfo{
		{Session: "s1", WindowIndex: "1"},
		{Session: "s2", WindowIndex: "2"},
		{Session: "s3", WindowIndex: "3"},
	}
	m.addExtCursor = 0

	// Down
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.updateAddExternal(msg)
	rm := result.(Model)
	if rm.addExtCursor != 1 {
		t.Errorf("expected addExtCursor=1, got %d", rm.addExtCursor)
	}

	// Up
	msg = tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ = rm.updateAddExternal(msg)
	rm = result.(Model)
	if rm.addExtCursor != 0 {
		t.Errorf("expected addExtCursor=0, got %d", rm.addExtCursor)
	}

	// Up wraps
	msg = tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ = rm.updateAddExternal(msg)
	rm = result.(Model)
	if rm.addExtCursor != 2 {
		t.Errorf("expected addExtCursor=2 (wrap), got %d", rm.addExtCursor)
	}
}

// --- ModeDashboard tests ---

func TestModeDashboard_EscGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateDashboard(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeDashboard_DGoesBackToList(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	msg := tea.KeyPressMsg{Code: 'd', Text: "d"}
	result, _ := m.updateDashboard(msg)
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
}

func TestModeDashboard_QQuits(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	msg := tea.KeyPressMsg{Code: 'q', Text: "q"}
	_, cmd := m.updateDashboard(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from q")
	}
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", result)
	}
}

func TestModeDashboard_Navigation(t *testing.T) {
	// 6 sessions, width=180 → 4 cols (>=180); use width=160 → 3 cols (>=120)
	sessions := make([]session.Session, 6)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.mode = ModeDashboard
	m.width = 160  // >= 120 → 3 cols
	m.height = 100 // enough height so all 6 sessions fit on one page
	m.dashCursor = 0

	// Move right
	msg := tea.KeyPressMsg{Code: 'l', Text: "l"}
	result, _ := m.updateDashboard(msg)
	rm := result.(Model)
	if rm.dashCursor != 1 {
		t.Errorf("after l: expected dashCursor=1, got %d", rm.dashCursor)
	}

	// Move down (3 cols → cursor goes from 1 to 4)
	msg = tea.KeyPressMsg{Code: 'j', Text: "j"}
	result, _ = rm.updateDashboard(msg)
	rm = result.(Model)
	if rm.dashCursor != 4 {
		t.Errorf("after j: expected dashCursor=4, got %d", rm.dashCursor)
	}

	// Move left
	msg = tea.KeyPressMsg{Code: 'h', Text: "h"}
	result, _ = rm.updateDashboard(msg)
	rm = result.(Model)
	if rm.dashCursor != 3 {
		t.Errorf("after h: expected dashCursor=3, got %d", rm.dashCursor)
	}

	// Move up (3 → 0)
	msg = tea.KeyPressMsg{Code: 'k', Text: "k"}
	result, _ = rm.updateDashboard(msg)
	rm = result.(Model)
	if rm.dashCursor != 0 {
		t.Errorf("after k: expected dashCursor=0, got %d", rm.dashCursor)
	}
}

func TestModeDashboard_ShiftKSetsConfirmKill(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 200
	m.dashCursor = 0

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'K', Text: "K"})
	rm := result.(Model)
	if rm.mode != ModeConfirmKill {
		t.Errorf("expected ModeConfirmKill, got %v", rm.mode)
	}
	if rm.confirmWindowIndex != testSessions[0].WindowIndex {
		t.Errorf("expected confirmWindowIndex=%q, got %q", testSessions[0].WindowIndex, rm.confirmWindowIndex)
	}
}

func TestModeDashboard_ShiftKWithEmptyList(t *testing.T) {
	m := testModel(nil)
	m.mode = ModeDashboard
	m.width = 200

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'K', Text: "K"})
	rm := result.(Model)
	if rm.mode != ModeDashboard {
		t.Errorf("expected mode to remain ModeDashboard, got %v", rm.mode)
	}
}

func TestModeDashboard_NSetsNewSession(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 200

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'n', Text: "n"})
	rm := result.(Model)
	if rm.mode != ModeNewSession {
		t.Errorf("expected ModeNewSession, got %v", rm.mode)
	}
}

func TestModeDashboard_SlashSetsFilterMode(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 200

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: '/', Text: "/"})
	rm := result.(Model)
	if rm.mode != ModeFilter {
		t.Errorf("expected ModeFilter, got %v", rm.mode)
	}
}

func TestModeDashboard_BSetsBroadcastSelect(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 200

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'b', Text: "b"})
	rm := result.(Model)
	if rm.mode != ModeBroadcastSelect {
		t.Errorf("expected ModeBroadcastSelect, got %v", rm.mode)
	}
}

// ====================================================================
// ModeBroadcastSelect key handling tests
// ====================================================================

func broadcastModel(dirs []string) Model {
	m := New()
	m.mode = ModeBroadcastSelect
	m.broadcastDirs = dirs
	m.broadcastFiltered = dirs
	m.broadcastSelected = make(map[string]bool)
	_ = m.broadcastInput.Focus()
	return m
}

func TestModeBroadcastSelect_EscGoesBackToList(t *testing.T) {
	m := broadcastModel([]string{"/repo/a", "/repo/b"})
	m.broadcastSelected["/repo/a"] = true
	result, _ := m.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if len(rm.broadcastSelected) != 0 {
		t.Errorf("expected broadcastSelected cleared, got %d", len(rm.broadcastSelected))
	}
}

func TestModeBroadcastSelect_SpaceTogglesSelection(t *testing.T) {
	dirs := []string{"/repo/a", "/repo/b"}
	m := broadcastModel(dirs)
	m.broadcastCursor = 0

	// Toggle on
	result, _ := m.updateBroadcastSelect(tea.KeyPressMsg{Code: ' ', Text: " "})
	rm := result.(Model)
	if !rm.broadcastSelected["/repo/a"] {
		t.Error("expected /repo/a to be selected")
	}

	// Toggle off
	result, _ = rm.updateBroadcastSelect(tea.KeyPressMsg{Code: ' ', Text: " "})
	rm = result.(Model)
	if rm.broadcastSelected["/repo/a"] {
		t.Error("expected /repo/a to be deselected")
	}
}

func TestModeBroadcastSelect_UpDownMovesCursor(t *testing.T) {
	dirs := []string{"/repo/a", "/repo/b", "/repo/c"}
	m := broadcastModel(dirs)
	m.broadcastCursor = 0

	// Down
	result, _ := m.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyDown})
	rm := result.(Model)
	if rm.broadcastCursor != 1 {
		t.Errorf("expected cursor=1, got %d", rm.broadcastCursor)
	}

	// Up wraps
	rm.broadcastCursor = 0
	result, _ = rm.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyUp})
	rm = result.(Model)
	if rm.broadcastCursor != 2 {
		t.Errorf("expected cursor to wrap to 2, got %d", rm.broadcastCursor)
	}

	// Ctrl+J
	rm.broadcastCursor = 0
	result, _ = rm.updateBroadcastSelect(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	rm = result.(Model)
	if rm.broadcastCursor != 1 {
		t.Errorf("expected cursor=1 after ctrl+j, got %d", rm.broadcastCursor)
	}

	// Ctrl+K
	rm.broadcastCursor = 1
	result, _ = rm.updateBroadcastSelect(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	rm = result.(Model)
	if rm.broadcastCursor != 0 {
		t.Errorf("expected cursor=0 after ctrl+k, got %d", rm.broadcastCursor)
	}
}

func TestModeBroadcastSelect_EnterWithSelectionGoesToPrompt(t *testing.T) {
	dirs := []string{"/repo/a", "/repo/b"}
	m := broadcastModel(dirs)
	m.broadcastSelected["/repo/a"] = true

	result, cmd := m.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(Model)
	if rm.mode != ModeBroadcastPrompt {
		t.Errorf("expected ModeBroadcastPrompt, got %v", rm.mode)
	}
	if len(rm.broadcastTargets) != 1 {
		t.Errorf("expected 1 target, got %d", len(rm.broadcastTargets))
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (focus prompt)")
	}
}

func TestModeBroadcastSelect_EnterWithNoSelectionUsesCurrentItem(t *testing.T) {
	dirs := []string{"/repo/a", "/repo/b"}
	m := broadcastModel(dirs)
	m.broadcastCursor = 1

	result, _ := m.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(Model)
	if rm.mode != ModeBroadcastPrompt {
		t.Errorf("expected ModeBroadcastPrompt, got %v", rm.mode)
	}
	if len(rm.broadcastTargets) != 1 || rm.broadcastTargets[0].dir != "/repo/b" {
		t.Errorf("expected cursor item /repo/b as target, got %+v", rm.broadcastTargets)
	}
}

func TestModeBroadcastSelect_EnterWithEmptyListStays(t *testing.T) {
	m := broadcastModel([]string{})
	result, cmd := m.updateBroadcastSelect(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(Model)
	if rm.mode != ModeBroadcastSelect {
		t.Errorf("expected ModeBroadcastSelect, got %v", rm.mode)
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

// ====================================================================
// ModeBroadcastPrompt key handling tests
// ====================================================================

func TestModeBroadcastPrompt_EscGoesBackToSelect(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a"}}
	_ = m.broadcastPromptInput.Focus()

	result, cmd := m.updateBroadcastPrompt(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := result.(Model)
	if rm.mode != ModeBroadcastSelect {
		t.Errorf("expected ModeBroadcastSelect, got %v", rm.mode)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (focus broadcast input)")
	}
}

func TestModeBroadcastPrompt_EnterWithEmptyStays(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	_ = m.broadcastPromptInput.Focus()
	m.broadcastPromptInput.SetValue("")

	result, cmd := m.updateBroadcastPrompt(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(Model)
	if rm.mode != ModeBroadcastPrompt {
		t.Errorf("expected ModeBroadcastPrompt, got %v", rm.mode)
	}
	if cmd != nil {
		t.Error("expected nil cmd for empty prompt")
	}
}

func TestModeBroadcastPrompt_EnterWithTextReturnsCmd(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a"}}
	_ = m.broadcastPromptInput.Focus()
	m.broadcastPromptInput.SetValue("do the thing")

	result, cmd := m.updateBroadcastPrompt(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(Model)
	if rm.broadcastPrompt != "do the thing" {
		t.Errorf("expected broadcastPrompt='do the thing', got %q", rm.broadcastPrompt)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

// ====================================================================
// ModeBroadcastWait key handling tests
// ====================================================================

func TestModeBroadcastWait_EscCancels(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a", ready: false}}
	m.broadcastPrompt = "do stuff"

	result, _ := m.updateBroadcastWait(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.broadcastTargets != nil {
		t.Error("expected broadcastTargets cleared")
	}
	if rm.broadcastPrompt != "" {
		t.Errorf("expected broadcastPrompt cleared, got %q", rm.broadcastPrompt)
	}
}

func TestModeBroadcastWait_OtherKeyStays(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a"}}

	result, _ := m.updateBroadcastWait(tea.KeyPressMsg{Code: 'q', Text: "q"})
	rm := result.(Model)
	if rm.mode != ModeBroadcastWait {
		t.Errorf("expected ModeBroadcastWait, got %v", rm.mode)
	}
}

// ====================================================================
// Update message handling tests
// ====================================================================

func TestUpdate_PreviewMsg(t *testing.T) {
	m := testModel(testSessions)
	m.previewEnabled = true
	result, _ := m.Update(previewMsg("pane content here"))
	rm := result.(Model)
	if rm.previewContent != "pane content here" {
		t.Errorf("expected previewContent='pane content here', got %q", rm.previewContent)
	}
}

func TestUpdate_ExternalWindowsMsg(t *testing.T) {
	m := New()
	m.mode = ModeAddExternal
	windows := []tmux.ExternalWindowInfo{
		{Session: "main", WindowIndex: "1", WindowName: "editor", Dir: "/home"},
		{Session: "work", WindowIndex: "2", WindowName: "shell", Dir: "/tmp"},
	}
	result, _ := m.Update(externalWindowsMsg(windows))
	rm := result.(Model)
	if len(rm.externalWindows) != 2 {
		t.Errorf("expected 2 external windows, got %d", len(rm.externalWindows))
	}
	if len(rm.filteredExtWindows) != 2 {
		t.Errorf("expected 2 filtered windows, got %d", len(rm.filteredExtWindows))
	}
}

func TestUpdate_ExternalAddedMsg(t *testing.T) {
	m := New()
	m.mode = ModeAddExternal
	result, cmd := m.Update(externalAddedMsg{})
	rm := result.(Model)
	// externalAddedMsg is not handled in Update — check it doesn't panic
	_ = rm
	_ = cmd
}

func TestUpdate_DashPreviewsMsg(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	previews := map[int]string{0: "content0", 1: "content1"}
	result, _ := m.Update(dashPreviewsMsg(previews))
	rm := result.(Model)
	if len(rm.dashPreviews) != 2 {
		t.Errorf("expected 2 dash previews, got %d", len(rm.dashPreviews))
	}
	if rm.dashPreviews[0] != "content0" {
		t.Errorf("expected dashPreviews[0]='content0', got %q", rm.dashPreviews[0])
	}
}

func TestUpdate_SessionUnregistered(t *testing.T) {
	m := testModel(testSessions)
	_, cmd := m.Update(sessionUnregistered{})
	if cmd == nil {
		t.Error("expected non-nil cmd to refresh sessions")
	}
}

func TestUpdate_BroadcastGhqDirsMsg(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastSelect
	dirs := []string{"/repo/a", "/repo/b", "/repo/c"}
	result, _ := m.Update(broadcastGhqDirsMsg(dirs))
	rm := result.(Model)
	if len(rm.broadcastDirs) != 3 {
		t.Errorf("expected 3 broadcastDirs, got %d", len(rm.broadcastDirs))
	}
	if len(rm.broadcastFiltered) != 3 {
		t.Errorf("expected 3 broadcastFiltered, got %d", len(rm.broadcastFiltered))
	}
	if rm.broadcastCursor != 0 {
		t.Errorf("expected cursor reset to 0, got %d", rm.broadcastCursor)
	}
}

func TestUpdate_BroadcastTargetsResolvedMsg_WithTargets(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	targets := []broadcastTarget{
		{dir: "/repo/a", windowIndex: "1", paneIndex: "0"},
	}
	result, cmd := m.Update(broadcastTargetsResolvedMsg{targets: targets})
	rm := result.(Model)
	if rm.mode != ModeBroadcastWait {
		t.Errorf("expected ModeBroadcastWait, got %v", rm.mode)
	}
	if len(rm.broadcastTargets) != 1 {
		t.Errorf("expected 1 target, got %d", len(rm.broadcastTargets))
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestUpdate_BroadcastTargetsResolvedMsg_Empty(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	result, _ := m.Update(broadcastTargetsResolvedMsg{targets: nil})
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList when no targets, got %v", rm.mode)
	}
}

func TestUpdate_BroadcastTargetsUpdatedMsg(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a", ready: false}}
	updated := []broadcastTarget{{dir: "/repo/a", ready: true}}
	result, _ := m.Update(broadcastTargetsUpdatedMsg(updated))
	rm := result.(Model)
	if !rm.broadcastTargets[0].ready {
		t.Error("expected target to be ready after update")
	}
}

func TestUpdate_BroadcastTimeoutMsg(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a"}}
	m.broadcastPrompt = "do stuff"
	result, _ := m.Update(broadcastTimeoutMsg{})
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if rm.broadcastTargets != nil {
		t.Error("expected broadcastTargets cleared")
	}
	if rm.err == nil {
		t.Error("expected error set on timeout")
	}
}

func TestUpdate_BroadcastReadyMsg(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a", windowIndex: "1", paneIndex: "0", ready: true}}
	m.broadcastPrompt = "do stuff"
	result, cmd := m.Update(broadcastReadyMsg{})
	rm := result.(Model)
	if rm.mode != ModeList {
		t.Errorf("expected ModeList, got %v", rm.mode)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd to send keys")
	}
}

// ====================================================================
// View rendering tests
// ====================================================================

func TestView_ModeAddExternal(t *testing.T) {
	m := New()
	m.mode = ModeAddExternal
	m.width = 80
	m.height = 24
	m.externalWindows = []tmux.ExternalWindowInfo{
		{Session: "main", WindowIndex: "1", WindowName: "editor", Dir: "/home"},
	}
	m.filteredExtWindows = m.externalWindows
	out := m.View().Content
	if !strings.Contains(out, "Add External") && !strings.Contains(out, "external") && !strings.Contains(out, "Register") {
		// Check for any expected content - the view should render something
		if out == "" {
			t.Error("expected non-empty view for ModeAddExternal")
		}
	}
}

func TestView_ModeDashboard(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 120
	m.height = 40
	m.dashPreviews = map[int]string{0: "preview content"}
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view for ModeDashboard")
	}
}

func TestView_ModeBroadcastSelect(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastSelect
	m.width = 80
	m.height = 24
	m.broadcastDirs = []string{"/repo/a", "/repo/b"}
	m.broadcastFiltered = m.broadcastDirs
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view for ModeBroadcastSelect")
	}
}

func TestView_ModeBroadcastPrompt(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastPrompt
	m.width = 80
	m.height = 24
	m.broadcastTargets = []broadcastTarget{{dir: "/repo/a"}}
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view for ModeBroadcastPrompt")
	}
}

func TestView_ModeBroadcastWait(t *testing.T) {
	m := New()
	m.mode = ModeBroadcastWait
	m.width = 80
	m.height = 24
	m.broadcastTargets = []broadcastTarget{
		{dir: "/repo/a", windowIndex: "1", ready: false},
		{dir: "/repo/b", windowIndex: "2", ready: true},
	}
	m.broadcastPrompt = "run tests"
	out := m.View().Content
	if out == "" {
		t.Error("expected non-empty view for ModeBroadcastWait")
	}
}

// --- Feature A: Preview Scroll ---

func TestPreviewHeight(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	h := previewHeight(m)
	if h <= 0 {
		t.Errorf("expected previewHeight > 0, got %d", h)
	}
	if h > m.height {
		t.Errorf("previewHeight %d exceeds terminal height %d", h, m.height)
	}
}

func TestPreviewHeight_ShortTerminal(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 5 // too short to show preview
	h := previewHeight(m)
	// topLines = 2+1+3+1 = 7, which > height, so h <= 0
	if h > 0 {
		t.Errorf("expected previewHeight <= 0 for short terminal, got %d", h)
	}
}

func TestPreviewScrollStep(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	step := previewScrollStep(m)
	if step <= 0 {
		t.Errorf("expected previewScrollStep > 0, got %d", step)
	}
	h := previewHeight(m)
	if h >= 2 && step != h/2 {
		t.Errorf("expected previewScrollStep = %d (h/2), got %d", h/2, step)
	}
}

func TestPreviewScrollStep_SmallHeight(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 5
	step := previewScrollStep(m)
	if step != 1 {
		t.Errorf("expected previewScrollStep=1 when previewHeight < 2, got %d", step)
	}
}

func TestPreviewScrollOffset_ResetOnCursorMove(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 10

	// Move cursor down, offset should reset
	result, _ := m.updateList(tea.KeyPressMsg{Code: 'j', Text: "j"})
	rm := result.(Model)
	if rm.previewScrollOffset != 0 {
		t.Errorf("expected previewScrollOffset=0 after cursor move, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScrollOffset_ResetOnCursorMoveUp(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 5
	m.cursor = 1

	result, _ := m.updateList(tea.KeyPressMsg{Code: 'k', Text: "k"})
	rm := result.(Model)
	if rm.previewScrollOffset != 0 {
		t.Errorf("expected previewScrollOffset=0 after cursor move up, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScrollOffset_ResetOnPreviewToggleOff(t *testing.T) {
	m := testModel(testSessions)
	m.previewEnabled = true
	m.previewScrollOffset = 7
	m.previewContent = "some content"

	result, _ := m.updateList(tea.KeyPressMsg{Code: 'p', Text: "p"})
	rm := result.(Model)
	if rm.previewScrollOffset != 0 {
		t.Errorf("expected previewScrollOffset=0 after toggle off, got %d", rm.previewScrollOffset)
	}
	if rm.previewEnabled {
		t.Error("expected previewEnabled=false after toggle")
	}
}

func TestPreviewScrollOffset_ResetOnDashboardEntry(t *testing.T) {
	m := testModel(testSessions)
	m.width = 200
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 10

	result, _ := m.updateList(tea.KeyPressMsg{Code: 'd', Text: "d"})
	rm := result.(Model)
	if rm.mode != ModeDashboard {
		t.Errorf("expected ModeDashboard, got %v", rm.mode)
	}
	if rm.previewScrollOffset != 0 {
		t.Errorf("expected previewScrollOffset=0 after entering dashboard, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScroll_CtrlU_IncreasesOffset(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 0

	result, _ := m.updateList(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'u'})
	rm := result.(Model)
	if rm.previewScrollOffset <= 0 {
		t.Errorf("expected previewScrollOffset > 0 after Ctrl+U, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScroll_CtrlD_DecreasesOffset(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 20

	result, _ := m.updateList(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'd'})
	rm := result.(Model)
	if rm.previewScrollOffset >= 20 {
		t.Errorf("expected previewScrollOffset < 20 after Ctrl+D, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScroll_CtrlD_ClampsAtZero(t *testing.T) {
	m := testModel(testSessions)
	m.width = 80
	m.height = 40
	m.previewEnabled = true
	m.previewScrollOffset = 1 // less than scroll step

	result, _ := m.updateList(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'd'})
	rm := result.(Model)
	if rm.previewScrollOffset < 0 {
		t.Errorf("expected previewScrollOffset >= 0, got %d", rm.previewScrollOffset)
	}
}

func TestPreviewScroll_NoOpWhenPreviewDisabled(t *testing.T) {
	m := testModel(testSessions)
	m.previewEnabled = false
	m.previewScrollOffset = 0

	result, _ := m.updateList(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'u'})
	rm := result.(Model)
	if rm.previewScrollOffset != 0 {
		t.Errorf("expected previewScrollOffset=0 when preview disabled, got %d", rm.previewScrollOffset)
	}
}

// --- Feature B: Dashboard Layout Improvements ---

func TestDashMaxVisible(t *testing.T) {
	sessions := make([]session.Session, 10)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.width = 160
	m.height = 50
	maxVisible := m.dashMaxVisible()
	if maxVisible <= 0 {
		t.Errorf("expected dashMaxVisible > 0, got %d", maxVisible)
	}
}

func TestDashMaxVisible_ShortTerminal(t *testing.T) {
	sessions := make([]session.Session, 5)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.width = 80
	m.height = 5 // very short
	maxVisible := m.dashMaxVisible()
	// Should return at least cols (one row fallback)
	cols := m.dashCols()
	if maxVisible < cols {
		t.Errorf("expected dashMaxVisible >= cols (%d), got %d", cols, maxVisible)
	}
}

func TestDashCols_WithFewSessions(t *testing.T) {
	// With only 1 session, dashCols should return 1 regardless of width
	m := testModel([]session.Session{
		{Name: "solo", Status: session.StatusIdle, WindowIndex: "0"},
	})
	m.width = 200
	got := m.dashCols()
	if got != 1 {
		t.Errorf("expected dashCols=1 with 1 session, got %d", got)
	}
}

func TestDashCols_WithTwoSessions(t *testing.T) {
	sessions := []session.Session{
		{Name: "s0", Status: session.StatusIdle, WindowIndex: "0"},
		{Name: "s1", Status: session.StatusIdle, WindowIndex: "1"},
	}
	m := testModel(sessions)
	m.width = 200 // maxColsByWidth=4, but n=2 → should cap at 2
	got := m.dashCols()
	if got != 2 {
		t.Errorf("expected dashCols=2 with 2 sessions on wide terminal, got %d", got)
	}
}

func TestDashboard_FocusModeToggle(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 160
	m.height = 50
	m.dashCursor = 0

	// Press f to enter focus mode
	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'f', Text: "f"})
	rm := result.(Model)
	if !rm.dashFocused {
		t.Error("expected dashFocused=true after f")
	}

	// Press f again to exit focus mode
	result2, _ := rm.updateDashboard(tea.KeyPressMsg{Code: 'f', Text: "f"})
	rm2 := result2.(Model)
	if rm2.dashFocused {
		t.Error("expected dashFocused=false after second f")
	}
}

func TestDashboard_EscExitsFocusMode(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 160
	m.height = 50
	m.dashFocused = true

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := result.(Model)
	if rm.dashFocused {
		t.Error("expected dashFocused=false after Esc")
	}
	if rm.mode != ModeDashboard {
		t.Errorf("expected mode to remain ModeDashboard after Esc from focus, got %v", rm.mode)
	}
}

func TestDashboard_Pagination_NextPage(t *testing.T) {
	// Create enough sessions to require pagination
	sessions := make([]session.Session, 20)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.mode = ModeDashboard
	m.width = 80  // 1 col
	m.height = 20 // limited height forces pagination
	m.dashPageOffset = 0

	maxVisible := m.dashMaxVisible()
	if maxVisible >= 20 {
		t.Skip("terminal too large for pagination test")
	}

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: ']', Text: "]"})
	rm := result.(Model)
	if rm.dashPageOffset != maxVisible {
		t.Errorf("expected dashPageOffset=%d after ], got %d", maxVisible, rm.dashPageOffset)
	}
	if rm.dashCursor != 0 {
		t.Errorf("expected dashCursor=0 after page change, got %d", rm.dashCursor)
	}
}

func TestDashboard_Pagination_PrevPage(t *testing.T) {
	sessions := make([]session.Session, 20)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.mode = ModeDashboard
	m.width = 80
	m.height = 20
	maxVisible := m.dashMaxVisible()
	m.dashPageOffset = maxVisible // start on page 2
	m.dashCursor = 0

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: '[', Text: "["})
	rm := result.(Model)
	if rm.dashPageOffset != 0 {
		t.Errorf("expected dashPageOffset=0 after [, got %d", rm.dashPageOffset)
	}
}

func TestDashboard_Pagination_PrevPage_AtStart(t *testing.T) {
	sessions := make([]session.Session, 5)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.mode = ModeDashboard
	m.width = 80
	m.height = 20
	m.dashPageOffset = 0 // already on first page

	result, _ := m.updateDashboard(tea.KeyPressMsg{Code: '[', Text: "["})
	rm := result.(Model)
	if rm.dashPageOffset != 0 {
		t.Errorf("expected dashPageOffset=0 (no change), got %d", rm.dashPageOffset)
	}
}

func TestDashboard_ViewFocusMode(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeDashboard
	m.width = 80
	m.height = 24
	m.dashFocused = true
	m.dashCursor = 0
	m.dashPreviews = map[int]string{0: "preview content here"}

	view := m.viewDashboard(&strings.Builder{})
	// The focused session is filtered[dashPageOffset + dashCursor] = filtered[0]
	// DisplayName() returns Summary if non-empty, else Name
	focusedSession := m.filtered[0]
	expectedDisplay := focusedSession.DisplayName()
	if !strings.Contains(view, expectedDisplay) {
		t.Errorf("focus mode view should contain display name %q, got:\n%s", expectedDisplay, view)
	}
	if !strings.Contains(view, "exit-focus") {
		t.Error("focus mode help bar should mention exit-focus")
	}
}

func TestDashboard_ViewShowsPageIndicator(t *testing.T) {
	sessions := make([]session.Session, 20)
	for i := range sessions {
		sessions[i] = session.Session{Name: fmt.Sprintf("s%d", i), Status: session.StatusIdle, WindowIndex: fmt.Sprintf("%d", i)}
	}
	m := testModel(sessions)
	m.mode = ModeDashboard
	m.width = 80
	m.height = 20
	m.dashPageOffset = 0

	var b strings.Builder
	view := m.viewDashboard(&b)
	if m.dashMaxVisible() < 20 {
		if !strings.Contains(view, "Page") {
			t.Error("expected 'Page' indicator in dashboard header when multiple pages exist")
		}
	}
}

// --- Grouping tests ---

func TestGroupKey_WithGhqRoot(t *testing.T) {
	ghqRoot := "/Users/h-tanaka/go/src/"
	s := session.Session{Dir: "/Users/h-tanaka/go/src/github.com/tanaka0325/clux"}
	key := groupKey(s, ghqRoot)
	if key != "github.com/tanaka0325/clux" {
		t.Errorf("expected 'github.com/tanaka0325/clux', got %q", key)
	}
}

func TestGroupKey_WithoutGhqRoot(t *testing.T) {
	s := session.Session{Dir: "/some/random/path/to/project"}
	key := groupKey(s, "")
	if key != "to/project" {
		t.Errorf("expected 'to/project', got %q", key)
	}
}

func TestGroupKey_External(t *testing.T) {
	s := session.Session{External: true, Dir: "/any/dir"}
	key := groupKey(s, "/some/root/")
	if key != "External" {
		t.Errorf("expected 'External', got %q", key)
	}
}

func TestGroupKey_WorktreeStripping(t *testing.T) {
	ghqRoot := "/Users/h-tanaka/go/src/"
	s := session.Session{Dir: "/Users/h-tanaka/go/src/github.com/tanaka0325/clux/.claude/worktrees/agent-abc123"}
	key := groupKey(s, ghqRoot)
	if key != "github.com/tanaka0325/clux" {
		t.Errorf("expected 'github.com/tanaka0325/clux', got %q", key)
	}
}

func TestGroupKey_DirNotUnderGhqRoot(t *testing.T) {
	ghqRoot := "/Users/h-tanaka/go/src/"
	s := session.Session{Dir: "/opt/projects/my-app"}
	key := groupKey(s, ghqRoot)
	if key != "projects/my-app" {
		t.Errorf("expected 'projects/my-app', got %q", key)
	}
}

func TestGroupKey_ShortPath(t *testing.T) {
	s := session.Session{Dir: "/root"}
	key := groupKey(s, "")
	// len(parts) >= 2: ["", "root"] -> "/root"
	if key != "/root" {
		t.Errorf("expected '/root', got %q", key)
	}
}

func TestBuildGroups_CorrectGrouping(t *testing.T) {
	sessions := []session.Session{
		{Name: "s1", Dir: "/home/user/go/src/github.com/owner/repo-a", Status: session.StatusIdle},
		{Name: "s2", Dir: "/home/user/go/src/github.com/owner/repo-b", Status: session.StatusWorking},
		{Name: "s3", Dir: "/home/user/go/src/github.com/owner/repo-a", Status: session.StatusWaiting},
	}
	ghqRoot := "/home/user/go/src/"
	groups := buildGroups(sessions, ghqRoot)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Groups should be sorted by activity: repo-a has Waiting (3), repo-b has Working (2).
	if groups[0].name != "github.com/owner/repo-a" {
		t.Errorf("expected first group 'github.com/owner/repo-a', got %q", groups[0].name)
	}
	if len(groups[0].sessions) != 2 {
		t.Errorf("expected 2 sessions in first group, got %d", len(groups[0].sessions))
	}
	if groups[1].name != "github.com/owner/repo-b" {
		t.Errorf("expected second group 'github.com/owner/repo-b', got %q", groups[1].name)
	}
	if len(groups[1].sessions) != 1 {
		t.Errorf("expected 1 session in second group, got %d", len(groups[1].sessions))
	}
}

func TestBuildGroups_IndexPreserved(t *testing.T) {
	sessions := []session.Session{
		{Name: "s0", Dir: "/home/user/projects/a", Status: session.StatusIdle},
		{Name: "s1", Dir: "/home/user/projects/b", Status: session.StatusIdle},
		{Name: "s2", Dir: "/home/user/projects/a", Status: session.StatusIdle},
	}
	groups := buildGroups(sessions, "")

	for _, g := range groups {
		for _, is := range g.sessions {
			if sessions[is.index].Name != is.session.Name {
				t.Errorf("index %d points to %q but session is %q", is.index, sessions[is.index].Name, is.session.Name)
			}
		}
	}
}

func TestGroupPriority(t *testing.T) {
	tests := []struct {
		name     string
		statuses []session.Status
		expected int
	}{
		{"waiting_highest", []session.Status{session.StatusIdle, session.StatusWaiting}, 3},
		{"working_only", []session.Status{session.StatusWorking}, 2},
		{"idle_only", []session.Status{session.StatusIdle}, 1},
		{"unknown_only", []session.Status{session.StatusUnknown}, 0},
		{"mixed", []session.Status{session.StatusUnknown, session.StatusIdle, session.StatusWorking}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sessions []indexedSession
			for i, st := range tc.statuses {
				sessions = append(sessions, indexedSession{index: i, session: session.Session{Status: st}})
			}
			g := sessionGroup{name: "test", sessions: sessions}
			if got := groupPriority(g); got != tc.expected {
				t.Errorf("expected priority %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestGroupedSessionRows(t *testing.T) {
	groups := []sessionGroup{
		{name: "group-a", sessions: []indexedSession{{}, {}, {}}},
		{name: "group-b", sessions: []indexedSession{{}}},
	}
	// 2 headers + 4 sessions = 6
	if got := groupedSessionRows(groups); got != 6 {
		t.Errorf("expected 6 rows, got %d", got)
	}
}

func TestToggleGroupKey(t *testing.T) {
	m := testModel(testSessions)
	if m.GroupEnabled() {
		t.Fatal("groupEnabled should default to false")
	}

	// Press 'g' to enable.
	m2, _ := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = m2.(Model)
	if !m.GroupEnabled() {
		t.Error("expected groupEnabled to be true after pressing 'g'")
	}

	// Press 'g' again to disable.
	m3, _ := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = m3.(Model)
	if m.GroupEnabled() {
		t.Error("expected groupEnabled to be false after pressing 'g' again")
	}
}

func TestViewGroupedRendering(t *testing.T) {
	sessions := []session.Session{
		{Name: "s1", Dir: "/home/user/go/src/github.com/owner/repo-a", Status: session.StatusIdle, WindowIndex: "0", PaneIndex: "0"},
		{Name: "s2", Dir: "/home/user/go/src/github.com/owner/repo-b", Status: session.StatusWorking, WindowIndex: "1", PaneIndex: "0"},
		{Name: "s3", Dir: "/home/user/go/src/github.com/owner/repo-a", Status: session.StatusWaiting, WindowIndex: "2", PaneIndex: "0"},
	}
	m := testModel(sessions)
	m.groupEnabled = true
	m.ghqRoot = "/home/user/go/src/"
	m.width = 120
	m.height = 40

	view := m.View()
	viewStr := fmt.Sprint(view)

	// Verify group headers appear.
	if !strings.Contains(viewStr, "repo-a") {
		t.Error("expected group header containing 'repo-a' in view")
	}
	if !strings.Contains(viewStr, "repo-b") {
		t.Error("expected group header containing 'repo-b' in view")
	}
	// Verify session names appear.
	if !strings.Contains(viewStr, "s1") {
		t.Error("expected session 's1' in view")
	}
	if !strings.Contains(viewStr, "s2") {
		t.Error("expected session 's2' in view")
	}
	if !strings.Contains(viewStr, "s3") {
		t.Error("expected session 's3' in view")
	}
}

func TestViewGroupedHelpBar(t *testing.T) {
	m := testModel(testSessions)
	m.width = 120
	m.height = 40

	// Flat mode: help bar should show "g:group".
	view := m.View()
	viewStr := fmt.Sprint(view)
	if !strings.Contains(viewStr, "g:group") {
		t.Error("expected 'g:group' in help bar when grouping is off")
	}

	// Grouped mode: help bar should show "g:group(on)".
	m.groupEnabled = true
	view = m.View()
	viewStr = fmt.Sprint(view)
	if !strings.Contains(viewStr, "g:group(on)") {
		t.Error("expected 'g:group(on)' in help bar when grouping is on")
	}
}

func TestBuildGroups_WorktreeSessionsGroupTogether(t *testing.T) {
	sessions := []session.Session{
		{Name: "main", Dir: "/home/user/go/src/github.com/owner/repo", Status: session.StatusIdle},
		{Name: "wt1", Dir: "/home/user/go/src/github.com/owner/repo/.claude/worktrees/agent-abc", Status: session.StatusWorking},
	}
	ghqRoot := "/home/user/go/src/"
	groups := buildGroups(sessions, ghqRoot)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group (worktree sessions should group with base repo), got %d", len(groups))
	}
	if groups[0].name != "github.com/owner/repo" {
		t.Errorf("expected group name 'github.com/owner/repo', got %q", groups[0].name)
	}
	if len(groups[0].sessions) != 2 {
		t.Errorf("expected 2 sessions in group, got %d", len(groups[0].sessions))
	}
}
