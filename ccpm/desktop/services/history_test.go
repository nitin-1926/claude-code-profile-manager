//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
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
		// A refusal is now an ERROR, not an empty success. The reader could not
		// previously distinguish "denied", "pruned" and "this conversation is
		// empty", and rendered a blank page for all three. What must never
		// change is that no content comes back and the slices stay non-nil.
		page, err := h.Transcript(name, bad, "", 0, 10)
		if err == nil {
			t.Errorf("Transcript(%q) was allowed — path traversal", bad)
		}
		if len(page.Turns) != 0 {
			t.Errorf("Transcript(%q) returned %d turns — path traversal", bad, len(page.Turns))
		}
		if page.Turns == nil {
			t.Errorf("Transcript(%q) returned a nil Turns slice on the error path", bad)
		}
		body, err := h.ToolBody(name, bad, "", "any", 0)
		if err == nil {
			t.Errorf("ToolBody(%q) was allowed — path traversal", bad)
		}
		if body.Body != "" {
			t.Errorf("ToolBody(%q) returned content — path traversal", bad)
		}
	}
}

// An unknown session now reports why rather than returning a blank page, but
// the DTO it returns alongside the error must still satisfy the non-nil
// invariant — Wails marshals the value whether or not the error is set, and a
// nil slice becomes JSON null and breaks the frontend's .map.
func TestHistoryTranscriptUnknownSessionErrorsWithANonNilPage(t *testing.T) {
	name := firstProfile(t)
	page, err := NewHistory().Transcript(name, "no-such-session", "", 0, 10)
	if err == nil {
		t.Error("an unknown session should report that it could not be found")
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

// TestReaderFindsATranscriptWrittenSinceTheLastListing is the regression for a
// blank page with no error.
//
// Search enumerates the filesystem live; the reader authorises against the
// sidecar. Nothing rebuilt the sidecar in between, and watcher.go deliberately
// skips projects/, so a transcript written after the History tab last listed —
// a subagent file a running session just spawned — was a legitimate search hit
// that the reader then refused, returning an empty page and no explanation. The
// window never closed on its own.
func TestReaderFindsATranscriptWrittenSinceTheLastListing(t *testing.T) {
	profile := syntheticProfile(t)
	dir := profileDir(profile)
	if dir == "" {
		t.Fatal("synthetic profile has no directory")
	}

	// A session that exists when the tab first lists.
	sessDir := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "sess")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "sess.jsonl")
	writeLine := func(path, uuid, text string) {
		t.Helper()
		line := `{"type":"user","uuid":"` + uuid + `","sessionId":"sess","cwd":"/repo",` +
			`"timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"` + text + `"}}` + "\n"
		if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeLine(parent, "u1", "hello")

	h := NewHistory()
	if _, err := h.Sessions(profile); err != nil {
		t.Fatalf("initial listing: %v", err)
	}

	// Now a subagent transcript appears, exactly as a running session would
	// produce it. The sidecar on disk predates it.
	subs := filepath.Join(sessDir, "subagents")
	if err := os.MkdirAll(subs, 0o700); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(subs, "agent-a.jsonl")
	writeLine(agent, "s1", "from the subagent")

	rel, err := filepath.Rel(filepath.Join(dir, "projects"), agent)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)

	// This is the exact call the frontend makes with a fresh search hit.
	page, err := h.Transcript(profile, "sess", rel, 0, 10)
	if err != nil {
		t.Fatalf("a transcript written since the last listing was refused: %v", err)
	}
	if len(page.Turns) == 0 {
		t.Error("resolved the path but read no turns — the blank page this test exists to prevent")
	}
}

// The rebuild must not become a way to launder an arbitrary path: a relPath
// that is not in the index after a fresh build is still refused.
func TestRebuildDoesNotAdmitAnArbitraryPath(t *testing.T) {
	profile := syntheticProfile(t)
	h := NewHistory()
	if _, err := h.Transcript(profile, "sess", "../../../../etc/passwd", 0, 10); err == nil {
		t.Error("a traversal path was admitted after the index rebuild")
	}
}
