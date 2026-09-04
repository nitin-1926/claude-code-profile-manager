package transcript

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	ModTime   int64                   `json:"mtime"`
	Size      int64                   `json:"size"`
}

// Index is the sidecar, keyed by session id.
type Index struct {
	Version int               `json:"version"`
	Entries map[string]*Entry `json:"entries"`
}

func newIndex() *Index { return &Index{Version: indexVersion, Entries: map[string]*Entry{}} }

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
	return &ix
}

// BuildIndex refreshes the sidecar for one profile and returns it.
//
// Only transcripts whose size or mtime changed are re-scanned; everything else
// is reused. Entries whose transcript has since been deleted are RETAINED — the
// whole point of a sidecar is that history does not evaporate when Claude Code
// prunes a file.
func BuildIndex(profileDir string) (*Index, error) {
	ix := LoadIndex(profileDir)
	changed := false

	err := usage.WalkTranscripts(profileDir, "", func(abs, rel string) error {
		if skipTranscript(abs, rel) {
			return nil
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil // unreadable right now (native claude mid-write); try again next time
		}
		id := sessionIDFromPath(rel)
		if prev, ok := ix.Entries[id]; ok && prev.ModTime == fi.ModTime().Unix() && prev.Size == fi.Size() {
			return nil // unchanged since last build
		}
		meta, err := Scan(abs)
		if err != nil {
			return nil
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
			Title:     meta.Title,
			Model:     meta.Model,
			ByModel:   meta.ByModel,
			Cwd:       meta.Cwd,
			GitBranch: meta.GitBranch,
			FirstTS:   meta.FirstTS,
			LastTS:    meta.LastTS,
			Turns:     meta.Turns,
			RelPath:   rel,
			ModTime:   fi.ModTime().Unix(),
			Size:      fi.Size(),
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
// files (work), 81 of 110 (labs), 31 of 53 (cin), and their content is copied
// verbatim into the parent session as sidechain turns. Indexing them would list
// each subagent as a session and return every search hit twice — once from the
// subagent file and once from the parent. They stay reachable through the
// parent's sidechain toggle.
//
// Symlinks are rejected because a profile directory can be shared or restored
// from elsewhere, and filepath.WalkDir happily reports a symlinked *file*; Go's
// open then follows it. A link named evil.jsonl pointing at ~/.ssh/id_rsa would
// otherwise be scanned and rendered.
func skipTranscript(abs, rel string) bool {
	if slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "subagents") {
		return true
	}
	fi, err := os.Lstat(abs)
	if err != nil || fi.Mode()&fs.ModeSymlink != 0 {
		return true
	}
	return false
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
	counted := map[string]bool{}

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
		// Per-model tokens, deduped the way usage does it: one API response is
		// written as several assistant lines, so count each id once.
		if l.Type == "assistant" && l.Message.Usage != nil {
			key := l.UUID
			if key == "" {
				key = l.Timestamp + "|" + l.Message.Model
			}
			if !counted[key] {
				counted[key] = true
				model := l.Message.Model
				if model == "" {
					model = "unknown"
				}
				t := m.ByModel[model]
				t.Add(usage.Tokens{
					Input:         l.Message.Usage.InputTokens,
					Output:        l.Message.Usage.OutputTokens,
					CacheCreation: l.Message.Usage.CacheCreationInputTokens,
					CacheRead:     l.Message.Usage.CacheReadInputTokens,
				})
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
// actually used. Falls back to the last-seen model when an older entry has no
// per-model tally.
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
