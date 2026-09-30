package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- fixture helpers -------------------------------------------------------
//
// Mirrors internal/usage's in-process fixture style: no testdata/ directory
// anywhere in this repo, everything fabricated and written to t.TempDir().

// jl marshals one transcript line from a free-form map.
func jl(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal fixture line: %v", err)
	}
	return string(b)
}

// writeJSONL writes lines (already-marshalled JSON, one per element) to a file
// under dir and returns its path. A trailing newline is written after each line
// unless the caller opted out via writeJSONLNoTrailer.
func writeJSONL(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// userLine builds a v2 user line whose content is a bare string.
func userLine(t *testing.T, uuid, text string) string {
	return jl(t, map[string]any{
		"type": "user", "uuid": uuid, "sessionId": "s1", "cwd": "/repo",
		"timestamp": "2026-06-27T10:00:00Z",
		"message":   map[string]any{"role": "user", "content": text},
	})
}

// asstLine builds an assistant line with typed content blocks.
func asstLine(t *testing.T, uuid, model string, blocks []any) string {
	return jl(t, map[string]any{
		"type": "assistant", "uuid": uuid, "sessionId": "s1", "cwd": "/repo",
		"timestamp": "2026-06-27T10:01:00Z",
		"message":   map[string]any{"role": "assistant", "model": model, "content": blocks},
	})
}

// --- ReadPage --------------------------------------------------------------

func TestReadPageDecodesBothContentShapes(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir, "s1.jsonl",
		userLine(t, "u1", "fix the fork bomb"),
		asstLine(t, "a1", "claude-opus-5", []any{
			map[string]any{"type": "thinking", "thinking": "considering"},
			map[string]any{"type": "text", "text": "The probe matched the GUI binary"},
			map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Bash",
				"input": map[string]any{"command": "grep -rn findCCPM"}},
		}),
	)

	page, err := ReadPage(path, 0, 100)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("Total = %d, want 2", page.Total)
	}
	if len(page.Turns) != 2 {
		t.Fatalf("len(Turns) = %d, want 2", len(page.Turns))
	}

	u := page.Turns[0]
	if u.Role != "user" || u.Index != 0 {
		t.Errorf("turn 0 = role %q index %d, want user 0", u.Role, u.Index)
	}
	if len(u.Blocks) != 1 || u.Blocks[0].Kind != KindText || u.Blocks[0].Text != "fix the fork bomb" {
		t.Errorf("user blocks = %+v, want one text block", u.Blocks)
	}

	a := page.Turns[1]
	if a.Model != "claude-opus-5" {
		t.Errorf("assistant model = %q", a.Model)
	}
	wantKinds := []Kind{KindThinking, KindText, KindToolUse}
	if len(a.Blocks) != len(wantKinds) {
		t.Fatalf("assistant blocks = %d, want %d", len(a.Blocks), len(wantKinds))
	}
	for i, want := range wantKinds {
		if a.Blocks[i].Kind != want {
			t.Errorf("block %d kind = %q, want %q", i, a.Blocks[i].Kind, want)
		}
	}
	if a.Blocks[2].ToolName != "Bash" {
		t.Errorf("tool name = %q, want Bash", a.Blocks[2].ToolName)
	}
	if !strings.Contains(a.Blocks[2].Preview, "findCCPM") {
		t.Errorf("tool_use preview lost the input: %q", a.Blocks[2].Preview)
	}
}

func TestReadPageWindows(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 25)
	for i := range 25 {
		lines = append(lines, userLine(t, "u", "prompt"))
		_ = i
	}
	path := writeJSONL(t, dir, "s1.jsonl", lines...)

	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 10 || page.Total != 25 {
		t.Errorf("got %d turns / total %d, want 10 / 25", len(page.Turns), page.Total)
	}
	if page.Turns[0].Index != 0 || page.Turns[9].Index != 9 {
		t.Errorf("indices = %d..%d, want 0..9", page.Turns[0].Index, page.Turns[9].Index)
	}

	// An offset past the end must render an empty view, not error and not nil.
	page, err = ReadPage(path, 999, 10)
	if err != nil {
		t.Fatalf("offset past end errored: %v", err)
	}
	if page.Turns == nil {
		t.Error("Turns is nil — it must be an empty slice so the frontend can .map it")
	}
	if len(page.Turns) != 0 || page.Total != 25 {
		t.Errorf("got %d turns / total %d, want 0 / 25", len(page.Turns), page.Total)
	}
}

func TestReadPageStopsBeforeIncompleteTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	// Two complete lines, then a third still being written (no newline).
	body := userLine(t, "u1", "one") + "\n" + userLine(t, "u2", "two") + "\n" + `{"type":"user","uuid":"u3","mess`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ReadPage(path, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2 — the partial trailing line must not count", page.Total)
	}
}

func TestReadPageSkipsMalformedLineWithoutAdvancingIndex(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir, "s1.jsonl",
		userLine(t, "u1", "one"),
		`{ this is not json `,
		userLine(t, "u2", "two"),
	)
	page, err := ReadPage(path, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("Total = %d, want 2", page.Total)
	}
	if page.Turns[0].Index != 0 || page.Turns[1].Index != 1 {
		t.Errorf("indices = %d,%d — a malformed line must not consume an index",
			page.Turns[0].Index, page.Turns[1].Index)
	}
}

