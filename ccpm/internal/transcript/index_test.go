package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// writeSessionTranscript writes lines into the real profile layout,
// <profileDir>/projects/<EncodeCwd(cwd)>/<sess>.jsonl, and returns the path.
func writeSessionTranscript(t *testing.T, profileDir, cwd, sess string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(profileDir, "projects", usage.EncodeCwd(cwd))
	return writeJSONL(t, dir, sess+".jsonl", lines...)
}

func aiTitleLine(t *testing.T, sess, title string) string {
	return jl(t, map[string]any{"type": "ai-title", "sessionId": sess, "aiTitle": title})
}

// userLineIn is userLine with an explicit cwd, for tests that assert on the cwd
// a transcript records rather than the directory it happens to live in.
func userLineIn(t *testing.T, uuid, cwd, text string) string {
	return jl(t, map[string]any{
		"type": "user", "uuid": uuid, "sessionId": "s1", "cwd": cwd,
		"timestamp": "2026-06-27T10:00:00Z",
		"message":   map[string]any{"role": "user", "content": text},
	})
}

func TestBuildIndexHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo/one", "s1",
		userLineIn(t, "u1", "/repo/one", "fix the fork bomb"),
		asstLine(t, "a1", "claude-opus-5", []any{map[string]any{"type": "text", "text": "done"}}),
		aiTitleLine(t, "s1", "Fix findCCPM fork bomb"),
	)
	writeSessionTranscript(t, dir, "/repo/two", "s2",
		userLineIn(t, "u1", "/repo/two", "add a health check"),
	)

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(ix.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(ix.Entries))
	}

	e1 := ix.Entries["s1"]
	if e1 == nil {
		t.Fatal("s1 missing from index")
	}
	if e1.Title != "Fix findCCPM fork bomb" {
		t.Errorf("s1 title = %q, want the ai-title value", e1.Title)
	}
	if e1.Model != "claude-opus-5" {
		t.Errorf("s1 model = %q", e1.Model)
	}
	if e1.Cwd != "/repo/one" {
		t.Errorf("s1 cwd = %q", e1.Cwd)
	}

	// No ai-title line, so the first real user prompt becomes the title.
	if got := ix.Entries["s2"].Title; got != "add a health check" {
		t.Errorf("s2 title = %q, want the first user prompt", got)
	}
}

func TestBuildIndexRelPathRoundTrips(t *testing.T) {
	// This is the whole reason RelPath exists. Deriving the directory from cwd
	// via EncodeCwd is what would have made the reader open nothing.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/Users/x/.claude-brain", "s1", userLine(t, "u1", "hi"))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := ix.Entries["s1"]
	if e == nil {
		t.Fatal("s1 missing")
	}
	full := filepath.Join(dir, "projects", e.RelPath)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("RelPath %q does not resolve to a real file: %v", e.RelPath, err)
	}
	if _, err := ReadPage(full, 0, 10); err != nil {
		t.Errorf("the reader cannot open the indexed path: %v", err)
	}
}

func TestBuildIndexSkipsSubagentTranscripts(t *testing.T) {
	// Subagent files are ~75% of transcripts on a real profile and their text
	// is duplicated into the parent as sidechain turns. Indexing them would
	// list each as its own session and double every search hit.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent", userLine(t, "u1", "parent prompt"))
	sub := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, sub, "agent-abc123.jsonl", userLine(t, "u1", "subagent prompt"))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Entries) != 1 {
		t.Fatalf("got %d entries, want 1 — a subagent transcript was indexed", len(ix.Entries))
	}
	if ix.Entries["agent-abc123"] != nil {
		t.Error("subagent transcript became its own session")
	}
}

