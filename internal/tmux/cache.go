package tmux

import (
	"hash/fnv"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/tanaka0325/clux/internal/session"
)

// idleTimerPattern matches Claude Code's idle timer lines that update
// every second (e.g., "◆ Baked for 3m 16s", "✻ Cogitated for 47s").
// Claude Code uses many timer words (Baked, Cooked, Cogitated, Sauteed, …)
// so we match the structural pattern: a non-ASCII bullet char, a
// capitalized word, "for", and a duration with time units.
var idleTimerPattern = regexp.MustCompile(`(?m)^\s*[^\x00-\x7F] [A-Z]\w* for \d+[smh].*$`)

// paneContentHashes stores the FNV-1a hash of the last captured pane content.
// Key format: "sessionName:windowIndex.paneIndex"
// paneCacheMu guards both paneContentHashes and paneDetectCache so that
// clearing operations are atomic and no goroutine can observe a partially
// cleared state.
var (
	paneCacheMu       sync.Mutex
	paneContentHashes = map[string]uint64{}
	paneDetectCache   = map[string]paneDetectResult{}
)

// paneDetectResult stores the last detected status and branch for each pane,
// keyed by the same paneKey. When the content hash hasn't changed between
// ticks, these cached values are reused to skip expensive pgrep/git calls.
type paneDetectResult struct {
	status  session.Status
	branch  string
	summary string
}

// paneKey returns the canonical map key for a pane's content hash.
func paneKey(sessionName, windowIndex, paneIndex string) string {
	return sessionName + ":" + windowIndex + "." + paneIndex
}

// hashContent computes the FNV-1a hash of a string.
func hashContent(content string) uint64 {
	// Strip idle timer lines that update every second (e.g., "◆ Baked for 3m 16s")
	// so that timer ticks alone do not cause hash changes.
	normalized := idleTimerPattern.ReplaceAllString(content, "")
	h := fnv.New64a()
	_, _ = io.WriteString(h, normalized)
	return h.Sum64()
}

// contentChanged compares the current content hash with the stored hash.
// Returns true if content changed since the last call. Updates the stored hash.
// On the first call for a given key (no previous hash), returns false —
// we cannot assume Working just because we haven't seen the pane before.
func contentChanged(key string, content string) bool {
	hash := hashContent(content)
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	prev, exists := paneContentHashes[key]
	paneContentHashes[key] = hash
	if !exists {
		return false // First time — no previous state to compare
	}
	return prev != hash
}

// ClearPaneHash removes the stored hash and cached detection result for a pane.
func ClearPaneHash(sessionName, windowIndex, paneIndex string) {
	key := paneKey(sessionName, windowIndex, paneIndex)
	paneCacheMu.Lock()
	delete(paneContentHashes, key)
	delete(paneDetectCache, key)
	paneCacheMu.Unlock()
}

// clearPaneHashByPrefix removes all stored hashes and cached results whose key starts with the given prefix.
func clearPaneHashByPrefix(prefix string) {
	paneCacheMu.Lock()
	for k := range paneContentHashes {
		if strings.HasPrefix(k, prefix) {
			delete(paneContentHashes, k)
		}
	}
	for k := range paneDetectCache {
		if strings.HasPrefix(k, prefix) {
			delete(paneDetectCache, k)
		}
	}
	paneCacheMu.Unlock()
}

// ClearAllPaneCache removes all stored hashes and cached detection results.
func ClearAllPaneCache() {
	paneCacheMu.Lock()
	clear(paneContentHashes)
	clear(paneDetectCache)
	paneCacheMu.Unlock()
}

// contentHashUnchanged checks whether the content hash for the given key
// matches the stored hash without updating it. Returns true if unchanged.
// Returns false if content changed or there is no stored hash.
func contentHashUnchanged(key, content string) bool {
	hash := hashContent(content)
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	prev, exists := paneContentHashes[key]
	if !exists {
		return false
	}
	return prev == hash
}

// getCachedResult returns the cached detection result for a pane, if any.
func getCachedResult(key string) (paneDetectResult, bool) {
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	r, ok := paneDetectCache[key]
	return r, ok
}

// setCachedResult stores a detection result in the cache.
func setCachedResult(key string, r paneDetectResult) {
	paneCacheMu.Lock()
	defer paneCacheMu.Unlock()
	paneDetectCache[key] = r
}
