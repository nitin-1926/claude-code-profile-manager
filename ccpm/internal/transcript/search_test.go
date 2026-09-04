package transcript

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// touch pins a transcript's mtime so recency ordering is deterministic.
func touch(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func scopeOf(dir string) []Scope { return []Scope{{Profile: "test", Dir: dir}} }

func toolUseLine(t *testing.T, uuid, tool, cmd string) string {
	return jl(t, map[string]any{
		"type": "assistant", "uuid": uuid, "sessionId": "s", "cwd": "/repo",
		"message": map[string]any{"role": "assistant", "model": "m", "content": []any{
			map[string]any{"type": "tool_use", "id": "t1", "name": tool,
				"input": map[string]any{"command": cmd}}}},
	})
}

func toolResultLine(t *testing.T, uuid, out string) string {
	return jl(t, map[string]any{
		"type": "user", "uuid": uuid, "sessionId": "s", "cwd": "/repo",
		"message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": out}}},
	})
}

func thinkingLine(t *testing.T, uuid, thought string) string {
	return jl(t, map[string]any{
		"type": "assistant", "uuid": uuid, "sessionId": "s", "cwd": "/repo",
		"message": map[string]any{"role": "assistant", "model": "m", "content": []any{
			map[string]any{"type": "thinking", "thinking": thought}}},
	})
}

func TestSearchFindsMatchesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	old := writeSessionTranscript(t, dir, "/repo", "older", userLine(t, "u1", "the fork bomb was here"))
	mid := writeSessionTranscript(t, dir, "/repo", "middle", userLine(t, "u1", "nothing relevant"))
	recent := writeSessionTranscript(t, dir, "/repo", "newer", userLine(t, "u1", "another fork bomb mention"))
	touch(t, old, time.Now().Add(-72*time.Hour))
	touch(t, mid, time.Now().Add(-48*time.Hour))
	touch(t, recent, time.Now())

	res := Search(context.Background(), scopeOf(dir), "fork bomb", SearchOpts{})
	if res.Sessions != 2 {
		t.Fatalf("Sessions = %d, want 2", res.Sessions)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(res.Hits))
	}
	if res.Hits[0].SessionID != "newer" || res.Hits[1].SessionID != "older" {
		t.Errorf("order = %s,%s — want newest first",
			res.Hits[0].SessionID, res.Hits[1].SessionID)
	}
}

func TestSearchSnippetOffsetsBracketTheMatch(t *testing.T) {
	// The highlighter uses these offsets verbatim. If they drift, the highlight
	// lands on the wrong words.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", strings.Repeat("padding ", 60)+"NEEDLE"+strings.Repeat(" trailing", 60)))

	res := Search(context.Background(), scopeOf(dir), "needle", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(res.Hits))
	}
	h := res.Hits[0]
	if !strings.EqualFold(h.Match, "needle") {
		t.Errorf("Match = %q, want the matched term", h.Match)
	}
	if h.Before == "" || h.After == "" {
		t.Errorf("snippet has no surrounding context: before=%q after=%q", h.Before, h.After)
	}
	if !strings.Contains(h.Before+h.Match+h.After, "NEEDLE") {
		t.Error("the three pieces do not reassemble to the original text")
	}
}