// usageAsstLine is an assistant line carrying a real usage block, in the shape
// real transcripts produce (distinct uuid, message.id and requestId present).
func usageAsstLine(t *testing.T, uuid, msgID, model string, in, out int64) string {
	t.Helper()
	return jl(t, map[string]any{
		"type": "assistant", "uuid": uuid, "requestId": "req_" + msgID,
		"sessionId": "s1", "cwd": "/repo", "timestamp": "2026-06-27T10:00:00Z",
		"message": map[string]any{"id": msgID, "role": "assistant", "model": model,
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
			"usage": map[string]any{"input_tokens": in, "output_tokens": out,
				"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}},
	})
}

// TestBuildIndexFoldsSubagentTokensIntoTheParent pins the folding that exists
// because subagent lines carry the PARENT's sessionId — internal/usage already
// bills their tokens to this session, so a History row that skipped them read
// 5-14% lower than the Usage tab for the same session.
//
// Nothing covered it: removing the fold loop entirely left the suite green,
// because TestBuildIndexDedupMatchesUsagePackage — the test whose whole job is
// agreeing with usage — has a fixture with no subagent transcripts at all,
// while 14 of 79 real transcripts on this machine have them.
func TestBuildIndexFoldsSubagentTokensIntoTheParent(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "do the thing"),
		usageAsstLine(t, "a1", "msg_p", "claude-opus-5", 100, 10),
	)
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, subs, "agent-a.jsonl", usageAsstLine(t, "s1", "msg_s1", "claude-opus-5", 1000, 100))
	writeJSONL(t, subs, "agent-b.jsonl", usageAsstLine(t, "s2", "msg_s2", "claude-opus-5", 2000, 200))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := ix.Entries["parent"]
	if e == nil {
		t.Fatal("parent session missing")
	}
	tok := e.ByModel["claude-opus-5"]
	// 100+1000+2000 in, 10+100+200 out. Asserting the exact total, not merely
	// "more than the parent alone", so a fold that double-counts also fails.
	if tok.Input != 3100 || tok.Output != 310 {
		t.Errorf("tokens = in %d / out %d, want in 3100 / out 310 — subagent usage was not folded in",
			tok.Input, tok.Output)
	}
}

// TestBuildIndexFoldsNestedWorkflowSubagents covers the deeper nesting that a
// flat directory read would miss. Workflow runs write to
// subagents/workflows/wf_<id>/agent-*.jsonl, and getting this wrong once
// already left a real session's tally 10% short. Every other fixture writes
// subagent files flat, so the recursive walk was untested.
//
// Real data on this machine: 24 of 279 subagent transcripts are nested.
func TestBuildIndexFoldsNestedWorkflowSubagents(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "run a workflow"),
		usageAsstLine(t, "a1", "msg_p", "claude-opus-5", 100, 10),
	)
	nested := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents", "workflows", "wf_abc123")
	writeJSONL(t, nested, "agent-deep.jsonl", usageAsstLine(t, "d1", "msg_d", "claude-opus-5", 5000, 500))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := ix.Entries["parent"]
	if e == nil {
		t.Fatal("parent session missing")
	}
	if tok := e.ByModel["claude-opus-5"]; tok.Input != 5100 {
		t.Errorf("input tokens = %d, want 5100 — a nested workflow subagent was not walked", tok.Input)
	}
	// It must also be reachable by the reader, not merely counted.
	found := false
	for _, p := range e.SubPaths {
		if strings.Contains(p, "workflows/wf_abc123/agent-deep.jsonl") {
			found = true
		}
	}
	if !found {
		t.Errorf("nested subagent missing from SubPaths %v — the reader could not open it", e.SubPaths)
	}
}

// TestBuildIndexRescansWhenOnlyASubagentChanged pins the other half of the
// freshness signature. The parent file is untouched, so a signature covering
// only the parent would treat the entry as fresh and never pick up the new
// subagent turn whose tokens belong to this session's total.
func TestBuildIndexRescansWhenOnlyASubagentChanged(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent",
		userLine(t, "u1", "start"),
		usageAsstLine(t, "a1", "msg_p", "claude-opus-5", 100, 10),
	)
	subs := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, subs, "agent-a.jsonl", usageAsstLine(t, "s1", "msg_s1", "claude-opus-5", 1000, 100))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Entries["parent"].ByModel["claude-opus-5"].Input; got != 1100 {
		t.Fatalf("baseline input = %d, want 1100", got)
	}

	// Grow the subagent transcript. The parent's own mtime and size do not move.
	writeJSONL(t, subs, "agent-a.jsonl",
		usageAsstLine(t, "s1", "msg_s1", "claude-opus-5", 1000, 100),
		usageAsstLine(t, "s2", "msg_s2", "claude-opus-5", 7000, 700),
	)

	again, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Entries["parent"].ByModel["claude-opus-5"].Input; got != 8100 {
		t.Errorf("input = %d, want 8100 — a changed subagent did not invalidate the parent's entry", got)
	}
}

