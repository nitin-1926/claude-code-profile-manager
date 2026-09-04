// Package transcript decodes the Claude Code session transcripts a profile
// accumulates under <profileDir>/projects/ into render-ready conversation
// turns, and searches their text.
//
// It is deliberately separate from internal/usage. That package answers "how
// many tokens did this cost", decoding the thin usage shape and nothing else;
// this one answers "what was actually said", which needs the full content-block
// model. The dependency runs one way only — transcript imports usage (for
// WalkTranscripts), never the reverse.
//
// Nothing here has a build tag: it compiles and race-tests on every CI OS.
package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

const (
	// maxLineBytes is the largest JSONL line we will read. The longest line
	// measured in a real profile was 1.3 MB (a single tool result), so this is
	// generous headroom; a line beyond it is counted and skipped rather than
	// aborting the file, because one pathological line must not cost the reader
	// every turn after it.
	//
	// This bounds ALLOCATION, not just decode cost: a transcript truncated or
	// concatenated without a trailing newline would otherwise have its entire
	// length materialised by ReadBytes before the cap could be consulted — 77 MB
	// on the largest file observed, more with buffer growth.
	maxLineBytes = 8 << 20

	// previewBytes is how much of a tool input or tool result is carried inline
	// on a Turn. The reader shows a one-line chip; the full body is fetched only
	// when the chip is expanded. This is what keeps an 80 MB transcript off the
	// Wails bridge.
	previewBytes = 2048

	// textBytes caps a single prose block. Total visible prose in even the
	// largest measured session is under 1 MB, so this only fires on a pasted
	// wall of text, where truncation is the kindness.
	textBytes = 64 << 10

	// TitleRunes caps a persisted session title. The title falls back to the
	// first user prompt, which can be a pasted stack trace; without a cap that
	// would be written verbatim into the sidecar index and sent across the
	// bridge once per row.
	TitleRunes = 200
)

// Kind is the sort of content one block carries.
type Kind string

const (
	KindText       Kind = "text"
	KindThinking   Kind = "thinking"
	KindToolUse    Kind = "tool_use"
	KindToolResult Kind = "tool_result"
	KindImage      Kind = "image"
	// KindUnknown is any block type Claude Code introduces that this package
	// does not model. Unknown blocks are preserved and surfaced rather than
	// dropped, so a format change shows up as a visible placeholder instead of
	// silently missing content.
	KindUnknown Kind = "unknown"
)

// Block is one piece of a turn's content.
type Block struct {
	Kind Kind `json:"kind"`
	// RawType is the type string as written, populated for KindUnknown so the
	// UI can name what it could not render.
	RawType string `json:"rawType,omitempty"`
	// Text carries prose for KindText and KindThinking.
	Text string `json:"text,omitempty"`
	// ToolName and ToolUseID identify a tool call and pair a result to it.
	ToolName  string `json:"toolName,omitempty"`
	ToolUseID string `json:"toolUseId,omitempty"`
	// Preview is the truncated tool input (KindToolUse) or tool output
	// (KindToolResult).
	Preview string `json:"preview,omitempty"`
	// FullBytes is the true size of what Preview was cut from, so the UI can
	// decide whether expanding is worth it.
	FullBytes int  `json:"fullBytes"`
	Truncated bool `json:"truncated"`
	IsError   bool `json:"isError,omitempty"`
}

// Turn is one visible unit of the conversation.
//
// Index is the turn's ordinal under the enumeration rule documented on
// countsAsTurn. It is deliberately independent of any UI filter, so paging and
// jump-to-turn keep addressing the same turn when the reader's thinking and
// sidechain toggles change.
type Turn struct {
	Index       int     `json:"index"`
	UUID        string  `json:"uuid,omitempty"`
	Role        string  `json:"role"`
	Timestamp   string  `json:"timestamp,omitempty"`
	Model       string  `json:"model,omitempty"`
	IsSidechain bool    `json:"isSidechain"`
	IsMeta      bool    `json:"isMeta"`
	Blocks      []Block `json:"blocks"`
}

