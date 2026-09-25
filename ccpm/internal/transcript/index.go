package transcript

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// indexVersion is this sidecar's own schema version, deliberately independent of
// usage.storeVersion.
//
// This index exists precisely so that adding a title and a model does NOT
// require bumping the usage store. A usage bump discards the store and rebuilds
// from transcripts that still exist, and Claude Code prunes transcripts — on one
// real profile 68 of 90 session records had no transcript left on disk, so a
// bump would have erased 76% of the session history to gain a title string.
// Keeping the records while resetting the ingest cursors is worse still: the
// per-file dedup map seeds from the previous run's tail, so a reset cursor
// re-counts every surviving session's tokens.
//
// This file is additive, rebuildable from nothing, and never authoritative for
// token accounting.
const indexVersion = 1

// Entry is what the sidecar knows about one session.
//
// RelPath is the single most important field: it is the path the walk actually
// handed us, so opening a transcript never depends on reconstructing a directory
// name from a cwd. usage.EncodeCwd is not used here at all.
type Entry struct {
	SessionID string                  `json:"session_id"`
	Title     string                  `json:"title,omitempty"`
	Model     string                  `json:"model,omitempty"`
	ByModel   map[string]usage.Tokens `json:"by_model,omitempty"`
	Cwd       string                  `json:"cwd,omitempty"`
	GitBranch string                  `json:"git_branch,omitempty"`
	FirstTS   string                  `json:"first_ts,omitempty"`
	LastTS    string                  `json:"last_ts,omitempty"`
	Turns     int                     `json:"turns"`
	RelPath   string                  `json:"rel_path"`
	// SubPaths are the session's subagent transcripts, relative to projects/.
	// They belong to this session — Claude Code does NOT copy their content back
	// into the parent (measured: isSidechain is present on 68,167 lines across 77
	// real parent transcripts and false on every one of them), so without these
	// the work a subagent did is unreachable.
	SubPaths []string `json:"sub_paths,omitempty"`
	ModTime  int64    `json:"mtime"`
	Size     int64    `json:"size"`
}

// Index is the sidecar, keyed by session id.
type Index struct {
	Version int               `json:"version"`
	Entries map[string]*Entry `json:"entries"`

	// pruned records that LoadIndex dropped a null entry, so BuildIndex knows
	// the on-disk copy differs from this one even when no transcript changed.
	// Unexported, so encoding/json ignores it without needing a tag.
	pruned bool
}

func newIndex() *Index { return &Index{Version: indexVersion, Entries: map[string]*Entry{}} }

// IsIndexFile reports whether path is the history sidecar or one of the
// siblings atomicwrite creates while replacing it (<path>.ccpm-staged-<rand>,
// <path>.ccpm-rollback).
//
// The desktop watcher uses it to ignore the sidecar's own writes. Matching the
// exact name was not enough: atomicwrite stages to a sibling and renames it
// into place, so the CREATE and RENAME events arrive under the staged name and
// every index save still fired a refresh.
func IsIndexFile(path string) bool {
	base := filepath.Base(path)
	return base == "history.json" || strings.HasPrefix(base, "history.json.ccpm-")
}

// IndexPath is the sidecar's location. It sits beside the usage store because
// it is derived from the same transcripts, but it is written and read
// independently.
func IndexPath(profileDir string) string {
	return filepath.Join(usage.Dir(profileDir), "history.json")
}

// LoadIndex reads the sidecar. A missing, unreadable, or stale-versioned file is
// not an error — it yields an empty index, which BuildIndex then repopulates.
func LoadIndex(profileDir string) *Index {
	b, err := os.ReadFile(IndexPath(profileDir))
	if err != nil {
		return newIndex()
	}
	var ix Index
	if json.Unmarshal(b, &ix) != nil || ix.Version != indexVersion || ix.Entries == nil {
		return newIndex()
	}
	// `{"entries": {"x": null}}` unmarshals to a nil *Entry, and every consumer
	// dereferences what it finds. The sidecar lives inside a profile directory
	// that may have been shared or restored, so it is not necessarily a file
	// this code wrote.
	for id, e := range ix.Entries {
		if e == nil {
			delete(ix.Entries, id)
			// Recorded so BuildIndex writes the cleaned map back. Pruning only
			// in memory meant the nulls survived on disk and were re-pruned on
			// every single load, forever.
			ix.pruned = true
		}
	}
	return &ix
}

// indexLocks serialises BuildIndex per profile directory. Keyed rather than a
// single mutex so a slow scan of one profile does not block another's — the
// desktop shows one profile at a time, but nothing enforces that.
var indexLocks sync.Map // profileDir -> *sync.Mutex