// TestBuildIndexBackfillsSubPathsIntoAStaleSidecar reproduces the shape a real
// sidecar can be left in, and was: SubPaths landed after the subagent-aware
// signature, so an entry written in between carries a correct mtime/size with
// no sub_paths at all.
//
// On an mtime-and-size freshness check alone such an entry looks fresh forever,
// so the field is never backfilled and the session's subagent transcripts stay
// out of the reader's allowlist — unsearchable and unopenable, with nothing
// visibly wrong. Measured across three real profiles before the fix: 12
// sessions had subagent transcripts on disk, 8 of them had no sub_paths.
//
// The fixture is built by indexing for real and then editing the field out,
// rather than by hand-writing a signature — a hand-computed one that happened
// not to match would make this pass for the wrong reason.
func TestBuildIndexBackfillsSubPathsIntoAStaleSidecar(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "parent", userLine(t, "u1", "parent prompt"))
	sub := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "parent", "subagents")
	writeJSONL(t, sub, "agent-abc123.jsonl", userLine(t, "u1", "subagent prompt"))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := ix.Entries["parent"].SubPaths
	if len(want) != 1 {
		t.Fatalf("fixture is wrong: a fresh index recorded %v, want one subagent path", want)
	}

	// Rewind to the pre-SubPaths shape, keeping the signature untouched.
	ix.Entries["parent"].SubPaths = nil
	if err := saveIndex(dir, ix); err != nil {
		t.Fatal(err)
	}
	if got := LoadIndex(dir).Entries["parent"].SubPaths; len(got) != 0 {
		t.Fatalf("fixture is wrong: sub_paths survived the rewind as %v", got)
	}

	// Nothing on disk changed, so a size/mtime check alone would return early.
	again, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Entries["parent"].SubPaths; !slices.Equal(got, want) {
		t.Errorf("stale sidecar was not backfilled: sub_paths = %v, want %v", got, want)
	}
}

// TestBuildIndexReusesEntriesWithNoSubagents guards the other side of that
// check: adding SubPaths to the freshness comparison must not make a session
// that simply has no subagents rebuild on every single pass. nil and empty have
// to compare equal for that, which is why slices.Equal is the right test.
func TestBuildIndexReusesEntriesWithNoSubagents(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "solo", userLine(t, "u1", "no subagents here"))

	if _, err := BuildIndex(dir); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(IndexPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	// A rebuild that changes nothing must not rewrite the file — saveIndex is
	// only reached when an entry was actually touched.
	if _, err := BuildIndex(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(IndexPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Error("a subagent-less session was treated as changed and rewritten")
	}
}

func TestBuildIndexSkipsSymlinkedTranscripts(t *testing.T) {
	// A profile dir can be shared or restored from elsewhere. WalkDir reports a
	// symlinked *file*, and Go's open follows it.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "real", userLine(t, "u1", "real one"))

	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"), "evil.jsonl")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ix.Entries["evil"] != nil {
		t.Error("a symlinked .jsonl was indexed — it could point anywhere on disk")
	}
	if len(ix.Entries) != 1 {
		t.Errorf("got %d entries, want 1", len(ix.Entries))
	}
}

