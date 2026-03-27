package tmux

import (
	"testing"

	"github.com/tanaka0325/clux/internal/session"
)

func resetPaneHashes(t *testing.T) {
	t.Helper()
	paneCacheMu.Lock()
	orig := paneContentHashes
	paneContentHashes = map[string]uint64{}
	origDetect := paneDetectCache
	paneDetectCache = map[string]paneDetectResult{}
	paneCacheMu.Unlock()
	t.Cleanup(func() {
		paneCacheMu.Lock()
		paneContentHashes = orig
		paneDetectCache = origDetect
		paneCacheMu.Unlock()
	})
}

// --- contentChanged ---

func TestContentChanged(t *testing.T) {
	resetPaneHashes(t)

	key := "test:0.0"
	helloHash := hashContent("hello")
	worldHash := hashContent("world")

	// First call — no previous hash, returns false
	if contentChanged(key, helloHash) {
		t.Error("first call should return false")
	}

	// Same content — returns false
	if contentChanged(key, helloHash) {
		t.Error("same content should return false")
	}

	// Different content — returns true
	if !contentChanged(key, worldHash) {
		t.Error("different content should return true")
	}

	// Same new content — returns false
	if contentChanged(key, worldHash) {
		t.Error("same content should return false")
	}
}

// Timer line changes (e.g., "Baked for 3m 16s" -> "3m 17s") should NOT cause hash change
func TestContentChanged_TimerIgnored(t *testing.T) {
	resetPaneHashes(t)

	key := "test:timer"
	base := "some output\n❯ \n  -- INSERT --"

	timerPairs := [][2]string{
		{"◆ Baked for 3m 16s", "◆ Baked for 3m 17s"},
		{"✻ Cogitated for 47s", "✻ Cogitated for 48s"},
		{"◆ Worked for 1m 0s", "◆ Worked for 1m 1s"},
	}
	for _, pair := range timerPairs {
		k := key + pair[0]
		content1 := pair[0] + "\n" + base
		content2 := pair[1] + "\n" + base
		contentChanged(k, hashContent(content1))
		if contentChanged(k, hashContent(content2)) {
			t.Errorf("timer-only change should not be detected for %q", pair[0])
		}
	}
}

// Real content change alongside timer should still be detected
func TestContentChanged_RealChangeWithTimer(t *testing.T) {
	resetPaneHashes(t)

	key := "test:timer2"
	base1 := "old output\n❯ \n  -- INSERT --"
	base2 := "new output\n❯ \n  -- INSERT --"

	content1 := "◆ Baked for 1s\n" + base1
	content2 := "◆ Baked for 2s\n" + base2

	contentChanged(key, hashContent(content1))
	if !contentChanged(key, hashContent(content2)) {
		t.Error("real content change should be detected even with timer")
	}
}

func TestClearPaneHash(t *testing.T) {
	resetPaneHashes(t)

	key := paneKey("test", "0", "0")
	contentChanged(key, hashContent("hello"))
	ClearPaneHash("test", "0", "0")

	// After clear, first call returns false again
	if contentChanged(key, hashContent("hello")) {
		t.Error("after clear, first call should return false")
	}

	// Clearing a non-existent key is a no-op (no panic).
	ClearPaneHash("nonexistent", "99", "99")
}

// --- contentHashUnchanged ---

func TestContentHashUnchanged(t *testing.T) {
	resetPaneHashes(t)

	key := "test:0.0"

	// No stored hash — returns (false, hash)
	unchanged, hash := contentHashUnchanged(key, "hello")
	if unchanged {
		t.Error("expected false when no stored hash exists")
	}
	if hash == 0 {
		t.Error("expected non-zero hash even when no stored hash exists")
	}

	// Store a hash via contentChanged
	contentChanged(key, hashContent("hello"))

	// Same content — returns (true, hash)
	unchanged, _ = contentHashUnchanged(key, "hello")
	if !unchanged {
		t.Error("expected true for unchanged content")
	}

	// Different content — returns (false, hash)
	unchanged, _ = contentHashUnchanged(key, "world")
	if unchanged {
		t.Error("expected false for changed content")
	}

	// contentHashUnchanged does not update the stored hash,
	// so checking the original content still matches
	unchanged, _ = contentHashUnchanged(key, "hello")
	if !unchanged {
		t.Error("expected true: contentHashUnchanged should not update stored hash")
	}
}