func TestSearchHitResolvesToTheRightTurn(t *testing.T) {
	// The jump-to-turn interaction rests on this: a hit's UUID must resolve to
	// the turn that actually matched, even though the search pass skipped lines
	// the reader pass decodes. Meta, sidechain and malformed lines are in the
	// fixture precisely because they are what would knock an index out of step.
	dir := t.TempDir()
	metaLine := jl(t, map[string]any{"type": "user", "uuid": "m1", "isMeta": true,
		"message": map[string]any{"role": "user", "content": "meta noise"}})
	sideLine := jl(t, map[string]any{"type": "assistant", "uuid": "sc1", "isSidechain": true,
		"message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "sidechain noise"}}}})
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "first"),
		metaLine,
		`{ malformed `,
		sideLine,
		userLine(t, "u2", "the UNIQUEMARKER is here"),
	)

	res := Search(context.Background(), scopeOf(dir), "uniquemarker", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(res.Hits))
	}
	h := res.Hits[0]
	if h.TurnUUID != "u2" {
		t.Fatalf("hit uuid = %q, want u2", h.TurnUUID)
	}

	at, err := IndexOfTurn(path, h.TurnUUID)
	if err != nil {
		t.Fatal(err)
	}
	// first(0), meta(1), sidechain(2), match(3) — the malformed line is not a
	// turn and must not consume an index.
	if at != 3 {
		t.Errorf("IndexOfTurn = %d, want 3", at)
	}
	page, err := ReadPage(path, at, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || page.Turns[0].UUID != h.TurnUUID {
		t.Errorf("resolving the hit landed on the wrong turn: %+v", page.Turns)
	}

	// And the reader's own entry point must land on it too.
	around, target, err := PageAround(path, h.TurnUUID, 200)
	if err != nil {
		t.Fatal(err)
	}
	if target != 3 {
		t.Errorf("PageAround target = %d, want 3", target)
	}
	var found bool
	for _, turn := range around.Turns {
		if turn.UUID == h.TurnUUID {
			found = true
		}
	}
	if !found {
		t.Error("PageAround did not include the turn it was asked to centre on")
	}
}

func TestPageAroundUnknownUUIDFallsBackToFirstPage(t *testing.T) {
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "one"), userLine(t, "u2", "two"))
	page, at, err := PageAround(path, "no-such-uuid", 200)
	if err != nil {
		t.Fatalf("an unknown uuid must not error: %v", err)
	}
	if at != -1 {
		t.Errorf("target = %d, want -1 for an unknown uuid", at)
	}
	if len(page.Turns) != 2 {
		t.Errorf("fallback page had %d turns, want the whole first page", len(page.Turns))
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "The Fork Bomb"))
	res := Search(context.Background(), scopeOf(dir), "fOrK bOmB", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Errorf("case-insensitive match failed: got %d hits", len(res.Hits))
	}
}

func TestSearchOneResultPerMessageWithMoreCount(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "alpha alpha alpha alpha alpha"))
	res := Search(context.Background(), scopeOf(dir), "alpha", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Fatalf("got %d hits, want 1 per matching message", len(res.Hits))
	}
	if res.Hits[0].More != 4 {
		t.Errorf("More = %d, want 4", res.Hits[0].More)
	}
	if res.Matches != 5 {
		t.Errorf("Matches = %d, want 5", res.Matches)
	}
}

func TestSearchScopeRules(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1",
		toolUseLine(t, "a1", "Bash", "grep -rn TOOLINPUTONLY ."),
		toolResultLine(t, "u1", "TOOLOUTPUTONLY appears here"),
		thinkingLine(t, "a2", "THINKINGONLY appears here"),
	)
	ctx := context.Background()

	// Tool INPUTS are in the default scope: a command or a filename is the
	// most likely real query.
	if res := Search(ctx, scopeOf(dir), "toolinputonly", SearchOpts{}); len(res.Hits) != 1 {
		t.Errorf("tool_use input not searched by default: %d hits", len(res.Hits))
	} else if res.Hits[0].Source != SourceToolUse {
		t.Errorf("source = %q, want tool_use", res.Hits[0].Source)
	}

	// Tool OUTPUT is opt-in: it is 82% of content by volume and where pasted
	// secrets end up.
	if res := Search(ctx, scopeOf(dir), "tooloutputonly", SearchOpts{}); len(res.Hits) != 0 {
		t.Errorf("tool_result matched in default scope: %d hits", len(res.Hits))
	}
	if res := Search(ctx, scopeOf(dir), "tooloutputonly", SearchOpts{IncludeToolResults: true}); len(res.Hits) != 1 {
		t.Errorf("tool_result not found with IncludeToolResults: %d hits", len(res.Hits))
	}

	// Thinking is never searched, in either scope — a hit there is one the
	// reader hides by default.
	for _, o := range []SearchOpts{{}, {IncludeToolResults: true}} {
		if res := Search(ctx, scopeOf(dir), "thinkingonly", o); len(res.Hits) != 0 {
			t.Errorf("thinking matched (IncludeToolResults=%v): %d hits", o.IncludeToolResults, len(res.Hits))
		}
	}
}

