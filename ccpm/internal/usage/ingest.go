package usage

import "time"

// bucketLocation is the timezone used to bucket message timestamps into local
// calendar days. It is a package var (not a hardcoded time.Local) only so tests
// can pin it to UTC for deterministic day-boundary assertions.
var bucketLocation = time.Local

// straddleSentinel seeds the dedup map for a key counted in a prior ingest whose
// duplicate lines straddle the offset boundary. Its huge Total means any real
// duplicate line compares as "not larger" and is skipped (preserving the prior
// count without knowing its exact tokens).
var straddleSentinel = Tokens{Input: 1 << 62}

// transcriptLine is the minimal decoded shape of one JSONL transcript line.
// Claude Code writes many more fields per line; decoding only what usage needs
// means added/unknown keys are ignored rather than breaking ingest.
type transcriptLine struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Version   string `json:"version"`
	Slug      string `json:"slug"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string    `json:"id"`
		Model string    `json:"model"`
		Usage *rawUsage `json:"usage"`
	} `json:"message"`
}

// rawUsage is the token sub-object Claude Code writes under message.usage.
type rawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// dedupKey returns the identity used to count a usage-bearing line exactly once,
// or "" when the line carries no countable usage. A single API response is
// written as several assistant lines that repeat the same message.id; the key is
// (message.id + requestId) so distinct requests that happen to reuse a message.id
// (e.g. sidechain replays) are counted separately rather than collapsed. Falls
// back to model+timestamp for the rare transcript variant that carries usage but
// no message.id. Counting each unique key once (largest snapshot wins, see
// foldLine) is what keeps totals correct.
func (l transcriptLine) dedupKey() string {
	if l.Type != "assistant" || l.Message.Usage == nil {
		return ""
	}
	if l.Message.ID != "" {
		return l.Message.ID + "|" + l.RequestID
	}
	return l.Message.Model + "|" + l.Timestamp
}

func (l transcriptLine) tokens() Tokens {
	u := l.Message.Usage
	if u == nil {
		return Tokens{}
	}
	return Tokens{
		Input:         u.InputTokens,
		Output:        u.OutputTokens,
		CacheCreation: u.CacheCreationInputTokens,
		CacheRead:     u.CacheReadInputTokens,
	}
}

// foldLine applies one decoded line to the in-memory indexes, returning true if
// it counted (or revised an earlier count). counted holds the tokens already
// attributed to each dedup key this ingest. Claude appends several lines for one
// request, sometimes with growing usage snapshots; the LARGEST-total snapshot
// wins — a smaller/equal duplicate is skipped, and a larger one revises the
// prior contribution by the delta (so no line is under- or double-counted). The
// day bucket comes from the line's own timestamp, so multi-day sessions split
// correctly.
func foldLine(l transcriptLine, sess *Sessions, day *Daily, counted map[string]Tokens) bool {
	key := l.dedupKey()
	if key == "" {
		return false
	}
	tok := l.tokens()
	prev, seen := counted[key]
	if seen && tok.Total() <= prev.Total() {
		return false // not a larger snapshot — ignore this duplicate
	}
	delta := tok.Minus(prev) // prev is zero when unseen → delta == tok
	counted[key] = tok
	first := !seen
	model := l.Message.Model

	// Session record (created on first sight of the sessionId). cwd makes this
	// the source for the by-project view; no per-day project map is kept.
	rec := sess.Records[l.SessionID]
	if rec == nil {
		rec = &SessionRecord{SessionID: l.SessionID}
		sess.Records[l.SessionID] = rec
	}
	if l.Cwd != "" {
		rec.Cwd = l.Cwd
	}
	if l.GitBranch != "" {
		rec.GitBranch = l.GitBranch
	}
	if l.Slug != "" {
		rec.Slug = l.Slug
	}
	if l.Timestamp != "" {
		if rec.FirstTS == "" || l.Timestamp < rec.FirstTS {
			rec.FirstTS = l.Timestamp
		}
		if l.Timestamp > rec.LastTS {
			rec.LastTS = l.Timestamp
		}
	}
	rec.Tokens.Add(delta)
	if first {
		rec.Messages++
	}

	// Daily ledger — bucket by the message's own local calendar day, split by
	// model so totals, by-model, and the time series fold from here.
	if dayKey := dayBucket(l.Timestamp); dayKey != "" {
		dr := day.Days[dayKey]
		if dr == nil {
			dr = &DailyRecord{ByModel: map[string]Tokens{}}
			day.Days[dayKey] = dr
		}
		dr.Tokens.Add(delta)
		if first {
			dr.Messages++
		}
		addByModel(dr.ByModel, model, delta)
	}
	return true
}

// addByModel accumulates tok under key in m, normalising an empty key.
func addByModel(m map[string]Tokens, key string, tok Tokens) {
	if key == "" {
		key = "unknown"
	}
	v := m[key]
	v.Add(tok)
	m[key] = v
}

// dayBucket parses an RFC3339 timestamp and returns the YYYY-MM-DD calendar day
// in bucketLocation, or "" when the timestamp is missing or unparseable.
func dayBucket(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return t.In(bucketLocation).Format("2006-01-02")
}
