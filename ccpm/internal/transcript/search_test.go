package transcript

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
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

func TestSearchDeDupesOnlyGenuineDuplication(t *testing.T) {
	// Claude Code does NOT copy a subagent's conversation into its parent —
	// measured across 77 real transcripts, isSidechain is false on all 68,167
	// lines that carry it. An earlier version of this test fabricated a parent
	// line with isSidechain:true, "proving" a de-duplication that was really
	// just content loss. Text that appears in only one file must be found once;
	// text genuinely present in both is two distinct messages and is two hits.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent", userLine(t, "u1", "UNIQUEPARENT text"))
	sub := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, sub, "agent-x.jsonl", userLine(t, "s1", "UNIQUESUB text"))

	for _, q := range []string{"uniqueparent", "uniquesub"} {
		if res := Search(context.Background(), scopeOf(dir), q, SearchOpts{}); len(res.Hits) != 1 {
			t.Errorf("%q: got %d hits, want 1", q, len(res.Hits))
		}
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

// TestSearchPerSessionCapSpansSubagentFiles is the regression for a quota that
// multiplied. Every subagent transcript is its own scan candidate, so a quota
// applied per file gave one session a fresh MaxPerSession for each of them — a
// session with several matching subagents could then fill MaxResults on its own
// and push every other session out of the results, which is the exact outcome
// the cap exists to prevent.
func TestSearchPerSessionCapSpansSubagentFiles(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 10)
	for range 10 {
		lines = append(lines, userLine(t, "u", "repeated hitme line"))
	}
	writeSessionTranscript(t, dir, "/repo", "parent", lines...)

	// Three subagent transcripts under the same session, each full of matches.
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	for _, name := range []string{"agent-a.jsonl", "agent-b.jsonl", "agent-c.jsonl"} {
		writeJSONL(t, subs, name, lines...)
	}

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 3})
	if len(res.Hits) != 3 {
		t.Errorf("got %d hits, want 3 — one session spends one quota across all its files", len(res.Hits))
	}
	for _, h := range res.Hits {
		if h.SessionID != "parent" {
			t.Errorf("hit attributed to %q, want the parent session", h.SessionID)
		}
	}
	if !res.Truncated {
		t.Error("Truncated must be set when a session's remaining files go unscanned")
	}
	if res.Sessions != 1 {
		t.Errorf("Sessions = %d, want 1", res.Sessions)
	}
}

// TestSearchPerSessionCapCarriesPartialSpendForward is the near-neighbour of
// the bug the per-session quota fixed, and it needs its own fixture.
//
// TestSearchPerSessionCapSpansSubagentFiles cannot see it: there the parent
// file exhausts the whole quota by itself, so the state 0 < emitted <
// MaxPerSession never occurs and passing `opts.MaxPerSession` instead of the
// REMAINING quota looks identical. Here the parent produces 2 of a quota of 3,
// so the subagent must be allowed exactly 1 more — a budget that ignores what
// the parent already spent lets it emit 3 for a total of 5.
func TestSearchPerSessionCapCarriesPartialSpendForward(t *testing.T) {
	dir := t.TempDir()
	// Parent: exactly 2 matches, below the quota of 3.
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "hitme once"),
		userLine(t, "u2", "hitme twice"),
	)
	// Subagent: plenty more.
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	many := make([]string, 0, 5)
	for i := range 5 {
		// Distinct uuids, as every real transcript line has. A shared uuid would
		// let a regression that collapses turns by uuid hide behind the quota.
		many = append(many, userLine(t, "s"+strconv.Itoa(i), "hitme from the subagent"))
	}
	writeJSONL(t, subs, "agent-a.jsonl", many...)

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 3})
	if len(res.Hits) != 3 {
		t.Errorf("got %d hits, want 3 — the subagent was given a fresh quota instead of the remaining 1", len(res.Hits))
	}
	// And the split must be right: 2 from the parent, 1 from the subagent.
	var parent, sub int
	for _, h := range res.Hits {
		if h.Subagent {
			sub++
		} else {
			parent++
		}
	}
	if parent != 2 || sub != 1 {
		t.Errorf("hits split parent=%d subagent=%d, want 2 and 1", parent, sub)
	}
}