// Page is a window of turns plus what the scan learned about the whole file.
type Page struct {
	Turns []Turn `json:"turns"`
	// Total is every turn in the file, not just this window.
	Total int `json:"total"`
	// UnknownBlocks counts blocks whose type this package does not model —
	// a non-zero value here is the early warning that the transcript format
	// has moved.
	UnknownBlocks int `json:"unknownBlocks"`
	// SkippedLines counts lines too long to decode.
	SkippedLines int `json:"skippedLines"`
}

// rawLine is one decoded JSONL line. Claude Code writes many line types
// (attachment, file-history-snapshot, ai-title, cost-state, …); only user and
// assistant lines carrying a message become turns.
type rawLine struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	AITitle     string `json:"aiTitle"`
	RequestID   string `json:"requestId"`
	Message     *struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// usageKey is the identity under which a usage-bearing line is counted exactly
// once, or "" when the line carries no countable usage.
//
// This MUST match internal/usage/ingest.go's dedupKey. Claude Code writes one
// API response as several assistant lines sharing a message.id, each carrying a
// growing usage snapshot, so summing lines over-counts about 2x — measured
// 1.87x-2.29x across five real transcripts. Every line has its own uuid, so
// keying on uuid dedups nothing at all.
func (l rawLine) usageKey() string {
	if l.Type != "assistant" || l.Message == nil || l.Message.Usage == nil {
		return ""
	}
	if l.Message.ID != "" {
		return l.Message.ID + "|" + l.RequestID
	}
	return l.Message.Model + "|" + l.Timestamp
}

// usageTokens is the four-way tally on this line, zero when it carries none.
func (l rawLine) usageTokens() usage.Tokens {
	if l.Message == nil || l.Message.Usage == nil {
		return usage.Tokens{}
	}
	u := l.Message.Usage
	return usage.Tokens{
		Input:         u.InputTokens,
		Output:        u.OutputTokens,
		CacheCreation: u.CacheCreationInputTokens,
		CacheRead:     u.CacheReadInputTokens,
	}
}

// countsAsTurn is THE turn-enumeration rule, and the only place it is decided.
// ReadPage and IndexOfTurn both route through it, so resolving a search hit's
// UUID lands on the same turn the reader renders.
//
// A line is a turn when it decoded cleanly, is a user or assistant line, and
// carries a message. Meta and sidechain lines DO count — they are turns that
// the UI may choose to hide, and making enumeration depend on a UI filter would
// break every stored index the moment a toggle flipped. Malformed lines and
// lines too long to decode do not count, because they never became turns.
func (l rawLine) countsAsTurn() bool {
	return (l.Type == "user" || l.Type == "assistant") && l.Message != nil
}

// rawBlock is one entry of a message.content array.
type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// blocks decodes message.content, which is either a bare string (an older
// shape, still written for plain user prompts) or an array of typed blocks.
func (l rawLine) blocks() ([]Block, int) {
	if l.Message == nil || len(l.Message.Content) == 0 {
		return []Block{}, 0
	}
	var s string
	if err := json.Unmarshal(l.Message.Content, &s); err == nil {
		text, truncated := clip(s, textBytes)
		return []Block{{Kind: KindText, Text: text, FullBytes: len(s), Truncated: truncated}}, 0
	}
	var raws []rawBlock
	if err := json.Unmarshal(l.Message.Content, &raws); err != nil {
		return []Block{}, 0
	}
	out := make([]Block, 0, len(raws))
	unknown := 0
	for _, rb := range raws {
		b := Block{RawType: rb.Type}
		switch rb.Type {
		case "text":
			b.Kind = KindText
			b.Text, b.Truncated = clip(rb.Text, textBytes)
			b.FullBytes = len(rb.Text)
		case "thinking":
			b.Kind = KindThinking
			b.Text, b.Truncated = clip(rb.Thinking, textBytes)
			b.FullBytes = len(rb.Thinking)
		case "tool_use":
			b.Kind = KindToolUse
			b.ToolName, b.ToolUseID = rb.Name, rb.ID
			b.Preview, b.FullBytes, b.Truncated = toolInputPreview(rb.Input)
		case "tool_result":
			b.Kind = KindToolResult
			b.ToolUseID, b.IsError = rb.ToolUseID, rb.IsError
			b.Preview, b.FullBytes, b.Truncated = previewOf(rb.Content)
		case "image":
			// The payload is base64 and enormous; record its presence only.
			b.Kind = KindImage
		default:
			b.Kind = KindUnknown
			unknown++
		}
		if b.Kind != KindUnknown {
			b.RawType = ""
		}
		out = append(out, b)
	}
	return out, unknown
}

