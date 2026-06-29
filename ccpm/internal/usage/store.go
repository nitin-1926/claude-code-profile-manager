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