func lockProfileIndex(profileDir string) func() {
	v, _ := indexLocks.LoadOrStore(profileDir, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// BuildIndex refreshes the sidecar for one profile and returns it.
//
// Only transcripts whose size or mtime changed are re-scanned; everything else
// is reused. Entries whose transcript has since been deleted are RETAINED — the
// whole point of a sidecar is that history does not evaporate when Claude Code
// prunes a file.
func BuildIndex(profileDir string) (*Index, error) {
	// Serialised per profile. BuildIndex is a read-modify-write of history.json
	// and Wails dispatches every bound method on its own goroutine, so two
	// overlapping Sessions calls — a tab remount racing a ccpm:changed refetch —
	// could each load the same map, scan, and save, with the slower one writing
	// back a map missing whatever the faster one had just added. Self-healing on
	// the next build, but it costs a stale list render.
	unlock := lockProfileIndex(profileDir)
	defer unlock()

	ix := LoadIndex(profileDir)
	changed := ix.pruned

	err := usage.WalkTranscripts(profileDir, "", func(abs, rel string) error {
		if skipTranscript(abs, rel) {
			return nil
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil // unreadable right now (native claude mid-write); try again next time
		}
		id := sessionIDFromPath(rel)
		// The freshness signature covers the session's subagent transcripts too,
		// since their usage folds into this entry.
		subs := subagentTranscripts(abs)
		mtime, size := signature(fi, subs)
		subRels := relSubPaths(profileDir, subs)
		// SubPaths is part of the freshness check, not just the signature.
		//
		// It was added after the subagent-aware signature, so sidecars written
		// in between carry a matching mtime/size with no sub_paths at all. On a
		// size-and-mtime check alone those entries look fresh forever and the
		// field is never backfilled, leaving the session's subagent transcripts
		// permanently unsearchable and unopenable — the reader's allowlist is
		// built from RelPath plus SubPaths. Measured on three real profiles: 12
		// sessions had subagent transcripts on disk and 8 of them had no
		// sub_paths recorded.
		//
		// slices.Equal treats nil and empty as equal, so a session that simply
		// has no subagents still short-circuits here rather than rebuilding on
		// every pass.
		// RelPath too: a transcript moved with `mv` or `cp -p` keeps its size
		// and mtime, and an entry fresh on those alone kept pointing at the old
		// path, leaving the row permanently unopenable.
		if prev, ok := ix.Entries[id]; ok && prev.ModTime == mtime && prev.Size == size &&
			slices.Equal(prev.SubPaths, subRels) && prev.RelPath == filepath.ToSlash(rel) {
			return nil // unchanged since last build
		}
		meta, err := Scan(abs)
		if err != nil {
			return nil
		}
		// Fold each subagent transcript's usage into the parent.
		//
		// Subagent lines carry the PARENT's sessionId, so internal/usage already
		// attributes their tokens to this session; skipping them here entirely
		// would leave History reporting 5-14% less than the Usage tab for the
		// same session. They are not indexed as sessions of their OWN — that
		// would bury the real rows — but they ARE searched and readable through
		// this parent, via SubPaths (see search.go's collectCandidates).
		//
		// An earlier version of this comment said their text was duplicated into
		// the parent as sidechain turns, and that they were not searched. Both
		// were wrong, and it is the belief this branch exists to correct:
		// isSidechain is present on 68,167 lines across 77 real parent
		// transcripts and false on every one, so nothing is copied back.
		//
		// Each file is deduped independently, exactly as usage.ingestFile does,
		// so the two agree by construction.
		for _, sub := range subs {
			sm, serr := Scan(sub)
			if serr != nil {
				continue
			}
			for model, tok := range sm.ByModel {
				t := meta.ByModel[model]
				t.Add(tok)
				meta.ByModel[model] = t
			}
		}
		// Key on the filename, not the sessionId inside the JSON. Claude Code
		// names each transcript after its session and `--resume` matches on
		// that name, so the filename is authoritative; trusting the field
		// instead lets two transcripts whose content disagrees collide into one
		// entry and silently drop a session. cmd/sessions.go already prefers
		// the filename for the same reason.
		if id == "" {
			id = meta.SessionID
		}
		if id == "" {
			return nil
		}
		ix.Entries[id] = &Entry{
			SessionID: id,
			SubPaths:  subRels,
			Title:     meta.Title,
			Model:     meta.Model,
			ByModel:   meta.ByModel,
			Cwd:       meta.Cwd,
			GitBranch: meta.GitBranch,
			FirstTS:   meta.FirstTS,
			LastTS:    meta.LastTS,
			Turns:     meta.Turns,
			RelPath:   filepath.ToSlash(rel),
			ModTime:   mtime,
			Size:      size,
		}
		changed = true
		return nil
	})
	if err != nil {
		return ix, err
	}
	if !changed {
		return ix, nil
	}
	return ix, saveIndex(profileDir, ix)
}

func saveIndex(profileDir string, ix *Index) error {
	if err := os.MkdirAll(usage.Dir(profileDir), config.DirPerm); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ix, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(IndexPath(profileDir), b, config.FilePerm),
	})
}