func TestReadPageEnumerationIgnoresUIFilters(t *testing.T) {
	// Meta and sidechain turns must still occupy an index. If they did not,
	// a stored search hit would address a different turn whenever the reader's
	// toggles changed.
	dir := t.TempDir()
	meta := jl(t, map[string]any{
		"type": "user", "uuid": "m1", "isMeta": true,
		"message": map[string]any{"role": "user", "content": "injected context"},
	})
	side := jl(t, map[string]any{
		"type": "assistant", "uuid": "sc1", "isSidechain": true,
		"message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "subagent talking"}}},
	})
	path := writeJSONL(t, dir, "s1.jsonl", userLine(t, "u1", "one"), meta, side, userLine(t, "u2", "two"))

	page, err := ReadPage(path, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 {
		t.Fatalf("Total = %d, want 4 (meta and sidechain both count)", page.Total)
	}
	if !page.Turns[1].IsMeta {
		t.Error("meta turn lost its IsMeta flag")
	}
	if !page.Turns[2].IsSidechain {
		t.Error("sidechain turn lost its IsSidechain flag")
	}
	if page.Turns[3].Index != 3 {
		t.Errorf("last turn index = %d, want 3", page.Turns[3].Index)
	}
}

func TestReadPagePreservesUnknownBlockTypes(t *testing.T) {
	// Claude Code owns this format and does not version it. An unrecognised
	// block must survive to the UI as a visible placeholder rather than
	// disappearing, so a format change is self-reporting.
	dir := t.TempDir()
	path := writeJSONL(t, dir, "s1.jsonl", asstLine(t, "a1", "m", []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "some_future_block", "payload": "?"},
	}))
	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.UnknownBlocks != 1 {
		t.Errorf("UnknownBlocks = %d, want 1", page.UnknownBlocks)
	}
	blocks := page.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 — the unknown block was dropped", len(blocks))
	}
	if blocks[1].Kind != KindUnknown || blocks[1].RawType != "some_future_block" {
		t.Errorf("unknown block = %+v, want kind=unknown rawType=some_future_block", blocks[1])
	}
}

func TestReadPageToolResultBothShapes(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir, "s1.jsonl", jl(t, map[string]any{
		"type": "user", "uuid": "u1",
		"message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "plain string output"},
			map[string]any{"type": "tool_result", "tool_use_id": "t2", "content": []any{
				map[string]any{"type": "text", "text": "block array output"}}},
		}},
	}))
	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	blocks := page.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Preview != "plain string output" {
		t.Errorf("string tool_result preview = %q", blocks[0].Preview)
	}
	if blocks[1].Preview != "block array output" {
		t.Errorf("array tool_result preview = %q", blocks[1].Preview)
	}
}

func TestReadPageTruncatesHugeToolResult(t *testing.T) {
	dir := t.TempDir()
	huge := strings.Repeat("x", 200_000)
	path := writeJSONL(t, dir, "s1.jsonl", jl(t, map[string]any{
		"type": "user", "uuid": "u1",
		"message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": huge}}},
	}))
	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	b := page.Turns[0].Blocks[0]
	if !b.Truncated {
		t.Error("a 200 KB tool result must be marked truncated")
	}
	if len(b.Preview) > previewBytes {
		t.Errorf("preview is %d bytes, over the %d cap", len(b.Preview), previewBytes)
	}
	if b.FullBytes != len(huge) {
		t.Errorf("FullBytes = %d, want %d — the UI needs the true size", b.FullBytes, len(huge))
	}
}

func TestReadPageHandlesLargeLine(t *testing.T) {
	// The longest line measured in a real profile was 1.3 MB. bufio's 64 KB
	// default would have silently ended the file there.
	dir := t.TempDir()
	big := strings.Repeat("y", 2<<20)
	path := writeJSONL(t, dir, "s1.jsonl",
		jl(t, map[string]any{"type": "user", "uuid": "u1",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "t", "content": big}}}}),
		userLine(t, "u2", "after the big line"),
	)
	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2 — a 2 MB line must not end the file", page.Total)
	}
}

func TestReadPageErrors(t *testing.T) {
	if _, err := ReadPage(filepath.Join(t.TempDir(), "nope.jsonl"), 0, 10); err == nil {
		t.Error("a nonexistent path must error")
	}
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ReadPage(empty, 0, 10)
	if err != nil {
		t.Errorf("an empty file must not error: %v", err)
	}
	if page.Total != 0 || len(page.Turns) != 0 {
		t.Errorf("empty file yielded %d turns", len(page.Turns))
	}
}

// --- clipping --------------------------------------------------------------

func TestClipDoesNotSplitRunes(t *testing.T) {
	// "é" is two bytes; cutting at an odd boundary must back off, not produce
	// invalid UTF-8.
	s := strings.Repeat("é", 100)
	got, truncated := clip(s, 51)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !utf8.ValidString(got) {
		t.Errorf("clip produced invalid UTF-8: %q", got)
	}
	if len(got) > 51 {
		t.Errorf("clip returned %d bytes, over the 51 cap", len(got))
	}
}

