//go:build darwin

package services

import (
	"strings"
	"sync"
	"testing"
)

func TestHistorySessionsUnknownProfileIsSafe(t *testing.T) {
	rows, err := NewHistory().Sessions("definitely-not-a-real-profile-xyz")
	if err != nil {
		t.Fatalf("unknown profile errored: %v", err)
	}
	if rows == nil {
		t.Error("Sessions must return an empty slice, never nil — the frontend maps over it")
	}
	if len(rows) != 0 {
		t.Errorf("unknown profile returned %d rows", len(rows))
	}
}

func TestHistorySessionsOnRealProfile(t *testing.T) {
	name := firstProfile(t)
	h := NewHistory()
	rows, err := h.Sessions(name)
	if err != nil {
		t.Fatalf("Sessions(%s): %v", name, err)
	}
	assertNoNullArrays(t, map[string]any{"sessions": rows}, "sessions")
	if len(rows) == 0 {
		t.Skip("profile has no sessions to assert on")
	}
	for _, r := range rows {
		if r.ID == "" {
			t.Error("a session row has no id")
		}
	}
	// Newest first.
	for i := 1; i < len(rows); i++ {
		if rows[i-1].LastTS < rows[i].LastTS {
			t.Errorf("rows are not sorted newest-first at %d: %q before %q",
				i, rows[i-1].LastTS, rows[i].LastTS)
			break
		}
	}
	// Subagent transcripts must never surface as their own session.
	for _, r := range rows {
		if strings.HasPrefix(r.ID, "agent-") {
			t.Errorf("a subagent transcript was listed as a session: %s", r.ID)
		}
	}
	t.Logf("%s: %d sessions", name, len(rows))
}

// TestHistoryTranscriptRejectsTraversal is the regression guard for the
// arbitrary-file-read this API would otherwise be. Session ids reach it from
// on-disk JSON, and a profile directory can be shared or restored.
func TestHistoryTranscriptRejectsTraversal(t *testing.T) {
	name := firstProfile(t)
	h := NewHistory()
	for _, bad := range []string{
		"../../../../etc/passwd",
		"../../../etc/passwd",
		"/etc/passwd",
		"..",
	} {
		page, err := h.Transcript(name, bad, "", 0, 10)
		if err != nil {
			t.Fatalf("Transcript(%q) errored instead of returning empty: %v", bad, err)
		}
		if len(page.Turns) != 0 {
			t.Errorf("Transcript(%q) returned %d turns — path traversal", bad, len(page.Turns))
		}
		body, err := h.ToolBody(name, bad, "", "any", 0)
		if err != nil {
			t.Fatalf("ToolBody(%q) errored: %v", bad, err)
		}
		if body.Body != "" {
			t.Errorf("ToolBody(%q) returned content — path traversal", bad)
		}
	}
}

func TestHistoryTranscriptUnknownSessionIsEmptyNotNil(t *testing.T) {
	name := firstProfile(t)
	page, err := NewHistory().Transcript(name, "no-such-session", "", 0, 10)
	if err != nil {
		t.Fatalf("unknown session errored: %v", err)
	}
	if page.Turns == nil {
		t.Error("Turns must be an empty slice, never nil")
	}
	if page.TargetIndex != -1 {
		t.Errorf("TargetIndex = %d, want -1", page.TargetIndex)
	}
}

// TestHistoryCancelBeforeSearch covers the race Wails makes easy: every bound
// call runs on its own goroutine, so a debounced UI can land CancelSearch(tok)
// before Search(tok) registers. Without the tombstone the cancel is a no-op and
// a full profile scan runs anyway.
func TestHistoryCancelBeforeSearch(t *testing.T) {
	name := firstProfile(t)
	h := NewHistory()
	h.CancelSearch("tok-early")
	res, err := h.Search(name, "the", "tok-early", false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !res.Cancelled {
		t.Error("a search whose cancel arrived first must return cancelled")
	}
	if len(res.Hits) != 0 {
		t.Errorf("cancelled search returned %d hits", len(res.Hits))
	}
	if res.Hits == nil {
		t.Error("Hits must be an empty slice, never nil")
	}
}

func TestHistorySearchTokensAreIndependentAndDoNotLeak(t *testing.T) {
	name := firstProfile(t)
	h := NewHistory()

	var wg sync.WaitGroup
	for _, tok := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.Search(name, "fork bomb", tok, false); err != nil {
				t.Errorf("Search(%s): %v", tok, err)
			}
		}()
	}
	wg.Wait()

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cancels) != 0 {
		t.Errorf("%d cancel funcs leaked after the searches completed", len(h.cancels))
	}
}

func TestHistorySearchUnknownProfileIsSafe(t *testing.T) {
	res, err := NewHistory().Search("definitely-not-a-real-profile-xyz", "anything", "t", false)
	if err != nil {
		t.Fatalf("unknown profile errored: %v", err)
	}
	if res.Hits == nil {
		t.Error("Hits must be an empty slice, never nil")
	}
}

func TestHistoryTombstonesAreBounded(t *testing.T) {
	h := NewHistory()
	for i := range maxTombstones + 10 {
		h.CancelSearch(strings.Repeat("t", i%7+1) + string(rune('a'+i%26)) + string(rune('0'+i%10)))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.tombstones) > maxTombstones {
		t.Errorf("tombstones grew to %d, over the %d bound", len(h.tombstones), maxTombstones)
	}
}
