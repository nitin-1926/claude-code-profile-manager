package usage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// linesJSONL marshals transcript lines to a JSONL byte slice (each line + \n).
func linesJSONL(lines ...transcriptLine) []byte {
	var buf bytes.Buffer
	for _, l := range lines {
		b, _ := json.Marshal(l)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// writeTranscript writes raw bytes to <profileDir>/projects/<enc>/<sess>.jsonl.
func writeTranscript(t *testing.T, profileDir, cwd, sess string, data []byte) string {
	t.Helper()
	dir := filepath.Join(profileDir, "projects", EncodeCwd(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sess+".jsonl")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncOffsetAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	// Two requests, each written as 3 duplicate lines (the real-world shape).
	r1 := []transcriptLine{
		asst("msg_1", "opus", "s1", "/repo", "2026-06-27T10:00:00Z", 100, 20, 0, 0),
	}
	r1 = append(r1, r1[0], r1[0])
	r2 := []transcriptLine{
		asst("msg_2", "opus", "s1", "/repo", "2026-06-27T10:05:00Z", 2, 381, 0, 0),
	}
	r2 = append(r2, r2[0], r2[0])
	writeTranscript(t, dir, "/repo", "s1", linesJSONL(append(r1, r2...)...))

	sess, day, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := sess.Records["s1"]
	if rec == nil || rec.Messages != 2 {
		t.Fatalf("messages = %v, want 2 (dedup)", rec)
	}
	if got := rec.Tokens.Total(); got != 503 { // 120 + 383
		t.Fatalf("session total = %d, want 503", got)
	}
	if got := day.Days["2026-06-27"].Tokens.Total(); got != 503 {
		t.Fatalf("daily total = %d, want 503", got)
	}

	// Re-sync with no file change: totals must be unchanged (idempotent).
	sess2, _, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess2.Records["s1"].Tokens.Total(); got != 503 {
		t.Fatalf("after no-op re-sync total = %d, want 503", got)
	}
	if got := sess2.Records["s1"].Messages; got != 2 {
		t.Fatalf("after no-op re-sync messages = %d, want 2", got)
	}

	// Append a new request and re-sync: only the new tokens are added.
	r3 := linesJSONL(asst("msg_3", "sonnet", "s1", "/repo", "2026-06-27T11:00:00Z", 7, 8, 0, 0))
	f, _ := os.OpenFile(filepath.Join(dir, "projects", EncodeCwd("/repo"), "s1.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.Write(r3)
	f.Close()

	sess3, day3, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess3.Records["s1"].Total(); got != 518 { // +15
		t.Fatalf("after append total = %d, want 518", got)
	}
	if got := sess3.Records["s1"].Messages; got != 3 {
		t.Fatalf("after append messages = %d, want 3", got)
	}
	var sonnet int64
	for _, dr := range day3.Days {
		sonnet += dr.ByModel["sonnet"].Total()
	}
	if sonnet != 15 {
		t.Fatalf("sonnet by-model = %d, want 15", sonnet)
	}
}

func TestSyncPartialLastLine(t *testing.T) {
	dir := t.TempDir()
	complete := linesJSONL(asst("msg_1", "opus", "s1", "/repo", "2026-06-27T10:00:00Z", 100, 0, 0, 0))
	// A second line that is mid-write: valid JSON but no trailing newline.
	partial, _ := json.Marshal(asst("msg_2", "opus", "s1", "/repo", "2026-06-27T10:01:00Z", 50, 0, 0, 0))
	writeTranscript(t, dir, "/repo", "s1", append(append([]byte{}, complete...), partial...))

	sess, _, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Records["s1"].Messages; got != 1 {
		t.Fatalf("partial line counted: messages = %d, want 1", got)
	}
	if got := sess.Records["s1"].Tokens.Total(); got != 100 {
		t.Fatalf("total = %d, want 100 (partial excluded)", got)
	}

	// Complete the second line (add newline) and re-sync: now it counts.
	path := filepath.Join(dir, "projects", EncodeCwd("/repo"), "s1.jsonl")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.Write([]byte("\n"))
	f.Close()

	sess2, _, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess2.Records["s1"].Messages; got != 2 {
		t.Fatalf("after completing line: messages = %d, want 2", got)
	}
	if got := sess2.Records["s1"].Tokens.Total(); got != 150 {
		t.Fatalf("after completing line: total = %d, want 150", got)
	}
}

// Total is a small test convenience on the record's token tally.
func (r *SessionRecord) Total() int64 { return r.Tokens.Total() }

// appendTranscript appends raw bytes to an existing transcript file.
func appendTranscript(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
}

// TestSyncStraddleDoesNotDoubleCount pins the fix for a duplicate of a
// NON-last dedup key arriving after the offset boundary. Seeding dedup with
// only the single last key let every other recently-counted key be treated as
// new on the next sync, inflating the total.
func TestSyncStraddleDoesNotDoubleCount(t *testing.T) {
	dir := t.TempDir()
	a := asst("msg_a", "opus", "s1", "/repo", "2026-06-27T10:00:00Z", 100, 0, 0, 0)
	b := asst("msg_b", "opus", "s1", "/repo", "2026-06-27T10:01:00Z", 50, 0, 0, 0)
	path := writeTranscript(t, dir, "/repo", "s1", linesJSONL(a, b))

	sess, _, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Records["s1"].Tokens.Total(); got != 150 {
		t.Fatalf("after first sync total = %d, want 150", got)
	}

	// Claude re-emits msg_a (an earlier key) after the boundary.
	appendTranscript(t, path, linesJSONL(a))
	sess, _, err = Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Records["s1"].Tokens.Total(); got != 150 {
		t.Fatalf("after straddling duplicate total = %d, want 150 (double count)", got)
	}
}

// TestSyncStraddleAdoptsLargerSnapshot pins the other half: Claude writes
// intermediate usage snapshots and then a larger final one. A max sentinel
// froze the earlier, smaller value forever when the boundary fell between them.
func TestSyncStraddleAdoptsLargerSnapshot(t *testing.T) {
	dir := t.TempDir()
	small := asst("msg_a", "opus", "s1", "/repo", "2026-06-27T10:00:00Z", 100, 0, 0, 0)
	path := writeTranscript(t, dir, "/repo", "s1", linesJSONL(small))

	if _, _, err := Sync(dir); err != nil {
		t.Fatal(err)
	}

	// The final snapshot for the SAME request lands after the boundary.
	large := asst("msg_a", "opus", "s1", "/repo", "2026-06-27T10:00:00Z", 100, 900, 0, 0)
	appendTranscript(t, path, linesJSONL(large))
	sess, day, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Records["s1"].Tokens.Total(); got != 1000 {
		t.Fatalf("session total = %d, want 1000 (largest snapshot wins across the boundary)", got)
	}
	if got := day.Days["2026-06-27"].Tokens.Total(); got != 1000 {
		t.Fatalf("daily total = %d, want 1000", got)
	}
	if got := sess.Records["s1"].Messages; got != 1 {
		t.Fatalf("messages = %d, want 1 (a revision is not a new message)", got)
	}
}

// Claude Code prunes transcripts; their cursors used to stay in state.json
// forever (79 dead entries on a real profile), rewritten on every sync. A
// cursor is dropped only once its file is really gone — one the walk merely
// could not reach this time keeps its cursor, or the next sync would re-count
// the whole file.
func TestSyncPrunesCursorsForDeletedTranscripts(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions enforced")
	}
	dir := t.TempDir()
	line := func(sess string) []byte {
		return []byte(realAsstLine(t, sess+"-u1", "msg_"+sess, "req_"+sess, sess, "/repo", "2026-06-27T10:00:00.000Z", "opus", 10, 0, "a"))
	}
	keep := writeTranscript(t, dir, "/repo", "keep", line("keep"))
	gone := writeTranscript(t, dir, "/repo", "gone", line("gone"))
	hidden := writeTranscript(t, dir, "/other", "hidden", line("hidden"))
	if _, _, err := Sync(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	// Unreadable, not deleted: the walk cannot enter it this time.
	hiddenDir := filepath.Dir(hidden)
	if err := os.Chmod(hiddenDir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(hiddenDir, 0o755) })
	if _, _, err := Sync(dir); err != nil {
		t.Fatal(err)
	}

	st, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel := func(p string) string { r, _ := filepath.Rel(filepath.Join(dir, "projects"), p); return r }
	if _, ok := st.Files[rel(gone)]; ok {
		t.Errorf("cursor for a deleted transcript survived: %v", st.Files)
	}
	if _, ok := st.Files[rel(keep)]; !ok {
		t.Errorf("cursor for a live transcript was dropped")
	}
	if _, ok := st.Files[rel(hidden)]; !ok {
		t.Errorf("cursor for a transiently unreachable transcript was dropped")
	}
}

// realAsstLine renders one assistant line in the shape Claude Code actually
// writes: its own uuid, the request's message.id + requestId (the dedup key),
// and a usage block under message. text pads message.content so a test can
// make the line as long as it needs.
func realAsstLine(t *testing.T, uuid, msgID, reqID, sess, cwd, ts, model string, in, out int64, text string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "assistant", "uuid": uuid, "parentUuid": nil, "isSidechain": false,
		"sessionId": sess, "cwd": cwd, "timestamp": ts, "requestId": reqID,
		"version": "2.1.0", "userType": "external",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant", "model": model,
			"content": []any{map[string]any{"type": "text", "text": text}},
			"usage": map[string]any{
				"input_tokens": in, "output_tokens": out,
				"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

// An over-cap line must be skipped — never materialised whole — and the cursor
// must still advance past it so the lines after it count. Without the cap the
// reader buffered the entire line (a newline-free file grows without bound) and
// decoded it as usage.
func TestSyncSkipsOversizeLineAndAdvancesPastIt(t *testing.T) {
	dir := t.TempDir()
	huge := strings.Repeat("z", MaxLineBytes+4096)
	body := realAsstLine(t, "u1", "msg_1", "req_1", "s1", "/repo", "2026-06-27T10:00:00.000Z", "opus", 100, 0, "a") +
		realAsstLine(t, "u2", "msg_big", "req_big", "s1", "/repo", "2026-06-27T10:01:00.000Z", "opus", 1000, 0, huge) +
		realAsstLine(t, "u3", "msg_2", "req_2", "s1", "/repo", "2026-06-27T10:02:00.000Z", "opus", 50, 0, "b")
	path := writeTranscript(t, dir, "/repo", "s1", []byte(body))

	sess, _, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Records["s1"].Total(); got != 150 {
		t.Fatalf("total = %d, want 150 (the over-cap line must be skipped, the lines around it counted)", got)
	}
	st, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(filepath.Join(dir, "projects"), path)
	if got, want := st.Files[rel].Offset, int64(len(body)); got != want {
		t.Fatalf("cursor = %d, want %d (end of file, past the skipped line)", got, want)
	}
}