// primaryInputKeys are the tool-input fields worth showing as a one-line
// summary, in preference order. A Bash call is its command, an Edit is its
// path, a Grep is its pattern.
var primaryInputKeys = []string{
	"command", "file_path", "path", "pattern", "query", "url",
	"description", "prompt", "notebook_path",
}

// toolInputPreview renders a tool call's input as something a human can scan.
//
// The raw input is a JSON object, and showing it verbatim fills the chip with
// `{"language":"go","code":"package main\n\nimport (...` — technically the
// input, useless as a summary. Pulling the one field that identifies the call
// turns the chip into "Bash  grep -rn findCCPM", which is what the row is for.
// FullBytes stays the size of the whole input so the expanded view is honest.
func toolInputPreview(raw json.RawMessage) (preview string, full int, truncated bool) {
	if len(raw) == 0 {
		return "", 0, false
	}
	full = len(raw)
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, k := range primaryInputKeys {
			v, ok := obj[k]
			if !ok {
				continue
			}
			var s string
			if json.Unmarshal(v, &s) != nil || strings.TrimSpace(s) == "" {
				continue
			}
			p, t := clip(collapseWS(s), previewBytes)
			return p, full, t || len(obj) > 1
		}
	}
	p, t := clip(string(raw), previewBytes)
	return p, full, t
}

// previewOf renders a tool input or tool result to a short display string.
// tool_result content arrives either as a plain string or as an array of typed
// blocks; both are flattened to text here.
func previewOf(raw json.RawMessage) (preview string, full int, truncated bool) {
	if len(raw) == 0 {
		return "", 0, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		p, t := clip(s, previewBytes)
		return p, len(s), t
	}
	var blocks []rawBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text != "" {
				sb.WriteString(b.Text)
			}
		}
		joined := sb.String()
		p, t := clip(joined, previewBytes)
		return p, len(joined), t
	}
	// A tool input is a JSON object; show it as written.
	p, t := clip(string(raw), previewBytes)
	return p, len(raw), t
}

