package usage

import "time"

// bucketLocation is the timezone used to bucket message timestamps into local
// calendar days. It is a package var (not a hardcoded time.Local) only so tests
// can pin it to UTC for deterministic day-boundary assertions.
var bucketLocation = time.Local

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
// written as several assistant lines that repeat the same message.id and the
// same usage; counting each unique message.id once is what keeps totals correct
// (without it, totals run ~3x high). Falls back to model+timestamp for the rare
// transcript variant that carries usage but no message.id.
func (l transcriptLine) dedupKey() string {
	if l.Type != "assistant" || l.Message.Usage == nil {
		return ""
	}
	if l.Message.ID != "" {
		return l.Message.ID
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