// TestSearchCountsEachSessionOnceAcrossItsFiles pins the Sessions counter,
// which is rendered as "N matches in M sessions". Counting per file rather than
// per session survived every existing fixture, because none had one session
// emitting hits from two different files.
func TestSearchCountsEachSessionOnceAcrossItsFiles(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent", userLine(t, "u1", "hitme in the parent"))
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, subs, "agent-a.jsonl", userLine(t, "s1", "hitme in a subagent"))
	writeJSONL(t, subs, "agent-b.jsonl", userLine(t, "s2", "hitme in another subagent"))

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 10})
	if len(res.Hits) != 3 {
		t.Fatalf("got %d hits, want all 3", len(res.Hits))
	}
	if res.Sessions != 1 {
		t.Errorf("Sessions = %d, want 1 — three files, one session", res.Sessions)
	}
}

// TestSearchPerSessionCapDoesNotStarveOtherSessions is the consequence that
// actually matters to a user: a chatty session with many subagents must not
// consume the global result budget that other sessions need.
func TestSearchPerSessionCapDoesNotStarveOtherSessions(t *testing.T) {
	dir := t.TempDir()
	many := make([]string, 0, 10)
	for range 10 {
		many = append(many, userLine(t, "u", "repeated hitme line"))
	}
	// The chatty session is NEWEST, so it is scanned first and would exhaust
	// MaxResults before the others are reached.
	writeSessionTranscript(t, dir, "/repo", "chatty", many...)
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "chatty", "subagents")
	for _, name := range []string{"agent-a.jsonl", "agent-b.jsonl", "agent-c.jsonl"} {
		writeJSONL(t, subs, name, many...)
	}
	for _, id := range []string{"quiet1", "quiet2"} {
		writeSessionTranscript(t, dir, "/repo", id, userLine(t, "u", "one hitme here"))
	}

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 2, MaxResults: 8})
	seen := map[string]int{}
	for _, h := range res.Hits {
		seen[h.SessionID]++
	}
	if seen["chatty"] > 2 {
		t.Errorf("chatty session emitted %d hits, want at most its quota of 2", seen["chatty"])
	}
	if seen["quiet1"] == 0 || seen["quiet2"] == 0 {
		t.Errorf("a quiet session was starved out: %v", seen)
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
	// ResolvePath now canonicalises, so the target must actually exist.
	good := filepath.Join("-repo", "s1.jsonl")
	if err := os.MkdirAll(filepath.Join(dir, "projects", "-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "projects", good), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolvePath(dir, good)
	if err != nil {
		t.Fatalf("ResolvePath rejected a legitimate path: %v", err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(dir, "projects", good))
	if got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}

// TestSearchRefusesSymlinkedTranscripts is the search-side twin of
// TestBuildIndexSkipsSymlinkedTranscripts, which did not exist.
//
// It matters because search does NOT go through ResolvePath: collectCandidates
// hands scanFile an absolute path straight from the walk, and eachLine opens it.
// The symlink refusal in collectCandidates is therefore the only thing standing
// between a shared or restored profile and having an arbitrary file's contents
// returned as search snippets. Deleting that check survived the whole suite.
func TestSearchRefusesSymlinkedTranscripts(t *testing.T) {
	dir := t.TempDir()
	// A real transcript, so the search has something legitimate to find.
	writeSessionTranscript(t, dir, "/repo", "real", userLine(t, "u1", "hitme in the real one"))

	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte(`{"type":"user","uuid":"x","sessionId":"s","message":{"role":"user","content":"hitme SUPERSECRET"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "evil.jsonl")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Confirm the premise: the link really does resolve to the secret.
	if b, err := os.ReadFile(link); err != nil || !strings.Contains(string(b), "SUPERSECRET") {
		t.Fatalf("fixture is wrong: the symlink does not read back the target (%v)", err)
	}

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{})
	for _, h := range res.Hits {
		if strings.Contains(h.Before+h.Match+h.After, "SUPERSECRET") {
			t.Fatalf("search returned content from a symlinked file: %+v", h)
		}
		if h.SessionID == "evil" {
			t.Errorf("search scanned a symlinked transcript as session %q", h.SessionID)
		}
	}
	// The legitimate transcript must still be found — a guard that refuses
	// everything would pass the assertions above.
	if len(res.Hits) == 0 {
		t.Error("the real transcript was not searched")
	}
}

// TestResolvePathRejectsSiblingDirectoryEscape covers the case a plain prefix
// comparison lets through. "<profile>/projects-evil" IS prefixed by
// "<profile>/projects", so a containment check written as
// strings.HasPrefix(child, parent) accepts ../projects-evil/x.jsonl.
//
// The other traversal cases cannot catch this: ../outside.jsonl does not share
// the prefix, so it is refused either way, and the whole test passed with the
// real check swapped for HasPrefix. within() uses filepath.Rel precisely to
// avoid this, and nothing pinned that until now.
func TestResolvePathRejectsSiblingDirectoryEscape(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A real, readable file in a sibling whose name extends "projects".
	evil := filepath.Join(dir, "projects-evil")
	if err := os.MkdirAll(evil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evil, "secret.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		filepath.Join("..", "projects-evil", "secret.jsonl"),
		filepath.Join("sub", "..", "..", "projects-evil", "secret.jsonl"),
	} {
		if p, err := ResolvePath(dir, bad); err == nil {
			t.Errorf("ResolvePath accepted sibling-directory escape %q -> %q", bad, p)
		}
	}
}

// TestResolvePathRejectsRootedPathsThatExist is the non-vacuous version of the
// rooted-path cases above.
//
// Those pass on POSIX for the wrong reason: `\Windows\System32\config\SAM` and
// `C:\...` name files that do not exist, so EvalSymlinks fails and the request
// is refused whether or not the portable guard runs. Deleting the entire
// drive-letter/rooted check survived the suite.
//
// Here the hostile names are created as real files INSIDE projects/, so
// existence cannot do the rejecting: only the rooted-form guard can. On POSIX
// these are ordinary (if bizarre) filenames; on Windows they are rooted paths.
// Either way they must be refused, which is the cross-platform parity the
// original test claimed to pin.
func TestResolvePathRejectsRootedPathsThatExist(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`\Windows`, `C:`} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			// Windows cannot create these names; the guard is what matters there.
			t.Logf("skipping %q on this filesystem: %v", name, err)
			continue
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("fixture %q was not created: %v", name, err)
		}
		if got, err := ResolvePath(dir, name); err == nil {
			t.Errorf("ResolvePath accepted rooted-form %q -> %q; the portable guard did not fire", name, got)
		}
	}
}

func TestSearchSeesFullToolContentNotJustThePreview(t *testing.T) {
	// Search must not run on display previews. A tool result clipped to the
	// 2 KB chip preview makes "include tool output" useless on exactly the large
	// outputs it exists for; a tool input reduced to its chip label makes the
	// code Claude wrote unfindable.
	dir := t.TempDir()
	deep := strings.Repeat("filler ", 2000) + " DEEPMARKER"
	if len(deep) <= previewBytes {
		t.Fatal("fixture must exceed the preview cap to be meaningful")
	}
	writeSessionTranscript(t, dir, "/repo", "s1",
		// A marker far past the preview cap inside a tool result.
		toolResultLine(t, "u1", deep),
		// A marker in a NON-primary tool input field: toolInputPreview shows
		// file_path in the chip, so new_string is only reachable if search reads
		// the raw input.
		jl(t, map[string]any{
			"type": "assistant", "uuid": "a1", "sessionId": "s", "cwd": "/repo",
			"message": map[string]any{"role": "assistant", "model": "m", "content": []any{
				map[string]any{"type": "tool_use", "id": "t2", "name": "Edit", "input": map[string]any{
					"file_path": "/repo/a.go", "old_string": "x", "new_string": "func NEWFUNCMARKER() {}"}}}},
		}),
	)
	ctx := context.Background()

	if res := Search(ctx, scopeOf(dir), "deepmarker", SearchOpts{IncludeToolResults: true}); len(res.Hits) != 1 {
		t.Errorf("a match past the preview cap in tool output was missed: %d hits", len(res.Hits))
	}
	res := Search(ctx, scopeOf(dir), "newfuncmarker", SearchOpts{})
	if len(res.Hits) != 1 {
		t.Fatalf("a match in a non-primary tool input field was missed: %d hits", len(res.Hits))
	}
	if res.Hits[0].ToolName != "Edit" {
		t.Errorf("tool name = %q, want Edit", res.Hits[0].ToolName)
	}
	// And the scope rule still holds: tool output stays out by default.
	if res := Search(ctx, scopeOf(dir), "deepmarker", SearchOpts{}); len(res.Hits) != 0 {
		t.Errorf("tool output matched in the default scope: %d hits", len(res.Hits))
	}
}

// TestResolvePathRejectsSymlinkedDirectoryComponent is the regression for a
// real sandbox escape. The guard used to be lexical plus an Lstat on the final
// component; a symlinked DIRECTORY inside projects/ defeated both, because the
// leaf under it genuinely is a regular file. The attacker's vehicle is a
// hand-written usage/history.json inside a shared or restored profile, whose
// rel_path LoadIndex parses verbatim.
func TestResolvePathRejectsSymlinkedDirectoryComponent(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("symlinked directory component", func(t *testing.T) {
		profile := t.TempDir()
		projects := filepath.Join(profile, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(projects, "link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if got, err := ResolvePath(profile, filepath.Join("link", "secret.jsonl")); err == nil {
			t.Errorf("escaped the profile via a symlinked directory: %q", got)
		}
	})

	t.Run("projects itself is a symlink", func(t *testing.T) {
		profile := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(profile, "projects")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if got, err := ResolvePath(profile, "secret.jsonl"); err == nil {
			t.Errorf("escaped via a symlinked projects/: %q", got)
		}
	})

	t.Run("a legitimately symlinked profile directory still resolves", func(t *testing.T) {
		// The root is canonicalised too, so a relocated or synced ~/.ccpm works.
		real := t.TempDir()
		projects := filepath.Join(real, "projects", "-repo")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projects, "s1.jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "profile-link")
		if err := os.Symlink(real, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := ResolvePath(link, filepath.Join("-repo", "s1.jsonl")); err != nil {
			t.Errorf("a symlinked profile dir must still work: %v", err)
		}
	})

	t.Run("a path that does not exist is refused", func(t *testing.T) {
		profile := t.TempDir()
		if err := os.MkdirAll(filepath.Join(profile, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolvePath(profile, "nope.jsonl"); err == nil {
			t.Error("a nonexistent path must not resolve")
		}
	})
}

func TestContainsFold(t *testing.T) {
	// The needle is always lowercase printable ASCII (prefilterNeedle enforces
	// it), so only the haystack varies in case.
	cases := []struct {
		raw, needle string
		want        bool
	}{
		{"the Fork Bomb here", "fork bomb", true},
		{"FORK BOMB", "fork bomb", true},
		{"fork bomb", "fork bomb", true},
		{"no match here", "fork bomb", false},
		{"forkbomb", "fork bomb", false},
		{"partial fork bom", "fork bomb", false},
		{"fork bomb", "", true},
		{"ab", "abc", false},
		{"aaab", "aab", true},           // overlapping candidate starts
		{"café FORK", "fork", true},     // multi-byte bytes in the haystack
		{"\x00\xff FORK", "fork", true}, // invalid UTF-8 in the haystack
	}
	for _, tc := range cases {
		if got := containsFold([]byte(tc.raw), []byte(tc.needle)); got != tc.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tc.raw, tc.needle, got, tc.want)
		}
	}
}

// TestContainsFoldMatchesToLowerContains pins the fast path to the obvious
// spelling it replaced, so the optimisation cannot drift from the semantics.
func TestContainsFoldMatchesToLowerContains(t *testing.T) {
	haystacks := []string{
		"", "a", "The quick BROWN fox", "\x00\x01\x02", "ZZZ zzz ZzZ",
		strings.Repeat("ab", 300) + "NEEDLE" + strings.Repeat("cd", 300),
		"café naïve ÄÖÜ", "MiXeD CaSe MiXeD",
	}
	needles := []string{"a", "z", "needle", "the quick", "mixed case", "zzz", "xyz"}
	for _, h := range haystacks {
		for _, n := range needles {
			want := bytes.Contains(bytes.ToLower([]byte(h)), []byte(n))
			if got := containsFold([]byte(h), []byte(n)); got != want {
				t.Errorf("containsFold(%q,%q)=%v but ToLower+Contains=%v", h, n, got, want)
			}
		}
	}
}

// TestSearchReportsCancellationInsideTheLastCandidate covers the tail case: the
// in-file checkpoint stops the read but cannot distinguish itself from a clean
// finish, so a cancel landing while the FINAL transcript is being scanned used
// to fall out of the loop reporting Cancelled=false — a truncated scan
// presented as complete.
func TestSearchReportsCancellationInsideTheLastCandidate(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 4000)
	for i := range 4000 {
		lines = append(lines, userLine(t, "u"+strconv.Itoa(i), "findme over and over"))
	}
	writeSessionTranscript(t, dir, "/repo", "only", lines...)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel while the single (therefore last) candidate is mid-scan.
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()
	res := Search(ctx, scopeOf(dir), "findme", SearchOpts{MaxPerSession: 100000, MaxResults: 100000})
	if !res.Cancelled {
		t.Error("a cancel inside the last candidate must be reported, not presented as a complete scan")
	}
}

func TestLoadIndexDropsNullEntries(t *testing.T) {
	// A hand-written sidecar in a shared or restored profile can carry a null
	// entry; every consumer dereferences what it finds.
	dir := t.TempDir()
	if err := os.MkdirAll(usage.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":1,"entries":{"good":{"session_id":"good","rel_path":"a/b.jsonl"},"bad":null}}`
	if err := os.WriteFile(IndexPath(dir), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := LoadIndex(dir)
	if _, ok := ix.Entries["bad"]; ok {
		t.Error("a null entry survived the load and will be dereferenced")
	}
	if ix.Entries["good"] == nil {
		t.Error("the valid entry was dropped")
	}
}

// The two tests below use the shape a real transcript has: every prompt is
// followed by the assistant's reply. The first version of the exact-budget fix
// passed its test only because the fixture ended on the matching line — in a
// real session the reply comes next, and that fix still reported the result as
// truncated. A fixture that never has a line after the last match cannot tell
// "stopped at the last match" from "stopped at the last line".
func reply(t *testing.T, uuid, text string) string {
	return asstLine(t, uuid, "claude-opus-5", []any{map[string]any{"type": "text", "text": text}})
}

// TestExactBudgetIsNotReportedAsTruncated: a session holding exactly
// MaxPerSession matches, each answered, with ordinary conversation after the
// last one, is a complete result.
func TestExactBudgetIsNotReportedAsTruncated(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "hitme once"), reply(t, "a1", "sure"),
		userLine(t, "u2", "hitme twice"), reply(t, "a2", "done"),
		userLine(t, "u3", "hitme thrice"), reply(t, "a3", "ok"),
		userLine(t, "u4", "thanks, that is all"), reply(t, "a4", "anytime"),
	)

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 3})
	if len(res.Hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(res.Hits))
	}
	if res.Truncated {
		t.Error("an exhaustive result was reported as truncated — the lines after the last match are not matches")
	}
	if res.Matches != 3 {
		t.Errorf("Matches = %d, want 3", res.Matches)
	}
}