func TestSearchIgnoresRawJSONKeys(t *testing.T) {
	// A byte-level scan would happily match transcript machinery.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", jl(t, map[string]any{
		"type": "assistant", "uuid": "a1", "sessionId": "s",
		"message": map[string]any{"role": "assistant", "model": "m",
			"content": []any{map[string]any{"type": "text", "text": "ordinary reply"}},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1,
				"cache_creation_input_tokens": 0, "cache_read_input_tokens": 5}},
	}))
	res := Search(context.Background(), scopeOf(dir), "cache_read_input_tokens", SearchOpts{})
	if len(res.Hits) != 0 {
		t.Errorf("matched a raw JSON key: %d hits", len(res.Hits))
	}
}

func TestSearchDoesNotDoubleCountSubagentContent(t *testing.T) {
	// A subagent's text is copied into its parent as a sidechain turn. Scanning
	// both would return every hit twice.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent",
		jl(t, map[string]any{"type": "assistant", "uuid": "sc1", "isSidechain": true, "sessionId": "s",
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "SHARED subagent finding"}}}}))
	sub := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, sub, "agent-x.jsonl",
		jl(t, map[string]any{"type": "assistant", "uuid": "a1", "sessionId": "agent-x",
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "SHARED subagent finding"}}}}))

	res := Search(context.Background(), scopeOf(dir), "shared subagent finding", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Errorf("got %d hits, want exactly 1 — the subagent file was scanned too", len(res.Hits))
	}
}

func TestSearchHandlesQueryNeedingNoPrefilter(t *testing.T) {
	// A quote is escaped in the raw JSON, so the byte prefilter would miss it.
	// Such queries must fall through to a full decode rather than return
	// nothing.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", `he said "hello there" loudly`))

	if n := prefilterNeedle(`said "hello`); n != nil {
		t.Error("a query containing a quote must disable the raw prefilter")
	}
	res := Search(context.Background(), scopeOf(dir), `said "hello`, SearchOpts{})
	if len(res.Hits) != 1 {
		t.Errorf("quoted query found %d hits, want 1", len(res.Hits))
	}
}

func TestSearchNonASCIIQuery(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "café serveur naïve"))
	if n := prefilterNeedle("café"); n != nil {
		t.Error("a non-ASCII query must disable the raw prefilter (JSON may \\u-escape it)")
	}
	res := Search(context.Background(), scopeOf(dir), "CAFÉ", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Fatalf("non-ASCII query found %d hits, want 1", len(res.Hits))
	}
	h := res.Hits[0]
	if !strings.EqualFold(h.Match, "café") {
		t.Errorf("Match = %q, want café — a byte offset would have mis-split this", h.Match)
	}
}

func TestSearchGlobalCapKeepsNewestAndReportsTruncation(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i, id := range []string{"oldest", "middle", "newest"} {
		p := writeSessionTranscript(t, dir, "/repo", id, userLine(t, "u1", "capme here"))
		touch(t, p, now.Add(time.Duration(i)*time.Hour))
	}
	res := Search(context.Background(), scopeOf(dir), "capme", SearchOpts{MaxResults: 1})
	if len(res.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(res.Hits))
	}
	if res.Hits[0].SessionID != "newest" {
		t.Errorf("cap kept %q — it must keep the most recent, not an alphabetical tail", res.Hits[0].SessionID)
	}
	if !res.Truncated || res.DroppedSessions != 2 {
		t.Errorf("Truncated=%v DroppedSessions=%d, want true/2", res.Truncated, res.DroppedSessions)
	}
}

func TestSearchPerSessionCap(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 20)
	for range 20 {
		lines = append(lines, userLine(t, "u", "repeated hitme line"))
	}
	writeSessionTranscript(t, dir, "/repo", "s1", lines...)

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 3})
	if len(res.Hits) != 3 {
		t.Errorf("got %d hits, want 3 (per-session cap)", len(res.Hits))
	}
	// Matches is a floor: the scan stops at the quota rather than reading on
	// just to keep counting, which is what bounds a common query. It must never
	// exceed what was actually seen.
	if res.Matches != 3 {
		t.Errorf("Matches = %d, want 3 — the floor is what the scan actually saw", res.Matches)
	}
}