// --- paneDetectCache ---

func TestGetSetCachedResult(t *testing.T) {
	resetPaneHashes(t)

	key := "test:1.0"

	// No cached result initially
	_, ok := getCachedResult(key)
	if ok {
		t.Error("expected no cached result for new key")
	}

	// Set and retrieve
	expected := paneDetectResult{
		status:  session.StatusWorking,
		branch:  "main",
		summary: "test summary",
	}
	setCachedResult(key, expected)

	got, ok := getCachedResult(key)
	if !ok {
		t.Fatal("expected cached result to exist")
	}
	if got.status != expected.status {
		t.Errorf("status = %v, want %v", got.status, expected.status)
	}
	if got.branch != expected.branch {
		t.Errorf("branch = %q, want %q", got.branch, expected.branch)
	}
	if got.summary != expected.summary {
		t.Errorf("summary = %q, want %q", got.summary, expected.summary)
	}
}

// --- ClearAllPaneCache ---

func TestClearAllPaneCache(t *testing.T) {
	resetPaneHashes(t)

	// Populate both caches
	contentChanged("a:0.0", hashContent("content-a"))
	contentChanged("b:1.0", hashContent("content-b"))
	setCachedResult("a:0.0", paneDetectResult{status: session.StatusWorking})
	setCachedResult("b:1.0", paneDetectResult{status: session.StatusIdle})

	ClearAllPaneCache()

	// Hash cache should be empty
	unchanged, _ := contentHashUnchanged("a:0.0", "content-a")
	if unchanged {
		t.Error("expected hash cache to be cleared for a:0.0")
	}
	unchanged, _ = contentHashUnchanged("b:1.0", "content-b")
	if unchanged {
		t.Error("expected hash cache to be cleared for b:1.0")
	}

	// Detect cache should be empty
	if _, ok := getCachedResult("a:0.0"); ok {
		t.Error("expected detect cache to be cleared for a:0.0")
	}
	if _, ok := getCachedResult("b:1.0"); ok {
		t.Error("expected detect cache to be cleared for b:1.0")
	}
}

// --- ClearPaneHash clears detect cache ---

func TestClearPaneHash_AlsoClearsDetectCache(t *testing.T) {
	resetPaneHashes(t)

	contentChanged("sess:0.0", hashContent("content"))
	setCachedResult("sess:0.0", paneDetectResult{status: session.StatusIdle, branch: "main"})

	ClearPaneHash("sess", "0", "0")

	if _, ok := getCachedResult("sess:0.0"); ok {
		t.Error("expected detect cache to be cleared after ClearPaneHash")
	}
}

// --- clearPaneHashByPrefix clears detect cache ---

func TestClearPaneHashByPrefix_AlsoClearsDetectCache(t *testing.T) {
	resetPaneHashes(t)

	contentChanged("clux:5.0", hashContent("content1"))
	contentChanged("clux:5.1", hashContent("content2"))
	contentChanged("clux:6.0", hashContent("content3"))
	setCachedResult("clux:5.0", paneDetectResult{status: session.StatusWorking})
	setCachedResult("clux:5.1", paneDetectResult{status: session.StatusIdle})
	setCachedResult("clux:6.0", paneDetectResult{status: session.StatusWaiting})

	clearPaneHashByPrefix("clux:5.")

	// Entries with prefix "clux:5." should be cleared
	if _, ok := getCachedResult("clux:5.0"); ok {
		t.Error("expected clux:5.0 to be cleared")
	}
	if _, ok := getCachedResult("clux:5.1"); ok {
		t.Error("expected clux:5.1 to be cleared")
	}

	// Entry with different prefix should remain
	if _, ok := getCachedResult("clux:6.0"); !ok {
		t.Error("expected clux:6.0 to remain")
	}
}