// skipTranscript rejects the files that must never become their own session.
//
// Subagent transcripts are the big one: on real profiles they are 88 of 113
// files (work), 81 of 110 (labs), 31 of 53 (cin). They are skipped as SESSIONS
// because listing each one as its own row would bury the real sessions under a
// pile of agent-*.jsonl entries nobody started.
//
// They are NOT skipped as content. This comment used to claim their text is
// copied verbatim into the parent as sidechain turns, which is false and is the
// belief that made three quarters of a profile unsearchable: isSidechain is
// present on 68,167 lines across 77 real parent transcripts and false on every
// one. They are indexed against the parent via Entry.SubPaths, searched by
// collectCandidates, and readable through the parent.
//
// Symlinks are rejected because a profile directory can be shared or restored
// from elsewhere, and filepath.WalkDir happily reports a symlinked *file*; Go's
// open then follows it. A link named evil.jsonl pointing at ~/.ssh/id_rsa would
// otherwise be scanned and rendered.
func skipTranscript(abs, rel string) bool {
	if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "subagents") {
		return true
	}
	return isSymlink(abs)
}

// subagentTranscripts returns the subagent transcripts belonging to the session
// whose transcript is at abs. Claude Code writes them beside it, under a
// directory named for the session: <dir>/<id>.jsonl and <dir>/<id>/subagents/.
func subagentTranscripts(abs string) []string {
	root := filepath.Join(strings.TrimSuffix(abs, ".jsonl"), "subagents")
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil
	}
	// Walk the whole subtree, not just the top level: workflow runs nest another
	// level deep as subagents/workflows/wf_<id>/agent-*.jsonl. Reading only the
	// immediate directory left one real session 10% short of the Usage tab.
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		if fi, lerr := os.Lstat(p); lerr != nil || fi.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		out = append(out, p)
		return nil
	})
	sort.Strings(out) // stable signature regardless of walk order
	return out
}

// relSubPaths converts absolute subagent transcript paths into the
// projects-relative, forward-slashed form stored in Entry.SubPaths and matched
// by the reader's allowlist. Always non-nil so an entry never serializes
// sub_paths as null.
func relSubPaths(profileDir string, subs []string) []string {
	root := filepath.Join(profileDir, "projects")
	out := make([]string, 0, len(subs))
	for _, sub := range subs {
		if r, rerr := filepath.Rel(root, sub); rerr == nil {
			out = append(out, filepath.ToSlash(r))
		}
	}
	return out
}

// signature combines a transcript and its subagents into one freshness key, so
// a new subagent turn re-scans the parent entry whose tally it changes.
func signature(fi os.FileInfo, subs []string) (mtime, size int64) {
	mtime, size = fi.ModTime().Unix(), fi.Size()
	for _, p := range subs {
		si, err := os.Stat(p)
		if err != nil {
			continue
		}
		size += si.Size()
		if m := si.ModTime().Unix(); m > mtime {
			mtime = m
		}
	}
	return mtime, size
}

// parentSessionID maps a transcript's relative path to the session it belongs
// to, and reports whether it is a subagent transcript.
//
// Subagent files live at projects/<enc-cwd>/<sessionID>/subagents/... (workflow
// runs nest one level deeper), so the owning session is the path segment
// immediately before "subagents".
func parentSessionID(rel string) (id string, subagent bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		if p == "subagents" && i > 0 {
			return parts[i-1], true
		}
	}
	return sessionIDFromPath(rel), false
}

// isSymlink reports whether path is a symbolic link. A shared or restored
// profile can contain one pointing anywhere, so neither the index nor search
// will open it.
func isSymlink(abs string) bool {
	fi, err := os.Lstat(abs)
	return err != nil || fi.Mode()&fs.ModeSymlink != 0
}

// sessionIDFromPath takes the id from the filename, matching how Claude Code
// names transcripts and how cmd/sessions.go already derives it.
func sessionIDFromPath(rel string) string {
	return strings.TrimSuffix(filepath.Base(rel), ".jsonl")
}

// Meta is what one full pass over a transcript learns about it.
type Meta struct {
	SessionID string
	Title     string
	Model     string
	ByModel   map[string]usage.Tokens
	Cwd       string
	GitBranch string
	FirstTS   string
	LastTS    string
	Turns     int
}

