//go:build darwin

package services

import (
	"context"
	"os"
	"sort"
	"sync"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/transcript"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// HistorySession is one row of the History tab.
//
// Responses is named for what it counts: usage.SessionRecord.Messages is the
// number of deduped usage-bearing assistant lines, which is not the turn count
// the reader shows. Calling it "messages" would have the two surfaces disagree
// on the same session for no reason.
type HistorySession struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Cwd       string  `json:"cwd"`
	Branch    string  `json:"branch"`
	Model     string  `json:"model"`
	Responses int64   `json:"responses"`
	Turns     int     `json:"turns"`
	Tokens    int64   `json:"tokens"`
	Cost      float64 `json:"cost"`
	FirstTS   string  `json:"firstTs"`
	LastTS    string  `json:"lastTs"`
	// Openable is false for a session the usage store knows about but whose
	// transcript is gone, so the UI can show the row without offering a reader
	// that would fail.
	Openable bool `json:"openable"`
}

// HistoryPage is a window of a session's conversation.
type HistoryPage struct {
	Turns         []transcript.Turn `json:"turns"`
	Total         int               `json:"total"`
	Offset        int               `json:"offset"`
	UnknownBlocks int               `json:"unknownBlocks"`
	SkippedLines  int               `json:"skippedLines"`
	// TargetIndex is where a jump-to-turn landed, or -1. The UI scrolls to and
	// flashes it.
	TargetIndex int `json:"targetIndex"`
}

// HistoryToolBody is one expanded tool payload.
type HistoryToolBody struct {
	Body      string `json:"body"`
	FullBytes int    `json:"fullBytes"`
	Truncated bool   `json:"truncated"`
}

// HistoryService serves the History tab: the session list, the reader, and
// search. It never calls usage.Sync — see Sessions.
type HistoryService struct {
	mu sync.Mutex
	// cancels maps a caller-supplied search token to its cancel func.
	cancels map[string]context.CancelFunc
	// tombstones remembers cancels that arrived before the search they name.
	// Wails v2 dispatches every bound call on its own goroutine, so a debounced
	// UI can easily land CancelSearch(N) before Search(N) has registered; without
	// this the cancel is a silent no-op and a full scan runs anyway.
	tombstones map[string]bool
}

func NewHistory() *HistoryService {
	return &HistoryService{
		cancels:    map[string]context.CancelFunc{},
		tombstones: map[string]bool{},
	}
}

// profileDir resolves a profile name to its directory, or "" when unknown.
func profileDir(name string) string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		return ""
	}
	return pc.Dir
}

