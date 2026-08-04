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
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/lock"
)

// storeVersion is the on-disk schema version for every store file. A loaded
// store whose version differs is discarded and rebuilt from the transcripts
// (cheap and idempotent), so a schema change never serves stale-shaped data.
// v2: dropped session.by_model/version and daily.by_project (project is derived
// from each session's cwd).
// v3: dedup key is now (message.id + requestId) with largest-token-total wins
// (Claude appends intermediate usage snapshots then a final larger one; first-
// wins undercounted). Bumping forces a clean re-ingest so existing stores adopt
// the corrected counts.
const storeVersion = 4

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

// Minus returns t - o field-wise (used to revise a dedup entry when a larger
// usage snapshot for the same request arrives).
func (t Tokens) Minus(o Tokens) Tokens {
	return Tokens{
		Input:         t.Input - o.Input,
		Output:        t.Output - o.Output,
		CacheCreation: t.CacheCreation - o.CacheCreation,
		CacheRead:     t.CacheRead - o.CacheRead,
	}
}

// FileState is the ingest cursor for one transcript file. Offset is the number
// of bytes already folded (always the end of a complete line). LastMsgID is the
// last message.id counted from this file, used to dedupe a request whose
// duplicate lines straddle the offset boundary between two ingest runs.
type FileState struct {
	Offset  int64        `json:"offset"`
	Size    int64        `json:"size"`
	ModTime int64        `json:"mtime"`
	Recent  []CountedKey `json:"recent,omitempty"`
}

// CountedKey is one dedup key and the token snapshot already attributed to it.
// The tail of these is persisted per transcript so an ingest that resumes
// mid-request can both skip duplicates it already counted AND still adopt a
// larger final snapshot for them (largest-wins across the offset boundary).
type CountedKey struct {
	Key    string `json:"key"`
	Tokens Tokens `json:"tokens"`
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

// Dir is the per-profile usage store directory, <profileDir>/usage.
func Dir(profileDir string) string { return filepath.Join(profileDir, "usage") }

func statePath(profileDir string) string    { return filepath.Join(Dir(profileDir), "state.json") }
func sessionsPath(profileDir string) string { return filepath.Join(Dir(profileDir), "sessions.json") }
func dailyPath(profileDir string) string    { return filepath.Join(Dir(profileDir), "daily.json") }
func lockPath(profileDir string) string     { return filepath.Join(Dir(profileDir), ".lock") }

func newState() *State { return &State{Version: storeVersion, Files: map[string]FileState{}} }
func newSessions() *Sessions {
	return &Sessions{Version: storeVersion, Records: map[string]*SessionRecord{}}
}
func newDaily() *Daily {
	return &Daily{Version: storeVersion, Days: map[string]*DailyRecord{}}
}

// loadJSON reads path into v. A missing file is not an error (returns false).
func loadJSON(path string, v interface{}) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, err
	}
	return true, nil
}

func loadState(profileDir string) (*State, error) {
	st := newState()
	if _, err := loadJSON(statePath(profileDir), st); err != nil {
		return nil, err
	}
	if st.Files == nil {
		st.Files = map[string]FileState{}
	}
	return st, nil
}

func loadSessions(profileDir string) (*Sessions, error) {
	s := newSessions()
	if _, err := loadJSON(sessionsPath(profileDir), s); err != nil {
		return nil, err
	}
	if s.Records == nil {
		s.Records = map[string]*SessionRecord{}
	}
	return s, nil
}

func loadDaily(profileDir string) (*Daily, error) {
	d := newDaily()
	if _, err := loadJSON(dailyPath(profileDir), d); err != nil {
		return nil, err
	}
	if d.Days == nil {
		d.Days = map[string]*DailyRecord{}
	}
	return d, nil
}

// Load reads the session and daily indexes for a profile without ingesting —
// used by read paths that only need to render what is already on disk. It takes
// the same advisory lock commit uses so a concurrent Sync can't expose a
// half-written (mixed-revision) set of index files.
func Load(profileDir string) (*Sessions, *Daily, error) {
	// No store dir → no writer to race with. Read directly rather than create
	// the dir/lock as a side effect of a pure read.
	if _, err := os.Stat(Dir(profileDir)); os.IsNotExist(err) {
		return loadIndexes(profileDir)
	}
	var sess *Sessions
	var day *Daily
	err := lock.Guard(lockPath(profileDir), lock.DefaultTimeout, func() error {
		var e error
		sess, day, e = loadIndexes(profileDir)
		return e
	})
	return sess, day, err
}

func loadIndexes(profileDir string) (*Sessions, *Daily, error) {
	sess, err := loadSessions(profileDir)
	if err != nil {
		return nil, nil, err
	}
	day, err := loadDaily(profileDir)
	if err != nil {
		return nil, nil, err
	}
	return sess, day, nil
}

// commit writes state, sessions, and daily together in a single atomic
// transaction so the ingest cursor only advances when the folded data lands.
func commit(profileDir string, st *State, sess *Sessions, day *Daily) error {
	if err := os.MkdirAll(Dir(profileDir), config.DirPerm); err != nil {
		return err
	}
	sd, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	ss, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	dd, err := json.MarshalIndent(day, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(statePath(profileDir), sd, config.FilePerm),
		atomicwrite.WriteFile(sessionsPath(profileDir), ss, config.FilePerm),
		atomicwrite.WriteFile(dailyPath(profileDir), dd, config.FilePerm),
	})
}