// Scan reads a transcript once and extracts everything the session list needs.
//
// Title resolution, in precedence order:
//  1. an "ai-title" line — Claude Code's own generated name, present in roughly
//     38% of sampled transcripts, and always the best answer when it exists
//  2. the first user prompt that is not meta and not a slash-command envelope
//
// An ai-title wins regardless of where it appears in the file, which is why this
// cannot short-circuit after the first few lines the way a header sniff does.
//
// ByModel is a genuine per-model tally, so a session's cost reconciles with the
// Usage tab (which prices per model per day) instead of contradicting it by up
// to 5x on a session that switched models mid-flight.
func Scan(path string) (Meta, error) {
	m := Meta{ByModel: map[string]usage.Tokens{}}
	var firstPrompt string
	// key -> tokens already attributed, so a later, larger snapshot for the same
	// request revises the tally by the delta instead of being summed on top.
	counted := map[string]usage.Tokens{}

	err := eachLine(path, func(raw []byte, skipped bool) bool {
		if skipped {
			return true
		}
		var l rawLine
		if json.Unmarshal(raw, &l) != nil {
			return true
		}
		if l.SessionID != "" && m.SessionID == "" {
			m.SessionID = l.SessionID
		}
		// FIRST-seen cwd, not last. A session's cwd drifts — 7 of 25 measured
		// sessions recorded more than one, and some ended in a subdirectory of
		// where they began. The transcript lives in the encoded directory of
		// the cwd Claude Code saw when it created the file, i.e. the first one,
		// so last-write-wins would display a project that disagrees with where
		// the transcript actually sits. usage.SessionRecord.Cwd takes the last
		// and is only used for grouping, where the distinction does not bite.
		if m.Cwd == "" {
			m.Cwd = l.Cwd
		}
		if l.GitBranch != "" {
			m.GitBranch = l.GitBranch
		}
		if l.Timestamp != "" {
			if m.FirstTS == "" || l.Timestamp < m.FirstTS {
				m.FirstTS = l.Timestamp
			}
			if l.Timestamp > m.LastTS {
				m.LastTS = l.Timestamp
			}
		}
		if l.Type == "ai-title" && l.AITitle != "" {
			m.Title = ClipRunes(strings.TrimSpace(l.AITitle), TitleRunes)
			return true
		}
		if !l.countsAsTurn() {
			return true
		}
		m.Turns++

		if l.Message.Model != "" {
			m.Model = l.Message.Model
		}
		// Per-model tokens, deduped exactly the way internal/usage does it:
		// (message.id + requestId) with the LARGEST snapshot winning. Claude Code
		// writes one response as several assistant lines sharing a message.id
		// with growing usage, so a naive per-line sum over-counts about 2x.
		if key := l.usageKey(); key != "" {
			tok := l.usageTokens()
			prev, seen := counted[key]
			if !seen || tok.Total() > prev.Total() {
				counted[key] = tok
				model := l.Message.Model
				if model == "" {
					model = "unknown"
				}
				t := m.ByModel[model]
				t.Add(tok.Minus(prev)) // prev is zero when unseen
				m.ByModel[model] = t
			}
		}

		if firstPrompt == "" && l.Type == "user" && !l.IsMeta && !l.IsSidechain {
			if p := promptText(l); isTitleWorthy(p) {
				firstPrompt = strings.TrimSpace(p)
			}
		}
		return true
	})
	if err != nil {
		return m, err
	}
	if m.Title == "" && firstPrompt != "" {
		m.Title = ClipRunes(collapseWS(firstLine(firstPrompt)), TitleRunes)
	}
	return m, nil
}

// firstLine takes the first non-empty line of a prompt, the way a commit
// message's subject is its first line. Without it, a headless session whose
// opening prompt is a long instruction block gets 200 runes of that block as
// its title — technically within the cap and useless in a row.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return s
}

// promptText pulls the first text out of a user line's content, handling both
// the bare-string and typed-block shapes.
func promptText(l rawLine) string {
	blocks, _ := l.blocks()
	for _, b := range blocks {
		if b.Kind == KindText {
			return b.Text
		}
	}
	return ""
}

// collapseWS flattens newlines and runs of spaces so a multi-line prompt reads
// as a single-line title.
func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// Cost returns the estimated USD for an entry, summed across every model it
// actually used, so a session that switched models is priced at each model's
// own rate. An entry with no per-model tally has no countable usage and costs
// nothing — Model is a display label and is deliberately not used for pricing.
func (e *Entry) Cost() float64 {
	if len(e.ByModel) == 0 {
		return 0
	}
	var total float64
	for model, tok := range e.ByModel {
		total += usage.CostFor(model, tok)
	}
	return total
}

// Tokens returns the entry's total token count across all models.
func (e *Entry) Tokens() usage.Tokens {
	var t usage.Tokens
	for _, tok := range e.ByModel {
		t.Add(tok)
	}
	return t
}
