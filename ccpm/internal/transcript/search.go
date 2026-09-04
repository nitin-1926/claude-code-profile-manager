package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// Scope is one searchable profile. Search takes a slice of these rather than a
// single directory so that cross-profile search (issue #17) is a UI change
// later, not a re-architecture. Today it is always called with one.
type Scope struct {
	Profile string // display name, carried onto every hit
	Dir     string // profile directory
}

// Source says which part of a turn a hit came from, so the reader knows whether
// it needs to expand a tool chip to reveal the match.
type Source string

const (
	SourceText       Source = "text"
	SourceToolUse    Source = "tool_use"
	SourceToolResult Source = "tool_result"
)

// SearchOpts tunes scope and bounds. The zero value is a sensible default.
type SearchOpts struct {
	// IncludeToolResults widens the scan to tool OUTPUT. It is off by default:
	// tool results are 81.9% of transcript content by volume and are where
	// pasted credentials and `cat` of a secrets file end up. Tool INPUTS
	// (command lines, file paths) are always searched — they are 12.2% of
	// content, low secret density, and carry the highest-recall queries
	// (a filename, a git command, an error string).
	IncludeToolResults bool
	// MaxPerSession caps hits from one transcript so a single chatty session
	// cannot fill the whole result set.
	MaxPerSession int
	// MaxResults caps the run. Reaching it stops the scan early, which is what
	// bounds a common query: without it, "the" decodes 17,530 surviving lines
	// and takes 2.2s on a 327 MB profile.
	MaxResults int
	// SnippetBytes is the display window around a match.
	SnippetBytes int
}

func (o SearchOpts) withDefaults() SearchOpts {
	if o.MaxPerSession <= 0 {
		o.MaxPerSession = 10
	}
	if o.MaxResults <= 0 {
		o.MaxResults = 200
	}
	if o.SnippetBytes <= 0 {
		o.SnippetBytes = 240
	}
	return o
}

// Hit is one matching message.
//
// The turn is identified by UUID, never by an index. An index would have to be
// produced by the search scan and consumed by the reader, and those are two
// different passes with two different filters — the search prefilter skips
// lines without decoding them, so it cannot know whether a skipped line was a
// turn. Any index it produced would drift. The reader resolves the UUID in its
// own pass instead, which is exact by construction.
type Hit struct {
	Profile   string `json:"profile"`
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	RelPath   string `json:"relPath"`
	ModTime   int64  `json:"mtime"`

	TurnUUID  string `json:"turnUuid,omitempty"`
	Role      string `json:"role"`
	Timestamp string `json:"timestamp,omitempty"`
	Source    Source `json:"source"`
	ToolName  string `json:"toolName,omitempty"`

	// Snippet is a window of the matched text. MatchStart/MatchEnd are byte
	// offsets INTO Snippet, computed by the same code that found the match, so
	// the highlighter cannot disagree with the matcher.
	Snippet    string `json:"snippet"`
	MatchStart int    `json:"matchStart"`
	MatchEnd   int    `json:"matchEnd"`
	// More counts further matches in this same message beyond the one shown.
	More int `json:"more"`
}

// SearchResult is the whole run, including what it could not do.
type SearchResult struct {
	Hits     []Hit `json:"hits"`
	Sessions int   `json:"sessions"`
	// Matches is a FLOOR, not a total: scanning a transcript stops once it has
	// produced its per-session quota, so matches beyond that point are never
	// counted. Present it as "N+" rather than "N" when Truncated is set.
	Matches int `json:"matches"`
	// Truncated is set when a cap stopped the scan; DroppedSessions counts
	// transcripts never opened because of it. The UI must say so rather than
	// implying the result set is complete.
	Truncated       bool `json:"truncated"`
	DroppedSessions int  `json:"droppedSessions"`
	// Unreadable counts transcripts that could not be read. Silently returning
	// fewer results would leave the user with no way to know the answer was
	// partial.
	Unreadable int  `json:"unreadable"`
	Cancelled  bool `json:"cancelled"`
}

func emptyResult() SearchResult { return SearchResult{Hits: []Hit{}} }

// candidate is one transcript queued for scanning.
type candidate struct {
	scope   Scope
	abs     string
	rel     string
	id      string
	modTime int64
}