func TestBuildIndexPerModelTallyAndCost(t *testing.T) {
	// A session that switches models must price each model at its own rate,
	// so the History row reconciles with the Usage tab instead of being off by
	// up to 5x.
	dir := t.TempDir()
	usageBlock := func(in, out int64) map[string]any {
		return map[string]any{"input_tokens": in, "output_tokens": out,
			"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}
	}
	// Real assistant lines always carry message.id and requestId. Omitting them
	// would exercise the model|timestamp FALLBACK dedup key rather than the real
	// one — the same shape of mistake as a fixture that repeats a uuid.
	line := func(uuid, msgID, model string, in, out int64) string {
		return jl(t, map[string]any{
			"type": "assistant", "uuid": uuid, "requestId": "req_" + msgID,
			"sessionId": "s1", "cwd": "/repo", "timestamp": "2026-06-27T10:00:00Z",
			"message": map[string]any{"id": msgID, "role": "assistant", "model": model,
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
				"usage":   usageBlock(in, out)},
		})
	}
	writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "mixed model session"),
		line("a1", "msg_h", "claude-haiku-4-5", 1_000_000, 0),
		line("a2", "msg_o", "claude-opus-5", 1_000_000, 0),
	)

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := ix.Entries["s1"]
	if len(e.ByModel) != 2 {
		t.Fatalf("ByModel has %d entries, want 2: %+v", len(e.ByModel), e.ByModel)
	}
	// haiku input is $1/1M, opus input is $5/1M — so the pair is $6, whereas
	// pricing the whole session at the last-seen model (opus) would say $10.
	want := usage.CostFor("claude-haiku-4-5", usage.Tokens{Input: 1_000_000}) +
		usage.CostFor("claude-opus-5", usage.Tokens{Input: 1_000_000})
	if got := e.Cost(); got != want {
		t.Errorf("Cost() = %v, want %v (per-model, not last-model)", got, want)
	}
	if got := e.Tokens().Total(); got != 2_000_000 {
		t.Errorf("Tokens().Total() = %d, want 2000000", got)
	}
}

func TestBuildIndexDedupesTheWayUsageDoes(t *testing.T) {
	// The shape real transcripts actually have: one API response written as
	// several assistant lines that share a message.id and requestId, each with
	// its own uuid and a GROWING usage snapshot. An earlier version of this test
	// repeated the uuid instead, which no real transcript does, so it passed
	// while the dedup key was wrong and tokens were ~2x inflated.
	dir := t.TempDir()
	line := func(uuid, msgID, reqID string, in, out int64) string {
		return jl(t, map[string]any{
			"type": "assistant", "uuid": uuid, "requestId": reqID, "sessionId": "s1", "cwd": "/repo",
			"message": map[string]any{
				"id": msgID, "role": "assistant", "model": "claude-opus-5",
				"content": []any{map[string]any{"type": "text", "text": "x"}},
				"usage": map[string]any{"input_tokens": in, "output_tokens": out,
					"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
			},
		})
	}
	writeSessionTranscript(t, dir, "/repo", "s1",
		// One response, three lines, distinct uuids, growing snapshot -> 150.
		line("u1", "msg_a", "req_a", 100, 0),
		line("u2", "msg_a", "req_a", 100, 30),
		line("u3", "msg_a", "req_a", 100, 50),
		// A genuinely separate response -> +40.
		line("u4", "msg_b", "req_b", 40, 0),
	)

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Entries["s1"].Tokens().Total(); got != 190 {
		t.Errorf("total = %d, want 190 (largest snapshot per message.id+requestId, not a per-line sum)", got)
	}
}

func TestBuildIndexDedupMatchesUsagePackage(t *testing.T) {
	// The History tab and the Usage tab must agree about the same session. The
	// only way to be sure is to run both over the same bytes.
	dir := t.TempDir()
	line := func(uuid, msgID, reqID string, in, out int64) string {
		return jl(t, map[string]any{
			"type": "assistant", "uuid": uuid, "requestId": reqID, "sessionId": "s1", "cwd": "/repo",
			"timestamp": "2026-06-27T10:00:00Z",
			"message": map[string]any{
				"id": msgID, "role": "assistant", "model": "claude-opus-5",
				"content": []any{map[string]any{"type": "text", "text": "x"}},
				"usage": map[string]any{"input_tokens": in, "output_tokens": out,
					"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
			},
		})
	}
	writeSessionTranscript(t, dir, "/repo", "s1",
		line("u1", "msg_a", "req_a", 100, 0),
		line("u2", "msg_a", "req_a", 100, 30),
		line("u3", "msg_b", "req_b", 200, 10),
		line("u4", "msg_b", "req_b", 200, 25),
	)

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, _, err := usage.Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := sess.Records["s1"]
	if rec == nil {
		t.Fatal("usage produced no record for s1")
	}
	got, want := ix.Entries["s1"].Tokens().Total(), rec.Tokens.Total()
	if got != want {
		t.Errorf("History total = %d but Usage total = %d — the two tabs would disagree about the same session", got, want)
	}
}

func TestBuildIndexReusesUnchangedEntries(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "original"))

	if _, err := BuildIndex(dir); err != nil {
		t.Fatal(err)
	}
	// Corrupt the entry in place; an unchanged transcript must not re-scan, so
	// the doctored title should survive a rebuild.
	ix := LoadIndex(dir)
	ix.Entries["s1"].Title = "SENTINEL"
	if err := saveIndex(dir, ix); err != nil {
		t.Fatal(err)
	}

	rebuilt, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Entries["s1"].Title != "SENTINEL" {
		t.Error("an unchanged transcript was re-scanned; the mtime/size check is not working")
	}
}