// clip truncates s to at most n bytes without splitting a rune.
func clip(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// ClipRunes truncates s to at most n runes. Used for titles, where a rune
// budget reads better than a byte budget.
func ClipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// eachLine streams path, handing every complete line to fn until fn returns
// false. It stops before a trailing line with no newline — that is a transcript
// still being written, and decoding half a JSON object would render a corrupt
// turn. This mirrors the rule internal/usage/sync.go applies for the same
// reason.
//
// A line longer than maxLineBytes is reported to fn as skipped rather than
// decoded, so one pathological line does not cost every turn after it.
//
// The early-stop return is what bounds search: once a file has produced its
// quota of hits there is nothing to gain from decoding the rest of it, and on a
// common query the rest is most of a 76 MB file.
func eachLine(path string, fn func(raw []byte, skipped bool) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// bufio.Scanner is not usable here: it yields a final line that has no
	// trailing newline, which is exactly the half-written line this must skip.
	// ReadBytes has the right semantics but would materialise a whole
	// newline-free file before any cap could be consulted, so the line is
	// assembled fragment by fragment and dropped once it passes maxLineBytes.
	r := bufio.NewReaderSize(f, 1<<20)
	var line []byte
	oversize := false
	for {
		frag, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			if oversize || len(line)+len(frag) > maxLineBytes {
				oversize = true // keep draining, stop accumulating
				line = line[:0]
			} else {
				line = append(line, frag...)
			}
			continue
		}
		if err != nil {
			// EOF with bytes pending is a line still being written: leave it.
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		// The cap must be re-checked here, not only in the ErrBufferFull branch:
		// the fragment that finally contains the newline arrives with err == nil,
		// so a line just over the limit would otherwise be assembled in full.
		if oversize || len(line)+len(frag) > maxLineBytes {
			oversize = false
			line = line[:0]
			if !fn(nil, true) {
				return nil
			}
			continue
		}
		var complete []byte
		if len(line) == 0 {
			complete = frag // fast path: whole line already contiguous
		} else {
			complete = append(line, frag...)
		}
		if !fn(complete, false) {
			return nil
		}
		line = line[:0]
	}
}

// ReadPage returns the turns in [offset, offset+limit) along with the file's
// total turn count. An offset past the end yields an empty (never nil) slice
// and no error, so the caller renders an empty view rather than crashing.
//
// The whole file is streamed on every call because Total is not knowable
// otherwise. At the measured throughput that is ~100-200 ms even for the
// largest transcript on disk, which is acceptable for a click; if paging ever
// feels slow, an offset index per file is the next step.
func ReadPage(path string, offset, limit int) (Page, error) {
	page := Page{Turns: []Turn{}}
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	idx := 0
	err := eachLine(path, func(raw []byte, skipped bool) bool {
		if skipped {
			page.SkippedLines++
			return true
		}
		var l rawLine
		if json.Unmarshal(raw, &l) != nil {
			return true // malformed line: not a turn, does not advance the index
		}
		if !l.countsAsTurn() {
			return true
		}
		i := idx
		idx++
		page.Total = idx
		if i < offset || i >= offset+limit {
			return true
		}
		blocks, unknown := l.blocks()
		page.UnknownBlocks += unknown
		page.Turns = append(page.Turns, Turn{
			Index:       i,
			UUID:        l.UUID,
			Role:        l.Message.Role,
			Timestamp:   l.Timestamp,
			Model:       l.Message.Model,
			IsSidechain: l.IsSidechain,
			IsMeta:      l.IsMeta,
			Blocks:      blocks,
		})
		return true
	})
	if err != nil {
		return Page{Turns: []Turn{}}, err
	}
	return page, nil
}

// searchTexts yields the full text of a turn's content for MATCHING, as opposed
// to blocks(), which truncates for display.
//
// Search must not run on display previews. A tool result clipped to 2 KB makes
// "include tool output" silently useless on the large outputs it exists for, and
// a tool input reduced to its one identifying field makes the code Claude wrote
// (an Edit's new_string, a Write's content) unfindable — while the docs promise
// both are searchable. Reading the raw blocks here costs nothing extra: this
// only runs on lines that already survived the byte prefilter.
func (l rawLine) searchTexts(includeToolResults bool) []searchable {
	if l.Message == nil || len(l.Message.Content) == 0 {
		return nil
	}
	var str string
	if json.Unmarshal(l.Message.Content, &str) == nil {
		return []searchable{{text: str, source: SourceText}}
	}
	var raws []rawBlock
	if json.Unmarshal(l.Message.Content, &raws) != nil {
		return nil
	}
	out := make([]searchable, 0, len(raws))
	for _, rb := range raws {
		switch rb.Type {
		case "text":
			out = append(out, searchable{text: rb.Text, source: SourceText})
		case "tool_use":
			// The whole input object, so every field is searchable — not just
			// the one toolInputPreview picks for the chip label.
			out = append(out, searchable{
				text:     rb.Name + " " + flatten(rb.Input),
				source:   SourceToolUse,
				toolName: rb.Name,
			})
		case "tool_result":
			if includeToolResults {
				out = append(out, searchable{text: flatten(rb.Content), source: SourceToolResult})
			}
		}
		// thinking is never searched: the reader hides it, so a hit there is one
		// the user cannot be shown.
	}
	return out
}

// ToolBody returns the full, untruncated payload of one block in one turn —
// what a reader fetches when the user expands a tool chip.
//
// maxBytes still bounds the result. The longest single line measured in a real
// profile was 1.3 MB, and handing that to a webview in one string locks it, so
// "full" means "the whole thing up to a cap the UI can render", with the true
// size reported alongside.
func ToolBody(path, turnUUID string, blockIndex, maxBytes int) (body string, full int, truncated bool, err error) {
	if maxBytes <= 0 {
		maxBytes = 256 << 10
	}
	found := false
	err = eachLine(path, func(raw []byte, skipped bool) bool {
		if skipped {
			return true
		}
		var l rawLine
		if json.Unmarshal(raw, &l) != nil {
			return true
		}
		if !l.countsAsTurn() || l.UUID != turnUUID {
			return true
		}
		found = true
		var raws []rawBlock
		if json.Unmarshal(l.Message.Content, &raws) != nil {
			return false
		}
		if blockIndex < 0 || blockIndex >= len(raws) {
			return false
		}
		rb := raws[blockIndex]
		payload := rb.Content
		if len(payload) == 0 {
			payload = rb.Input
		}
		text := flatten(payload)
		full = len(text)
		body, truncated = clip(text, maxBytes)
		return false
	})
	if err != nil {
		return "", 0, false, err
	}
	if !found {
		return "", 0, false, os.ErrNotExist
	}
	return body, full, truncated, nil
}

// flatten renders a tool payload to plain text, handling the string shape, the
// typed-block-array shape, and a raw JSON object input.
func flatten(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []rawBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			sb.WriteString(b.Text)
		}
		return sb.String()
	}
	return string(raw)
}