// Search scans the given profiles for query and returns matching messages.
//
// Matching is case-insensitive literal substring of the whole query — not
// regex, not word-AND. One rule, applied by one function, so the snippet
// offsets handed to the highlighter always bracket what actually matched.
func Search(ctx context.Context, scopes []Scope, query string, opts SearchOpts) SearchResult {
	res := emptyResult()
	q := strings.TrimSpace(query)
	if q == "" {
		return res // no query, no file I/O
	}
	opts = opts.withDefaults()
	lowerQuery := strings.ToLower(q)

	cands, unreadable := collectCandidates(scopes)
	res.Unreadable = unreadable

	// Newest first, so the cap truncates the OLDEST sessions rather than an
	// arbitrary alphabetical tail. WalkDir yields lexical order and transcript
	// filenames are UUIDs, which are random with respect to time — capping
	// during the raw walk would drop a random subset while the UI claims
	// "newest first".
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].modTime != cands[j].modTime {
			return cands[i].modTime > cands[j].modTime
		}
		return cands[i].rel < cands[j].rel
	})

	prefilter := prefilterNeedle(lowerQuery)
	indexes := map[string]*Index{}

	for i, c := range cands {
		select {
		case <-ctx.Done():
			res.Cancelled = true
			res.DroppedSessions = len(cands) - i
			return res
		default:
		}
		if len(res.Hits) >= opts.MaxResults {
			res.Truncated = true
			res.DroppedSessions = len(cands) - i
			break
		}

		ix, ok := indexes[c.scope.Dir]
		if !ok {
			ix = LoadIndex(c.scope.Dir)
			indexes[c.scope.Dir] = ix
		}

		hits, matches, capped, err := scanFile(ctx, c, lowerQuery, prefilter, opts, opts.MaxResults-len(res.Hits))
		if err != nil {
			res.Unreadable++
			continue
		}
		res.Matches += matches
		if capped {
			// A session stopped at its quota, so Matches is a floor. Say so,
			// rather than presenting a partial count as a total.
			res.Truncated = true
		}
		if len(hits) == 0 {
			continue
		}
		res.Sessions++
		// Session metadata comes from the sidecar when present, and falls back
		// to what the file itself said. A session with no usage-bearing
		// assistant line never gets a usage record, so relying on that store
		// would make some hits unopenable.
		title, cwd := "", ""
		if e := ix.Entries[c.id]; e != nil {
			title, cwd = e.Title, e.Cwd
		}
		for j := range hits {
			hits[j].Title, hits[j].Cwd = title, cwd
		}
		res.Hits = append(res.Hits, hits...)
	}
	return res
}

// collectCandidates enumerates every searchable transcript across the scopes,
// applying the same subagent and symlink exclusions the index uses.
func collectCandidates(scopes []Scope) ([]candidate, int) {
	var out []candidate
	unreadable := 0
	for _, s := range scopes {
		err := usage.WalkTranscripts(s.Dir, "", func(abs, rel string) error {
			if skipTranscript(abs, rel) {
				return nil
			}
			fi, err := os.Stat(abs)
			if err != nil {
				unreadable++
				return nil
			}
			out = append(out, candidate{
				scope: s, abs: abs, rel: rel,
				id:      sessionIDFromPath(rel),
				modTime: fi.ModTime().Unix(),
			})
			return nil
		})
		if err != nil {
			unreadable++
		}
	}
	return out, unreadable
}

// prefilterNeedle returns the bytes to reject non-matching lines with before
// paying for a JSON decode, or nil when that shortcut is unsound.
//
// The prefilter runs against RAW JSON, where the text has been escaped. A query
// containing a quote, a backslash, a control character, or any non-ASCII rune
// may appear escaped in the file (\", \\, \n, \uXXXX) and would be missed. Such
// queries skip the prefilter and every line is decoded — slower, but correct.
func prefilterNeedle(lowerQuery string) []byte {
	for i := 0; i < len(lowerQuery); i++ {
		c := lowerQuery[i]
		if c >= 0x80 || c < 0x20 || c == '"' || c == '\\' {
			return nil
		}
	}
	return []byte(lowerQuery)
}

// scanFile streams one transcript and returns its hits, plus every match it saw
// (including those past the per-session cap, so the UI can report honestly).
func scanFile(ctx context.Context, c candidate, lowerQuery string, prefilter []byte, opts SearchOpts, budget int) (_ []Hit, _ int, capped bool, _ error) {
	var hits []Hit
	matches := 0
	lines := 0

	quota := min(opts.MaxPerSession, budget)
	err := eachLine(c.abs, func(raw []byte, skipped bool) bool {
		if skipped {
			return true
		}
		lines++
		// Cancellation is checked periodically inside a file too — the largest
		// transcript is 76.9 MB and would otherwise run to completion after the
		// user has already typed the next keystroke.
		if lines%512 == 0 {
			select {
			case <-ctx.Done():
				return false
			default:
			}
		}
		if prefilter != nil && !bytes.Contains(bytes.ToLower(raw), prefilter) {
			return true
		}
		var l rawLine
		if json.Unmarshal(raw, &l) != nil {
			return true
		}
		if !l.countsAsTurn() {
			return true
		}
		hit, n := firstHitInTurn(l, lowerQuery, opts)
		matches += n
		if n > 0 {
			hit.Profile = c.scope.Profile
			hit.SessionID = c.id
			hit.RelPath = c.rel
			hit.ModTime = c.modTime
			hits = append(hits, hit)
		}
		// Stop the file once it has produced its quota. Continuing only to keep
		// counting is what made a common query cost 2.2s on a 327 MB profile:
		// every prefilter survivor still had to be decoded. Matches is
		// documented as a floor for exactly this reason.
		if len(hits) >= quota {
			capped = true
			return false
		}
		return true
	})
	if err != nil {
		return nil, 0, false, err
	}
	return hits, matches, capped, nil
}