func TestBuildIndexRetainsEntryWhoseTranscriptIsGone(t *testing.T) {
	// Claude Code prunes transcripts. On a real profile 68 of 90 session
	// records had no file left on disk — a rebuild that dropped them would
	// erase most of the visible history.
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "will be pruned"))
	writeSessionTranscript(t, dir, "/repo", "s2", userLine(t, "u1", "survivor"))

	if _, err := BuildIndex(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ix.Entries["s1"] == nil {
		t.Error("an entry whose transcript was pruned did not survive the rebuild")
	}
	if ix.Entries["s2"] == nil {
		t.Error("the surviving transcript lost its entry")
	}
}

func TestBuildIndexCapsTitleLength(t *testing.T) {
	// The title falls back to the first user prompt, which can be a pasted
	// stack trace. Uncapped it would be written to disk and sent across the
	// bridge once per row.
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", strings.Repeat("verylongword ", 5000)))

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	title := ix.Entries["s1"].Title
	if n := len([]rune(title)); n > TitleRunes {
		t.Errorf("title is %d runes, over the %d cap", n, TitleRunes)
	}
	// And the cap must hold in the persisted file, not just in memory.
	b, err := os.ReadFile(IndexPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Index
	if err := json.Unmarshal(b, &onDisk); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(onDisk.Entries["s1"].Title)); n > TitleRunes {
		t.Errorf("persisted title is %d runes, over the cap", n)
	}
}

func TestScanAITitleWinsRegardlessOfPosition(t *testing.T) {
	dir := t.TempDir()
	// ai-title written AFTER the first user prompt — the common real ordering.
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "some rambling first prompt"),
		asstLine(t, "a1", "m", []any{map[string]any{"type": "text", "text": "ok"}}),
		aiTitleLine(t, "s1", "Concise Generated Title"),
	)
	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Concise Generated Title" {
		t.Errorf("Title = %q, want the ai-title even though it came later", m.Title)
	}
}

func TestScanSkipsSlashCommandPromptsForTitle(t *testing.T) {
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "<command-name>/model</command-name>"),
		userLine(t, "u2", "<local-command-stdout>Set model to Opus</local-command-stdout>"),
		userLine(t, "u3", "now the real question"),
	)
	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "now the real question" {
		t.Errorf("Title = %q, want the first non-command prompt", m.Title)
	}
}

func TestScanSkipsMetaAndSidechainForTitle(t *testing.T) {
	dir := t.TempDir()
	metaLine := jl(t, map[string]any{"type": "user", "uuid": "m1", "isMeta": true,
		"message": map[string]any{"role": "user", "content": "injected system context"}})
	sideLine := jl(t, map[string]any{"type": "user", "uuid": "sc1", "isSidechain": true,
		"message": map[string]any{"role": "user", "content": "subagent instruction"}})
	path := writeSessionTranscript(t, dir, "/repo", "s1", metaLine, sideLine, userLine(t, "u1", "the actual ask"))

	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "the actual ask" {
		t.Errorf("Title = %q, want the first non-meta non-sidechain prompt", m.Title)
	}
}

