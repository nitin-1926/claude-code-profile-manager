// Package usage tracks per-profile token usage by reading the Claude Code
// session transcripts a profile accumulates under <profileDir>/projects/. It
// maintains a small JSON store per profile (an ingest cursor plus a session
// index and a daily index) so `ccpm usage` can report token totals, per-model /
// per-project / per-session breakdowns, and a contribution-style heatmap
// without re-parsing every transcript on each run.
//
// All token counts are raw counts only — this package deliberately computes no
// dollar cost (Claude Code transcripts carry no price, and a subscription's
// real cost is unrelated to API-equivalent pricing).
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// storeVersion is the on-disk schema version for every store file. A loaded
// store whose version differs is discarded and rebuilt from the transcripts
// (cheap and idempotent), so a schema change never serves stale-shaped data.
// v2: dropped session.by_model/version and daily.by_project (project is derived
// from each session's cwd).
const storeVersion = 2

// Tokens is the four-way token tally Claude Code reports per assistant message.
type Tokens struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheCreation int64 `json:"cache_creation"`
	CacheRead     int64 `json:"cache_read"`
}

// Add accumulates o into t in place.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheCreation += o.CacheCreation
	t.CacheRead += o.CacheRead
}

// Total is the sum of all four token kinds.
func (t Tokens) Total() int64 {
	return t.Input + t.Output + t.CacheCreation + t.CacheRead
}

// FileState is the ingest cursor for one transcript file. Offset is the number
// of bytes already folded (always the end of a complete line). LastMsgID is the
// last message.id counted from this file, used to dedupe a request whose
// duplicate lines straddle the offset boundary between two ingest runs.
type FileState struct {
	Offset    int64  `json:"offset"`
	Size      int64  `json:"size"`
	ModTime   int64  `json:"mtime"`
	LastMsgID string `json:"last_msg_id,omitempty"`
}

// State is the persisted ingest cursor set, keyed by transcript path relative
// to the profile's projects/ root.
type State struct {
	Version int                  `json:"version"`
	Files   map[string]FileState `json:"files"`
}

// SessionRecord is one row per Claude Code session. Its cwd is the project (the
// transcript dir is per-cwd), so the by-project view is derived by grouping
// these — no per-day project map is stored. The id list doubles as the
// `claude --resume` set.
type SessionRecord struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd,omitempty"`
	GitBranch string `json:"git_branch,omitempty"`
	Slug      string `json:"slug,omitempty"`
	FirstTS   string `json:"first_ts,omitempty"`
	LastTS    string `json:"last_ts,omitempty"`
	Tokens    Tokens `json:"tokens"`
	Messages  int64  `json:"messages"`
}

// Sessions is the session index, keyed by sessionId.
type Sessions struct {
	Version int                       `json:"version"`
	Records map[string]*SessionRecord `json:"records"`
}

// DailyRecord is the token tally for a single local calendar day, split by
// model (kept 4-way — the one grain where the input/output/cache split feeds any
// future cost math). Folding the days in a window yields totals, by-model, and
// the time series; by-project comes from the session index instead.
type DailyRecord struct {
	Tokens   Tokens            `json:"tokens"`
	Messages int64             `json:"messages"`
	ByModel  map[string]Tokens `json:"by_model,omitempty"`
}

// Daily is the canonical time-bucketed ledger: date "YYYY-MM-DD" (local day of
// each message) -> per-day tallies.
type Daily struct {
	Version int                     `json:"version"`
	Days    map[string]*DailyRecord `json:"days"`
}