func TestClipRunes(t *testing.T) {
	if got := ClipRunes("abcdef", 3); got != "abc" {
		t.Errorf("ClipRunes = %q, want abc", got)
	}
	if got := ClipRunes("éééééé", 3); got != "ééé" {
		t.Errorf("ClipRunes on multibyte = %q, want ééé", got)
	}
	if got := ClipRunes("short", 99); got != "short" {
		t.Errorf("ClipRunes shortened an under-cap string: %q", got)
	}
}

// --- FirstUserPrompt -------------------------------------------------------
//
// These cases are carried over from cmd/sessions_test.go, which owned this
// decoder before it moved here.

func TestFirstUserPrompt(t *testing.T) {
	cases := []struct {
		name  string
		entry map[string]any
		want  string
	}{
		{"v1 top-level content string",
			map[string]any{"content": "  hello v1  "}, "hello v1"},
		{"v2 message.content string",
			map[string]any{"message": map[string]any{"role": "user", "content": "hello v2"}}, "hello v2"},
		{"v2 message.content block array",
			map[string]any{"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "hello blocks"}}}}, "hello blocks"},
		{"skips non-text leading block",
			map[string]any{"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "thinking", "thinking": "hmm"},
				map[string]any{"type": "text", "text": "after thinking"}}}}, "after thinking"},
		{"assistant role is rejected",
			map[string]any{"message": map[string]any{"role": "assistant", "content": "not a prompt"}}, ""},
		{"empty entry",
			map[string]any{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FirstUserPrompt(tc.entry); got != tc.want {
				t.Errorf("FirstUserPrompt() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsTitleWorthy(t *testing.T) {
	// Real first-user lines are frequently slash-command envelopes; using one
	// as a title makes every other row read "/model".
	bad := []string{
		"", "   ",
		"<command-name>/model</command-name>\n<command-message>model</command-message>",
		"<local-command-stdout>Set model to Opus</local-command-stdout>",
	}
	for _, s := range bad {
		if isTitleWorthy(s) {
			t.Errorf("isTitleWorthy(%q) = true, want false", s)
		}
	}
	if !isTitleWorthy("Fix the fork bomb in findCCPM") {
		t.Error("a real prompt must be title-worthy")
	}
}

func TestToolInputPreviewPicksTheIdentifyingField(t *testing.T) {
	// A chip showing `{"language":"go","code":"package main\n\nimport (...` is
	// technically the input and useless as a summary.
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"bash command", `{"command":"grep -rn findCCPM ccpm/","timeout":120}`, "grep -rn findCCPM ccpm/"},
		{"edit path", `{"file_path":"/repo/a.go","old_string":"x","new_string":"y"}`, "/repo/a.go"},
		{"grep pattern", `{"pattern":"fork bomb","glob":"*.go"}`, "fork bomb"},
		{"collapses newlines", `{"command":"line one\n\nline   two"}`, "line one line two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, full, _ := toolInputPreview([]byte(tc.input))
			if got != tc.want {
				t.Errorf("preview = %q, want %q", got, tc.want)
			}
			if full != len(tc.input) {
				t.Errorf("FullBytes = %d, want the whole input (%d)", full, len(tc.input))
			}
		})
	}

	// No recognised field: fall back to the raw JSON rather than showing nothing.
	got, _, _ := toolInputPreview([]byte(`{"unusual":"shape"}`))
	if got != `{"unusual":"shape"}` {
		t.Errorf("fallback preview = %q, want the raw input", got)
	}
	if got, full, _ := toolInputPreview(nil); got != "" || full != 0 {
		t.Errorf("empty input = %q/%d, want empty", got, full)
	}
}

func TestEachLineBoundsAllocationOnAPathologicalLine(t *testing.T) {
	// A transcript with no newlines would otherwise have its whole length
	// materialised before any cap could be consulted. The over-cap line must be
	// reported as skipped, and the file must keep going afterwards.
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	huge := strings.Repeat("z", maxLineBytes+4096)
	body := userLine(t, "u1", "before") + "\n" +
		`{"type":"user","uuid":"big","message":{"role":"user","content":"` + huge + `"}}` + "\n" +
		userLine(t, "u2", "after") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ReadPage(path, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.SkippedLines != 1 {
		t.Errorf("SkippedLines = %d, want 1", page.SkippedLines)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2 — the lines around the oversize one must still parse", page.Total)
	}
}

func TestEachLineStillDropsIncompleteTrailingLine(t *testing.T) {
	// Guarded explicitly because the bounded reader is easy to rewrite with
	// bufio.Scanner, which yields the trailing partial line instead of skipping
	// it and would render half a JSON object from a session being written now.
	dir := t.TempDir()
	path := filepath.Join(dir, "s1.jsonl")
	body := userLine(t, "u1", "complete") + "\n" + `{"type":"user","uuid":"u2","messa`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ReadPage(path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}
