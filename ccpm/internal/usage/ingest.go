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