// ...and when a further match does exist — deep in the file, after plenty of
// non-matching conversation — the result must still say it is partial.
// Without this, "never report truncated" would pass the test above.
func TestAMatchBeyondTheBudgetIsStillTruncated(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		userLine(t, "u1", "hitme once"), reply(t, "a1", "sure"),
		userLine(t, "u2", "hitme twice"), reply(t, "a2", "done"),
		userLine(t, "u3", "hitme thrice"), reply(t, "a3", "ok"),
	}
	for i := range 40 {
		id := strconv.Itoa(i)
		lines = append(lines, userLine(t, "f"+id, "unrelated question"), reply(t, "r"+id, "unrelated answer"))
	}
	lines = append(lines, userLine(t, "u9", "one more hitme at the end"), reply(t, "a9", "fine"))
	writeSessionTranscript(t, dir, "/repo", "parent", lines...)

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 3})
	if len(res.Hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(res.Hits))
	}
	if !res.Truncated {
		t.Error("a fourth matching turn exists beyond the budget, but the result was not marked truncated")
	}
}

// A match that exists only where the current scope does not look — tool output,
// with tool output excluded — is not a further match, and must not mark the
// result truncated.
func TestAMatchOutsideTheScopeDoesNotTruncate(t *testing.T) {
	dir := t.TempDir()
	toolResult := jl(t, map[string]any{
		"type": "user", "uuid": "tr1", "sessionId": "s1", "cwd": "/repo",
		"timestamp": "2026-06-27T10:02:00Z",
		"message": map[string]any{"role": "user", "content": []any{map[string]any{
			"type": "tool_result", "tool_use_id": "t1", "content": "hitme inside tool output",
		}}},
	})
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "hitme once"), reply(t, "a1", "sure"),
		userLine(t, "u2", "hitme twice"), reply(t, "a2", "done"),
		toolResult,
	)
	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 2})
	if res.Truncated {
		t.Error("a match in excluded tool output marked the result truncated")
	}
}

