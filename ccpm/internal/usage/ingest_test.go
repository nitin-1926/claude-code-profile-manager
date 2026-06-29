package usage

import (
	"testing"
	"time"
)

// asst builds an assistant usage line for fold tests.
func asst(id, model, sessID, cwd, ts string, in, out, cc, cr int64) transcriptLine {
	var l transcriptLine
	l.Type = "assistant"
	l.SessionID = sessID
	l.Cwd = cwd
	l.Timestamp = ts
	l.Message.ID = id
	l.Message.Model = model
	l.Message.Usage = &rawUsage{
		InputTokens:              in,
		OutputTokens:             out,
		CacheCreationInputTokens: cc,
		CacheReadInputTokens:     cr,
	}
	return l
}

func freshIndexes() (*Sessions, *Daily, map[string]bool) {
	return newSessions(), newDaily(), map[string]bool{}
}

// TestFoldLineDedup is the load-bearing case: a single API response is written
// as several lines sharing one message.id; it must be counted exactly once.
func TestFoldLineDedup(t *testing.T) {
	sess, day, seen := freshIndexes()
	line := asst("msg_1", "claude-opus-4-8", "sess_a", "/repo", "2026-06-27T10:00:00Z", 100, 20, 5, 50)

	counted := 0
	for i := 0; i < 3; i++ { // same message.id three times
		if foldLine(line, sess, day, seen) {
			counted++
		}
	}
	if counted != 1 {
		t.Fatalf("dedup failed: counted %d times, want 1", counted)
	}
	rec := sess.Records["sess_a"]
	if rec == nil {
		t.Fatal("no session record created")
	}
	if rec.Messages != 1 {
		t.Fatalf("messages = %d, want 1", rec.Messages)
	}
	want := Tokens{Input: 100, Output: 20, CacheCreation: 5, CacheRead: 50}
	if rec.Tokens != want {
		t.Fatalf("tokens = %+v, want %+v", rec.Tokens, want)
	}
	if got := rec.Tokens.Total(); got != 175 {
		t.Fatalf("total = %d, want 175", got)
	}
	// by-model lives on the daily ledger now (sum across days, TZ-independent).
	var modelTotal int64
	for _, dr := range day.Days {
		modelTotal += dr.ByModel["claude-opus-4-8"].Total()
	}
	if modelTotal != 175 {
		t.Fatalf("daily by-model total = %d, want 175", modelTotal)
	}
}

// TestFoldLineFilters confirms non-countable lines are ignored.
func TestFoldLineFilters(t *testing.T) {
	cases := []struct {
		name string
		line transcriptLine
	}{
		{"user line", transcriptLine{Type: "user", SessionID: "s"}},
		{"assistant without usage", func() transcriptLine {
			l := transcriptLine{Type: "assistant", SessionID: "s"}
			l.Message.ID = "msg_x"
			return l
		}()},
		{"attachment noise", transcriptLine{Type: "attachment", SessionID: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess, day, seen := freshIndexes()
			if foldLine(tc.line, sess, day, seen) {
				t.Fatal("line was counted but should have been filtered")
			}
			if len(sess.Records) != 0 || len(day.Days) != 0 {
				t.Fatal("filtered line mutated the indexes")
			}
		})
	}
}

// TestDailyBucketingAcrossDayBoundary verifies tokens land in the day of each
// message's own timestamp, so a session spanning midnight splits across days
// while the session record sums both.
func TestDailyBucketingAcrossDayBoundary(t *testing.T) {
	// Pin bucketing to UTC so the assertion is host-timezone independent.
	prev := bucketLocation
	bucketLocation = time.UTC
	defer func() { bucketLocation = prev }()

	sess, day, seen := freshIndexes()
	foldLine(asst("m1", "opus", "s", "/r", "2026-06-26T23:59:00Z", 10, 1, 0, 0), sess, day, seen)
	foldLine(asst("m2", "opus", "s", "/r", "2026-06-27T00:01:00Z", 20, 2, 0, 0), sess, day, seen)

	if day.Days["2026-06-26"] == nil || day.Days["2026-06-27"] == nil {
		t.Fatalf("expected both day buckets, got %v", keys(day.Days))
	}
	if got := day.Days["2026-06-26"].Tokens.Total(); got != 11 {
		t.Fatalf("2026-06-26 total = %d, want 11", got)
	}
	if got := day.Days["2026-06-27"].Tokens.Total(); got != 22 {
		t.Fatalf("2026-06-27 total = %d, want 22", got)
	}
	if got := sess.Records["s"].Tokens.Total(); got != 33 {
		t.Fatalf("session total = %d, want 33", got)
	}
	if sess.Records["s"].FirstTS != "2026-06-26T23:59:00Z" || sess.Records["s"].LastTS != "2026-06-27T00:01:00Z" {
		t.Fatalf("first/last ts wrong: %q / %q", sess.Records["s"].FirstTS, sess.Records["s"].LastTS)
	}
}

func keys(m map[string]*DailyRecord) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
