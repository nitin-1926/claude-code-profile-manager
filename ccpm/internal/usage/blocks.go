package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"time"
)

// blockWindow is Claude's 5-hour billing/rate-limit window.
const blockWindow = 5 * time.Hour

// entry is one deduped usage event with its own timestamp — the grain blocks
// need but the persistent daily/session store doesn't keep. Collected on demand.
type entry struct {
	ts     time.Time
	model  string
	tokens Tokens
}

// Block is one 5-hour usage window (ccusage-style). A window starts at the
// hour-floor of its first entry and spans 5h; a gap of >=5h since the last entry
// also starts a new one. Burn/projection fields are set only on the active block.
type Block struct {
	Start        string  `json:"start"`        // RFC3339, hour-floored window start
	End          string  `json:"end"`          // RFC3339, Start + 5h
	LastActivity string  `json:"lastActivity"` // RFC3339 of the last entry
	Tokens       Tokens  `json:"tokens"`
	Total        int64   `json:"total"`
	Cost         float64 `json:"cost"`
	Messages     int64   `json:"messages"`
	IsActive     bool    `json:"isActive"`

	// active-block only
	BurnTokensPerMin float64 `json:"burnTokensPerMin"`
	CostPerHour      float64 `json:"costPerHour"`
	RemainingMinutes int64   `json:"remainingMinutes"`
	ProjectedTotal   int64   `json:"projectedTotal"`
	ProjectedCost    float64 `json:"projectedCost"`
}

// LoadBlocks reads a profile's transcripts, dedups usage entries, and groups
// them into 5-hour blocks (newest last). The final block is marked active when
// its window still contains now and had activity within the window.
func LoadBlocks(profileDir string, now time.Time) ([]Block, error) {
	entries, err := collectEntries(profileDir)
	if err != nil {
		return nil, err
	}
	return groupBlocks(entries, now), nil
}

// groupBlocks is the pure 5-hour windowing + active-block enrichment, split out
// so it can be tested with synthetic entries.
func groupBlocks(entries []entry, now time.Time) []Block {
	if len(entries) == 0 {
		return []Block{}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ts.Before(entries[j].ts) })

	var blocks []Block
	var acc *blockAccum
	flush := func() {
		if acc != nil {
			blocks = append(blocks, acc.finish())
		}
	}
	for _, e := range entries {
		if acc == nil {
			acc = newBlockAccum(e.ts)
		} else if e.ts.Sub(acc.start) >= blockWindow || e.ts.Sub(acc.lastTS) >= blockWindow {
			flush()
			acc = newBlockAccum(e.ts)
		}
		acc.add(e)
	}
	flush()

	// Mark + enrich the active block (last one whose window still contains now
	// and whose last activity is within the window).
	if n := len(blocks); n > 0 {
		b := &blocks[n-1]
		start, _ := time.Parse(time.RFC3339, b.Start)
		last, _ := time.Parse(time.RFC3339, b.LastActivity)
		windowEnd := start.Add(blockWindow)
		if now.Before(windowEnd) && now.Sub(last) < blockWindow {
			b.IsActive = true
			elapsed := now.Sub(start).Minutes()
			if elapsed < 1 {
				elapsed = 1
			}
			b.BurnTokensPerMin = float64(b.Total) / elapsed
			b.CostPerHour = b.Cost / elapsed * 60
			remaining := windowEnd.Sub(now).Minutes()
			if remaining < 0 {
				remaining = 0
			}
			b.RemainingMinutes = int64(remaining)
			b.ProjectedTotal = b.Total + int64(b.BurnTokensPerMin*remaining)
			b.ProjectedCost = b.Cost + b.CostPerHour/60*remaining
		}
	}
	return blocks
}

// ActiveBlock returns the active block from LoadBlocks, or nil if none is active.
func ActiveBlock(profileDir string, now time.Time) (*Block, error) {
	blocks, err := LoadBlocks(profileDir, now)
	if err != nil {
		return nil, err
	}
	if n := len(blocks); n > 0 && blocks[n-1].IsActive {
		b := blocks[n-1]
		return &b, nil
	}
	return nil, nil
}

type blockAccum struct {
	start  time.Time
	lastTS time.Time
	tokens Tokens
	cost   float64
	msgs   int64
}

func newBlockAccum(first time.Time) *blockAccum {
	return &blockAccum{start: first.UTC().Truncate(time.Hour)}
}

func (a *blockAccum) add(e entry) {
	a.tokens.Add(e.tokens)
	a.cost += CostFor(e.model, e.tokens)
	a.msgs++
	if e.ts.After(a.lastTS) {
		a.lastTS = e.ts
	}
}

func (a *blockAccum) finish() Block {
	return Block{
		Start:        a.start.Format(time.RFC3339),
		End:          a.start.Add(blockWindow).Format(time.RFC3339),
		LastActivity: a.lastTS.UTC().Format(time.RFC3339),
		Tokens:       a.tokens,
		Total:        a.tokens.Total(),
		Cost:         a.cost,
		Messages:     a.msgs,
	}
}

// collectEntries walks every transcript for a profile and returns the deduped
// usage entries (largest-token snapshot wins per key), each with its own
// timestamp. This is a full read (not incremental) — blocks are an on-demand
// view, so freshness matters more than the incremental store's speed.
func collectEntries(profileDir string) ([]entry, error) {
	counted := map[string]Tokens{}
	byKey := map[string]*entry{}

	err := WalkTranscripts(profileDir, "", func(abs, rel string) error {
		f, ferr := os.Open(abs)
		if ferr != nil {
			return nil // skip unreadable (e.g. mid-write)
		}
		defer f.Close()
		reader := bufio.NewReaderSize(f, 1024*1024)
		for {
			lineBytes, rerr := reader.ReadBytes('\n')
			if len(lineBytes) > 0 {
				var l transcriptLine
				if json.Unmarshal(lineBytes, &l) == nil {
					if key := l.dedupKey(); key != "" {
						tok := l.tokens()
						if prev, ok := counted[key]; !ok || tok.Total() > prev.Total() {
							counted[key] = tok
							ts, perr := time.Parse(time.RFC3339, l.Timestamp)
							if perr == nil {
								byKey[key] = &entry{ts: ts, model: l.Message.Model, tokens: tok}
							}
						}
					}
				}
			}
			if rerr != nil {
				if errors.Is(rerr, io.EOF) {
					break
				}
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(byKey))
	for _, e := range byKey {
		out = append(out, *e)
	}
	return out, nil
}