// searchable is one piece of a turn that is in scope, with where it came from.
type searchable struct {
	text     string
	source   Source
	toolName string
}

// searchables returns the parts of a turn the current scope covers.
//
// thinking is NEVER searched: it is hidden by default in the reader, so a hit
// there would be a result the user cannot see, and it is model scratch rather
// than anything they wrote or ran.
func searchables(l rawLine, opts SearchOpts) []searchable {
	blocks, _ := l.blocks()
	out := make([]searchable, 0, len(blocks))
	for _, b := range blocks {
		switch b.Kind {
		case KindText:
			out = append(out, searchable{text: b.Text, source: SourceText})
		case KindToolUse:
			out = append(out, searchable{text: b.ToolName + " " + b.Preview, source: SourceToolUse, toolName: b.ToolName})
		case KindToolResult:
			if opts.IncludeToolResults {
				out = append(out, searchable{text: b.Preview, source: SourceToolResult})
			}
		}
	}
	return out
}

// firstHitInTurn builds one hit for the turn's first match and counts the rest.
// One result per matching message: a message containing the term forty times
// must not flood the list, and the count is surfaced as Hit.More.
func firstHitInTurn(l rawLine, lowerQuery string, opts SearchOpts) (Hit, int) {
	total := 0
	var hit Hit
	found := false
	for _, s := range searchables(l, opts) {
		n, first := countFold(s.text, lowerQuery)
		if n == 0 {
			continue
		}
		total += n
		if found {
			continue
		}
		found = true
		snippet, ms, me := window(s.text, first, len(lowerQuery), opts.SnippetBytes)
		hit = Hit{
			TurnUUID: l.UUID, Role: l.Message.Role,
			Timestamp: l.Timestamp, Source: s.source, ToolName: s.toolName,
			Snippet: snippet, MatchStart: ms, MatchEnd: me,
		}
	}
	if !found {
		return Hit{}, 0
	}
	hit.More = total - 1
	return hit, total
}

// countFold returns how many times lowerSub occurs in s case-insensitively, and
// the byte offset of the first occurrence IN s.
//
// The fast path lowercases s wholesale, which is valid only when that does not
// change its byte length — true for ASCII and almost everything else, but not
// universally (some runes lowercase to a different width). When it does change,
// offsets into the lowered copy would not map back onto s and the highlight
// would land in the wrong place, so a rune-walking scan takes over.
func countFold(s, lowerSub string) (int, int) {
	if s == "" || lowerSub == "" {
		return 0, -1
	}
	ls := strings.ToLower(s)
	if len(ls) == len(s) {
		count, first, from := 0, -1, 0
		for {
			i := strings.Index(ls[from:], lowerSub)
			if i < 0 {
				return count, first
			}
			at := from + i
			if first < 0 {
				first = at
			}
			count++
			from = at + len(lowerSub)
		}
	}
	// Fold-width fallback: walk rune starts and lower a bounded chunk at each.
	// Advancing one rune per match means overlapping occurrences are each
	// counted, so More can read slightly high for a self-overlapping query in
	// text containing such a rune. That is the only consequence, and this path
	// is unreachable for ASCII.
	count, first := 0, -1
	for i := 0; i < len(s); {
		if !utf8RuneStart(s[i]) {
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		chunk := s[i:min(i+len(lowerSub)*4, len(s))]
		if strings.HasPrefix(strings.ToLower(chunk), lowerSub) {
			if first < 0 {
				first = i
			}
			count++
		}
		i += size
	}
	return count, first
}

// window cuts a display snippet around a match and returns the match's offsets
// within that snippet. Cuts land on rune boundaries so the snippet is always
// valid UTF-8.
func window(s string, at, matchLen, size int) (string, int, int) {
	if at < 0 {
		return ClipRunes(s, size), 0, 0
	}
	start := max(at-size/2, 0)
	end := min(at+matchLen+size/2, len(s))
	for start > 0 && start < len(s) && !utf8RuneStart(s[start]) {
		start--
	}
	for end < len(s) && !utf8RuneStart(s[end]) {
		end++
	}
	snippet := s[start:end]
	ms := at - start
	me := ms + matchLen
	me = min(me, len(snippet))
	ms = min(ms, len(snippet))
	return snippet, ms, me
}

func utf8RuneStart(b byte) bool { return utf8.RuneStart(b) }

// ResolvePath turns a hit or entry's RelPath into an absolute path, verifying
// it stays inside the profile's projects/ directory.
//
// This is the guard between on-disk JSON and an os.Open. Session ids and
// relative paths come out of transcript files, and a profile directory can be
// shared or restored from elsewhere; filepath.Join cleans ".." but does not
// sandbox, so without this check a crafted value is an arbitrary-file-read
// primitive that renders whatever it points at into the reader.
func ResolvePath(profileDir, relPath string) (string, error) {
	root := filepath.Join(profileDir, "projects")
	full := filepath.Join(root, relPath)
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", os.ErrNotExist
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", os.ErrNotExist
	}
	return full, nil
}