// IndexOfTurn returns the position of the turn with the given uuid, or -1 when
// the file has no such turn.
//
// This is how a search hit becomes a reader position. The search scan cannot
// produce a trustworthy index — its byte prefilter skips lines without decoding
// them, so it never learns whether a skipped line was a turn — so it hands back
// a UUID and this resolves it under the same enumeration rule ReadPage uses.
// One rule, one pass, no contract between two filters to drift.
func IndexOfTurn(path, uuid string) (int, error) {
	if uuid == "" {
		return -1, nil
	}
	idx, found := 0, -1
	err := eachLine(path, func(raw []byte, skipped bool) bool {
		if skipped {
			return true
		}
		var l rawLine
		if json.Unmarshal(raw, &l) != nil {
			return true
		}
		if !l.countsAsTurn() {
			return true
		}
		if l.UUID == uuid {
			found = idx
			return false // found it; no reason to read the rest
		}
		idx++
		return true
	})
	if err != nil {
		return -1, err
	}
	return found, nil
}

// PageAround returns the window of turns containing the turn with uuid, plus
// that turn's index so the caller can scroll to and flash it. An unknown uuid
// falls back to the first page rather than erroring — the transcript may have
// been rewritten since the search ran.
func PageAround(path, uuid string, limit int) (Page, int, error) {
	if limit <= 0 {
		limit = 200
	}
	at, err := IndexOfTurn(path, uuid)
	if err != nil {
		return Page{Turns: []Turn{}}, -1, err
	}
	offset := 0
	if at >= 0 {
		// Put the target a little way into the page so there is preceding
		// context to read rather than landing on the very first line.
		offset = max(at-limit/4, 0)
	}
	page, err := ReadPage(path, offset, limit)
	return page, at, err
}

// FirstUserPrompt pulls a human-readable preview out of one decoded transcript
// line. The shape varies across Claude Code versions, so a few known spots are
// probed: a v1 top-level "content" string, and v2's message.content as either a
// string or an array of typed blocks.
//
// This is the content-block decoding that cmd/sessions.go used to own. It lives
// here so there is exactly one implementation to keep current when the format
// moves.
func FirstUserPrompt(entry map[string]any) string {
	if role, _ := entry["role"].(string); role != "user" && entry["role"] != nil {
		return ""
	}
	if s, ok := entry["content"].(string); ok {
		return strings.TrimSpace(s)
	}
	msg, ok := entry["message"].(map[string]any)
	if !ok {
		return ""
	}
	if role, _ := msg["role"].(string); role != "user" && msg["role"] != nil {
		return ""
	}
	switch content := msg["content"].(type) {
	case string:
		return strings.TrimSpace(content)
	case []any:
		for _, blk := range content {
			bm, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "text" {
				continue
			}
			if text, ok := bm["text"].(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// isTitleWorthy rejects the prompts that make a useless session title: the
// slash-command envelopes Claude Code writes as ordinary user lines
// ("<command-name>/model</command-name>…") and its own stdout echoes. Without
// this the first "real" prompt in a large share of transcripts is "/model".
func isTitleWorthy(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	return !strings.HasPrefix(t, "<command-name>") &&
		!strings.HasPrefix(t, "<local-command-stdout>") &&
		!strings.HasPrefix(t, "<command-message>")
}