func TestScanRecordsFirstCwdNotLast(t *testing.T) {
	// A session's cwd drifts: it commonly starts at a repo root and ends in a
	// subdirectory. The transcript lives under the encoded directory of the cwd
	// Claude Code saw first, so taking the last one would display a project
	// that disagrees with where the file actually is.
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLineIn(t, "u1", "/repo", "started at the root"),
		userLineIn(t, "u2", "/repo/subdir", "moved into a subdir"),
	)
	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Cwd != "/repo" {
		t.Errorf("Cwd = %q, want the first-seen /repo", m.Cwd)
	}
}

func TestScanTitleTakesFirstLineAndCollapsesRuns(t *testing.T) {
	// A prompt's first line is its subject, the same convention a commit
	// message uses. Without it, a headless session opening with a long
	// instruction block gets 200 runes of that block as its title.
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "  fix   the fork bomb  \n\nand then explain why it happened\nplus more detail"))
	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "fix the fork bomb" {
		t.Errorf("Title = %q, want the first line with runs collapsed", m.Title)
	}
}

func TestScanTitleSkipsLeadingBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := writeSessionTranscript(t, dir, "/repo", "s1", userLine(t, "u1", "\n\n   \nthe real first line"))
	m, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "the real first line" {
		t.Errorf("Title = %q, want leading blank lines skipped", m.Title)
	}
}

func TestLoadIndexRejectsCorruptAndStaleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(usage.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IndexPath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ix := LoadIndex(dir); ix == nil || ix.Entries == nil || len(ix.Entries) != 0 {
		t.Error("a corrupt index must load as empty, not nil and not an error")
	}

	stale, _ := json.Marshal(Index{Version: indexVersion + 99, Entries: map[string]*Entry{"x": {}}})
	if err := os.WriteFile(IndexPath(dir), stale, 0o644); err != nil {
		t.Fatal(err)
	}
	if ix := LoadIndex(dir); len(ix.Entries) != 0 {
		t.Error("a future-versioned index must be discarded, not partially trusted")
	}
}

func TestBuildIndexMissingProjectsDirIsNotAnError(t *testing.T) {
	ix, err := BuildIndex(t.TempDir())
	if err != nil {
		t.Fatalf("a profile with no projects/ must not error: %v", err)
	}
	if ix.Entries == nil {
		t.Error("Entries must be a non-nil map")
	}
}

// TestNullEntriesArePrunedOnDisk is the regression for a sidecar that never
// self-healed.
//
// LoadIndex drops null entries because every consumer dereferences what it
// finds, but it dropped them only in memory: `changed` flipped solely on a
// rescan, so with no transcript change saveIndex was skipped and the nulls sat
// on disk being re-pruned on every single load, forever.
func TestNullEntriesArePrunedOnDisk(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "alive", userLineIn(t, "u1", "/repo", "hello"))

	// Build once so the sidecar exists and every transcript is recorded fresh.
	if _, err := BuildIndex(dir); err != nil {
		t.Fatalf("first build: %v", err)
	}

	// Hand-edit a null in, the shape a shared or restored profile can carry.
	raw, err := os.ReadFile(IndexPath(dir))
	if err != nil {
		t.Fatalf("reading sidecar: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing sidecar: %v", err)
	}
	entries, _ := doc["entries"].(map[string]any)
	if entries == nil {
		t.Fatal("sidecar has no entries object")
	}
	entries["ghost"] = nil
	edited, _ := json.Marshal(doc)
	if err := os.WriteFile(IndexPath(dir), edited, 0o600); err != nil {
		t.Fatalf("writing sidecar: %v", err)
	}

	// Nothing on disk changed, so this build rescans nothing. The prune alone
	// must be enough to make it write.
	if _, err := BuildIndex(dir); err != nil {
		t.Fatalf("second build: %v", err)
	}

	after, err := os.ReadFile(IndexPath(dir))
	if err != nil {
		t.Fatalf("re-reading sidecar: %v", err)
	}
	if bytes.Contains(after, []byte(`"ghost"`)) {
		t.Error("the null entry is still on disk — the prune was never persisted")
	}
}

