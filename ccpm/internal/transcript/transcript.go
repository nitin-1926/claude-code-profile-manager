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
)

const (
	// maxLineBytes is the largest JSONL line we will decode. The longest line
	// measured in a real profile was 1.3 MB (a single tool result), so this is
	// generous headroom; a line beyond it is counted and skipped rather than
	// aborting the file, because one pathological line must not cost the reader
	// every turn after it.
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
	Message     *struct {
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
			b.Preview, b.FullBytes, b.Truncated = previewOf(rb.Input)
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

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // trailing partial line — leave it for next time
			}
			return err
		}
		if len(line) > maxLineBytes {
			if !fn(nil, true) {
				return nil
			}
			continue
		}
		if !fn(line, false) {
			return nil
		}
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
