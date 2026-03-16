package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tanaka0325/clux/internal/session"
)

// testModel creates a Model with test sessions set in both sessions and filtered.
func testModel(sessions []session.Session) Model {
	m := New()
	m.sessions = sessions
	m.filtered = sessions
	return m
}

var testSessions = []session.Session{
	{Name: "alpha-session", Summary: "auth refactor", Dir: "/home/user/projects/alpha", Status: session.StatusWorking, WindowIndex: "0"},
	{Name: "beta-session", Dir: "/home/user/projects/beta", Status: session.StatusIdle, WindowIndex: "1"},
	{Name: "gamma-session", Summary: "fix bug #42", Dir: "/home/user/projects/gamma", Status: session.StatusWaiting, WindowIndex: "2"},
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
	if rm.mode != ModeNewSessionBranch {
		t.Errorf("expected ModeNewSessionBranch, got %v", rm.mode)
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

// --- ModeNewSessionBranch tests ---

func TestModeNewSessionBranch_EmptyBranchCreatesWindowDirectly(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSessionBranch
	m.selectedRepoDir = os.TempDir()
	m.branchInput.SetValue("")
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := m.updateNewSessionBranch(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd for empty branch (direct window creation)")
	}
}

func TestModeNewSessionBranch_EscGoesBackToModeNewSession(t *testing.T) {
	m := testModel(testSessions)
	m.mode = ModeNewSessionBranch
	m.selectedRepoDir = os.TempDir()
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.updateNewSessionBranch(msg)
	rm := result.(Model)
	if rm.mode != ModeNewSession {
		t.Errorf("expected ModeNewSession, got %v", rm.mode)
	}
}

func TestView_ModeNewSessionBranch(t *testing.T) {
	m := New()
	m.mode = ModeNewSessionBranch
	m.selectedRepoDir = "/tmp/test-repo"
	view := m.View().Content
	if !strings.Contains(view, "Branch Name") {
		t.Error("expected view to contain 'Branch Name'")
	}
	if !strings.Contains(view, "skip worktree") {
		t.Error("expected view to contain 'skip worktree'")
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