// TestIndexSavesAreRecognisedByTheirWatchEvents drives a real fsnotify watcher
// over a real BuildIndex save and asserts that every event it produces is one
// IsIndexFile recognises.
//
// This is the regression for a fix that did nothing: the desktop watcher
// skipped events named exactly "history.json", but atomicwrite stages the
// write to a sibling and renames it into place, so the events arrive under
// "history.json.ccpm-staged-<rand>" and every save still fired a refresh. A
// test that only checked the filter against a hand-written name would have
// passed against that broken fix, which is why this one watches the real write.
func TestIndexSavesAreRecognisedByTheirWatchEvents(t *testing.T) {
	dir := t.TempDir()
	writeSessionTranscript(t, dir, "/repo", "sess", userLineIn(t, "u1", "/repo", "hello"))
	usageDir := usage.Dir(dir)
	if err := os.MkdirAll(usageDir, 0o700); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable here: %v", err)
	}
	defer w.Close()
	if err := w.Add(usageDir); err != nil {
		t.Fatal(err)
	}

	if _, err := BuildIndex(dir); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	var seen []string
	deadline := time.After(750 * time.Millisecond)
collect:
	for {
		select {
		case ev := <-w.Events:
			seen = append(seen, filepath.Base(ev.Name))
		case err := <-w.Errors:
			t.Fatalf("watcher error: %v", err)
		case <-deadline:
			break collect
		}
	}

	if len(seen) == 0 {
		t.Fatal("the index save produced no watch events at all — this test is not observing the write")
	}
	for _, name := range seen {
		if !IsIndexFile(name) {
			t.Errorf("index save emitted an event for %q, which the watcher would treat as a real change", name)
		}
	}
	// Prove the events include the staged sibling, i.e. that the case the old
	// exact-name check missed actually occurs on this platform.
	var staged bool
	for _, name := range seen {
		staged = staged || strings.HasPrefix(name, "history.json.ccpm-")
	}
	if !staged {
		t.Logf("note: no staged-sibling event observed here (events: %v)", seen)
	}
}

func TestIsIndexFileDoesNotSwallowRealChanges(t *testing.T) {
	for name, want := range map[string]bool{
		"/p/usage/history.json":                   true,
		"/p/usage/history.json.ccpm-staged-4f2a":  true,
		"/p/usage/history.json.ccpm-rollback":     true,
		"/p/usage/sessions.json":                  false, // the usage store is a real change
		"/p/usage/history.jsonl":                  false,
		"/Users/x/.ccpm/config.json":              false,
		"/p/usage/sessions.json.ccpm-staged-4f2a": false,
	} {
		if got := IsIndexFile(name); got != want {
			t.Errorf("IsIndexFile(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestIndexFollowsAMovedTranscript: a transcript moved with its size and mtime
// intact (mv, cp -p) must not leave its entry pointing at the old path. The
// freshness check compared size, mtime and subagent paths only, so the entry
// looked unchanged forever and the row could never be opened again.
func TestIndexFollowsAMovedTranscript(t *testing.T) {
	dir := t.TempDir()
	old := writeSessionTranscript(t, dir, "/repo-a", "sess", userLineIn(t, "u1", "/repo-a", "hello"))
	if _, err := BuildIndex(dir); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(old)
	if err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(dir, "projects", usage.EncodeCwd("/repo-b"))
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(newDir, filepath.Base(old))
	if err := os.WriteFile(moved, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(moved, fi.ModTime(), fi.ModTime()); err != nil { // cp -p
		t.Fatal(err)
	}
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Rel(filepath.Join(dir, "projects"), moved)
	if got := ix.Entries["sess"].RelPath; got != filepath.ToSlash(want) {
		t.Errorf("RelPath = %q, want %q — the entry still points at where the file used to be", got, filepath.ToSlash(want))
	}
}