// Sessions lists a profile's sessions, newest first.
//
// It reads the usage store but never syncs it. <profileDir>/usage sits inside
// the watched ~/.ccpm tree, and usage.Sync rewrites all three store files
// unconditionally, so syncing here would emit a change event on every History
// fetch. UsageService.Get already owns the sync; History is a reader.
//
// The list is the union of the sidecar (every transcript on disk) and the usage
// session index (token counts). Neither alone is complete: a session with no
// usage-bearing assistant line never gets a usage record, and a session whose
// transcript Claude Code has pruned lives on only in the store.
func (s *HistoryService) Sessions(profile string) ([]HistorySession, error) {
	out := []HistorySession{}
	dir := profileDir(profile)
	if dir == "" {
		return out, nil
	}

	ix, _ := transcript.BuildIndex(dir) // a build error still yields a usable index
	records := map[string]*usage.SessionRecord{}
	if sess, _, err := usage.Load(dir); err == nil && sess != nil {
		records = sess.Records
	}

	seen := map[string]bool{}
	for id, e := range ix.Entries {
		seen[id] = true
		row := HistorySession{
			ID: id, Title: e.Title, Cwd: e.Cwd, Branch: e.GitBranch, Model: e.Model,
			Turns: e.Turns, FirstTS: e.FirstTS, LastTS: e.LastTS,
			Tokens: e.Tokens().Total(), Cost: e.Cost(),
			Openable: transcriptExists(dir, e.RelPath),
		}
		if r := records[id]; r != nil {
			row.Responses = r.Messages
			if row.Branch == "" {
				row.Branch = r.GitBranch
			}
			if row.LastTS == "" {
				row.LastTS = r.LastTS
			}
		}
		out = append(out, row)
	}
	// Sessions the store knows but whose transcript is gone: still real history,
	// just not readable.
	for id, r := range records {
		if seen[id] {
			continue
		}
		out = append(out, HistorySession{
			ID: id, Title: r.Slug, Cwd: r.Cwd, Branch: r.GitBranch,
			Responses: r.Messages, Tokens: r.Tokens.Total(),
			FirstTS: r.FirstTS, LastTS: r.LastTS, Openable: false,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastTS != out[j].LastTS {
			return out[i].LastTS > out[j].LastTS
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func transcriptExists(dir, relPath string) bool {
	if relPath == "" {
		return false
	}
	full, err := transcript.ResolvePath(dir, relPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(full)
	return err == nil
}

// resolve turns a profile and session id into a transcript path, refusing
// anything that escapes <profileDir>/projects.
//
// This is the guard between on-disk JSON and an os.Open. The frontend never
// passes a path — only a session id, which is looked up in the sidecar — and
// the resulting relative path is still sandboxed, because a profile directory
// can be shared or restored from elsewhere.
func resolveTranscript(profile, sessionID string) (string, error) {
	dir := profileDir(profile)
	if dir == "" {
		return "", os.ErrNotExist
	}
	e := transcript.LoadIndex(dir).Entries[sessionID]
	if e == nil || e.RelPath == "" {
		return "", os.ErrNotExist
	}
	return transcript.ResolvePath(dir, e.RelPath)
}

func emptyPage() HistoryPage {
	return HistoryPage{Turns: []transcript.Turn{}, TargetIndex: -1}
}

// Transcript returns a window of a session's conversation.
func (s *HistoryService) Transcript(profile, sessionID string, offset, limit int) (HistoryPage, error) {
	path, err := resolveTranscript(profile, sessionID)
	if err != nil {
		return emptyPage(), nil
	}
	page, err := transcript.ReadPage(path, offset, limit)
	if err != nil {
		return emptyPage(), nil
	}
	return HistoryPage{
		Turns: page.Turns, Total: page.Total, Offset: offset,
		UnknownBlocks: page.UnknownBlocks, SkippedLines: page.SkippedLines,
		TargetIndex: -1,
	}, nil
}

// TranscriptAround opens the window containing a specific turn, for arriving
// from a search hit. An unknown uuid falls back to the first page rather than
// erroring — the transcript may have grown since the search ran.
func (s *HistoryService) TranscriptAround(profile, sessionID, turnUUID string, limit int) (HistoryPage, error) {
	path, err := resolveTranscript(profile, sessionID)
	if err != nil {
		return emptyPage(), nil
	}
	page, at, err := transcript.PageAround(path, turnUUID, limit)
	if err != nil {
		return emptyPage(), nil
	}
	offset := 0
	if len(page.Turns) > 0 {
		offset = page.Turns[0].Index
	}
	return HistoryPage{
		Turns: page.Turns, Total: page.Total, Offset: offset,
		UnknownBlocks: page.UnknownBlocks, SkippedLines: page.SkippedLines,
		TargetIndex: at,
	}, nil
}

// ToolBody returns one tool payload in full, for an expanded chip.
func (s *HistoryService) ToolBody(profile, sessionID, turnUUID string, blockIndex int) (HistoryToolBody, error) {
	path, err := resolveTranscript(profile, sessionID)
	if err != nil {
		return HistoryToolBody{}, nil
	}
	body, full, truncated, err := transcript.ToolBody(path, turnUUID, blockIndex, 0)
	if err != nil {
		return HistoryToolBody{}, nil
	}
	return HistoryToolBody{Body: body, FullBytes: full, Truncated: truncated}, nil
}

// maxTombstones bounds the cancel-before-register map. A tombstone is normally
// consumed by the search it names; this only catches the case where that search
// never arrives.
const maxTombstones = 64

// Search scans a profile's transcripts. token identifies the run so the UI can
// cancel it — a dropped JS promise does not stop a Go goroutine, so an
// un-cancelled superseded scan would resolve later and clobber newer results.
func (s *HistoryService) Search(profile, query, token string, includeToolResults bool) (transcript.SearchResult, error) {
	dir := profileDir(profile)
	if dir == "" {
		return transcript.SearchResult{Hits: []transcript.Hit{}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.tombstones[token] {
		delete(s.tombstones, token)
		s.mu.Unlock()
		cancel()
		return transcript.SearchResult{Hits: []transcript.Hit{}, Cancelled: true}, nil
	}
	if token != "" {
		s.cancels[token] = cancel
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if token != "" {
			delete(s.cancels, token)
		}
		s.mu.Unlock()
		cancel()
	}()

	// Scoped to one profile today. The slice is what makes cross-profile search
	// (issue #17) a UI change rather than a re-architecture.
	scopes := []transcript.Scope{{Profile: profile, Dir: dir}}
	return transcript.Search(ctx, scopes, query, transcript.SearchOpts{
		IncludeToolResults: includeToolResults,
	}), nil
}

// CancelSearch stops the scan with the given token. Cancelling a token that has
// not registered yet leaves a tombstone so the scan returns immediately when it
// does start.
func (s *HistoryService) CancelSearch(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, ok := s.cancels[token]; ok {
		delete(s.cancels, token)
		cancel()
		return
	}
	if len(s.tombstones) >= maxTombstones {
		s.tombstones = map[string]bool{}
	}
	s.tombstones[token] = true
}
