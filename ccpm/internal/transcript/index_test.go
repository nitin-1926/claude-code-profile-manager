package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	line := func(uuid, model string, in, out int64) string {
		return jl(t, map[string]any{
			"type": "assistant", "uuid": uuid, "sessionId": "s1", "cwd": "/repo",
			"message": map[string]any{"role": "assistant", "model": model,
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
				"usage":   usageBlock(in, out)},
		})
	}
	writeSessionTranscript(t, dir, "/repo", "s1",
		userLine(t, "u1", "mixed model session"),
		line("a1", "claude-haiku-4-5", 1_000_000, 0),
		line("a2", "claude-opus-5", 1_000_000, 0),
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

func TestBuildIndexDedupesRepeatedUsageLines(t *testing.T) {
	// Claude writes several assistant lines per response; counting each once is
	// what keeps the token tally honest.
	dir := t.TempDir()
	same := jl(t, map[string]any{
		"type": "assistant", "uuid": "a1", "sessionId": "s1",
		"message": map[string]any{"role": "assistant", "model": "claude-opus-5",
			"content": []any{map[string]any{"type": "text", "text": "x"}},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 0,
				"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}},
	})
	writeSessionTranscript(t, dir, "/repo", "s1", same, same, same)

	ix, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Entries["s1"].Tokens().Total(); got != 100 {
		t.Errorf("total = %d, want 100 — repeated lines for one response were counted more than once", got)
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