func TestSearchStopsReadingAtPerSessionCap(t *testing.T) {
	// The bound that makes a common query usable: once a transcript has given
	// its quota, the rest of the file is not decoded. A marker placed far past
	// the cap must therefore never appear.
	dir := t.TempDir()
	lines := make([]string, 0, 60)
	for range 50 {
		lines = append(lines, userLine(t, "u", "common word here"))
	}
	lines = append(lines, userLine(t, "u", "common word plus TAILMARKER"))
	writeSessionTranscript(t, dir, "/repo", "s1", lines...)

	res := Search(context.Background(), scopeOf(dir), "common word", SearchOpts{MaxPerSession: 2})
	if len(res.Hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(res.Hits))
	}
	for _, h := range res.Hits {
		if strings.Contains(h.Before+h.Match+h.After, "TAILMARKER") {
			t.Error("the scan read past the per-session cap")
		}
	}
}

func TestSearchEmptyQueryDoesNothing(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "anything"))
	for _, q := range []string{"", "   "} {
		res := Search(context.Background(), scopeOf(dir), q, SearchOpts{})
		if len(res.Hits) != 0 || res.Sessions != 0 {
			t.Errorf("empty query returned results")
		}
		if res.Hits == nil {
			t.Error("Hits must be an empty slice, never nil — the frontend maps over it")
		}
	}
}

func TestSearchCancelledContextReturnsPromptly(t *testing.T) {
	dir := t.TempDir()
	for i := range 40 {
		writeSessionTranscript(t, dir, "/repo", "s"+string(rune('a'+i%26))+string(rune('a'+i/26)),
			userLine(t, "u1", "findme everywhere"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the scan starts

	res := Search(ctx, scopeOf(dir), "findme", SearchOpts{})
	if !res.Cancelled {
		t.Error("Cancelled must be reported so the UI can distinguish it from no results")
	}
	if len(res.Hits) != 0 {
		t.Errorf("a pre-cancelled search returned %d hits", len(res.Hits))
	}
}

func TestSearchCountsUnreadableTranscripts(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "ok", userLine(t, "u1", "readable hit"))
	bad := writeSessionTranscript(t, dir, "/repo", "bad", userLine(t, "u1", "readable hit"))
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skip("cannot change file mode here")
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })
	// Windows chmod only toggles the read-only bit, and running as root ignores
	// the mode entirely — in both cases the file stays readable and there is
	// nothing for this test to observe. Verify the premise instead of asserting
	// on a platform where it does not hold.
	if f, err := os.Open(bad); err == nil {
		f.Close()
		t.Skip("file mode does not prevent reads on this platform")
	}

	res := Search(context.Background(), scopeOf(dir), "readable", SearchOpts{})
	if res.Unreadable == 0 {
		t.Error("an unreadable transcript must be counted, not silently dropped")
	}
	if len(res.Hits) != 1 {
		t.Errorf("got %d hits, want the one readable transcript", len(res.Hits))
	}
}

func TestResolvePathRejectsTraversal(t *testing.T) {
	// Session ids and relative paths come out of on-disk JSON, and a profile
	// can be shared or restored. filepath.Join cleans "..", it does not sandbox.
	dir := t.TempDir()
	for _, bad := range []string{
		"../../../etc/passwd.jsonl",
		"..",
		"../outside.jsonl",
		"sub/../../../../etc/passwd",
		"", // empty
		// Rooted paths must be refused identically on every OS. filepath.IsAbs
		// alone would not do it: IsAbs("/etc/passwd") is false on Windows, so
		// relying on it made the guard platform-dependent for the same input.
		"/etc/passwd",
		`\Windows\System32\config\SAM`,
		`C:\Windows\System32\config\SAM`,
		`C:/Windows/System32/config/SAM`,
	} {
		if _, err := ResolvePath(dir, bad); err == nil {
			t.Errorf("ResolvePath accepted %q — that is an arbitrary-file-read primitive", bad)
		}
	}
	good := filepath.Join("-repo", "s1.jsonl")
	got, err := ResolvePath(dir, good)
	if err != nil {
		t.Fatalf("ResolvePath rejected a legitimate path: %v", err)
	}
	if want := filepath.Join(dir, "projects", good); got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}