// TestDroppedSessionsCountsQuotaSkippedFiles matches the field's own
// documentation: DroppedSessions counts transcripts never opened because a cap
// was already reached. The quota-skip path reported "truncated, 0 dropped"
// while leaving whole subagent transcripts unread.
func TestDroppedSessionsCountsQuotaSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	// The parent alone fills a quota of 2.
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "hitme once"),
		userLine(t, "u2", "hitme twice"),
		userLine(t, "u3", "hitme thrice"),
	)
	// Three subagent transcripts that will never be opened.
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	for _, name := range []string{"agent-a.jsonl", "agent-b.jsonl", "agent-c.jsonl"} {
		writeJSONL(t, subs, name, userLine(t, "s-"+name, "hitme from a subagent"))
	}

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 2})
	if !res.Truncated {
		t.Fatal("quota was exceeded but Truncated is false")
	}
	if res.DroppedSessions != 3 {
		t.Errorf("DroppedSessions = %d, want 3 — three subagent transcripts went unopened", res.DroppedSessions)
	}
}

// TestDroppedSessionsAccumulatesAcrossCaps: a transcript skipped for a spent
// session quota and transcripts cut by the global result cap are both "never
// opened", and the count must include both. The global-cap path assigned
// instead of adding, which threw the quota-skipped files away.
func TestDroppedSessionsAccumulatesAcrossCaps(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	age := func(path string, minutes int) {
		t.Helper()
		ts := now.Add(-time.Duration(minutes) * time.Minute)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	// Newest first: A's own transcript fills A's quota of 2...
	age(writeSessionTranscript(t, dir, "/repo", "A",
		userLine(t, "a1", "hitme"), reply(t, "ar1", "x"),
		userLine(t, "a2", "hitme"), reply(t, "ar2", "x"),
	), 1)
	// ...so A's subagent is skipped unopened (dropped: 1)...
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "A", "subagents")
	age(writeJSONL(t, subs, "agent-a.jsonl", userLine(t, "s1", "hitme")), 2)
	// ...B takes the last slot of the global cap of 3...
	age(writeSessionTranscript(t, dir, "/repo", "B", userLine(t, "b1", "hitme"), reply(t, "br1", "x")), 3)
	// ...and C is never reached (dropped: 2).
	age(writeSessionTranscript(t, dir, "/repo", "C", userLine(t, "c1", "hitme"), reply(t, "cr1", "x")), 4)

	res := Search(context.Background(), scopeOf(dir), "hitme", SearchOpts{MaxPerSession: 2, MaxResults: 3})
	if len(res.Hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(res.Hits))
	}
	if res.DroppedSessions != 2 {
		t.Errorf("DroppedSessions = %d, want 2 — A's subagent (quota) plus C (global cap)", res.DroppedSessions)
	}
}

// TestToolInputsAreSearchedAsTheyRead is the regression for code Claude wrote
// being unfindable by what it says. Tool inputs were searched as raw JSON, so a
// quote or a backslash in a Write's content was stored escaped and the query
// never matched. Real shape: an assistant turn carrying a tool_use block.
func TestToolInputsAreSearchedAsTheyRead(t *testing.T) {
	dir := t.TempDir()
	write := asstLine(t, "a1", "claude-opus-5", []any{map[string]any{
		"type": "tool_use", "id": "t1", "name": "Write",
		"input": map[string]any{
			"file_path": `C:\Users\dev\app.js`,
			"content":   "\"use strict\";\nconst x = 1;\n",
		},
	}})
	writeSessionTranscript(t, dir, "/repo", "sess",
		userLine(t, "u1", "write the file"), write,
	)

	for _, q := range []string{`"use strict"`, `C:\Users`, `const x = 1`} {
		res := Search(context.Background(), scopeOf(dir), q, SearchOpts{})
		if len(res.Hits) != 1 {
			t.Errorf("query %q: got %d hits, want 1 — tool input is being searched in its escaped form", q, len(res.Hits))
			continue
		}
		if res.Hits[0].Source != SourceToolUse {
			t.Errorf("query %q: hit source %q, want %q", q, res.Hits[0].Source, SourceToolUse)
		}
	}
	// The schema is not content: a key name must not produce a hit.
	if res := Search(context.Background(), scopeOf(dir), "file_path", SearchOpts{}); len(res.Hits) != 0 {
		t.Errorf("a JSON key matched as if it were content (%d hits)", len(res.Hits))
	}
}
