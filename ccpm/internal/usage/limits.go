package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// Subscription rate-limit windows, cached per profile.
//
// These do NOT come from the transcripts this package otherwise reads —
// transcripts carry token counts and nothing else (no quota, no reset clock).
// They come from the JSON Claude Code pipes to a statusLine command on every
// render, under `rate_limits`. `ccpm statusline` already parses that block to
// draw its window segments; LimitsFromStatusLine lets it also persist what it
// saw, so the desktop app can show real limits without a session running.
//
// Consequence worth stating plainly: this cache is only as fresh as the last
// Claude Code render for the profile. Callers must surface CapturedAt rather
// than implying the numbers are live. And because Claude Code only sends
// `rate_limits` for Claude.ai Pro/Max accounts, a profile can legitimately have
// no windows at all — see Limits.Available.
const limitsVersion = 1

// limitsPath is the sibling of state.json / sessions.json / daily.json.
func limitsPath(profileDir string) string { return filepath.Join(Dir(profileDir), "limits.json") }

// LimitWindow is one rate-limit window (a 5-hour session window, a 7-day
// window) as reported by Claude Code.
type LimitWindow struct {
	// Key is Claude Code's own identifier: "five_hour", "seven_day".
	Key string `json:"key"`
	// Label is the human name for the window, matching Claude Code's wording.
	Label string `json:"label"`
	// UsedPercentage is 0-100. It can exceed 100 on overage plans.
	UsedPercentage float64 `json:"used_percentage"`
	// ResetsAt is a Unix timestamp (seconds). Zero means Claude Code did not
	// report one, which is not the same as "resets now".
	ResetsAt int64 `json:"resets_at"`
}

// Limits is the per-profile cache of the most recent rate-limit reading.
type Limits struct {
	Version int `json:"version"`
	// CapturedAt is when we last saw a reading, in Unix seconds. This is the
	// freshness the UI must show; the numbers age the moment they are written.
	CapturedAt int64 `json:"captured_at"`
	// Source records where the reading came from, so a future second source
	// (a live API call, say) is distinguishable from this one.
	Source  string        `json:"source"`
	Windows []LimitWindow `json:"windows"`
}

// Available reports whether this cache holds a usable reading. An empty
// Windows slice is a meaningful state — a Claude.ai account that has not yet
// made a request this session, or an API-key profile that never reports limits
// at all — and must render as "no data", never as 0% used.
func (l Limits) Available() bool { return len(l.Windows) > 0 && l.CapturedAt > 0 }

// Age reports how stale the reading is. Zero CapturedAt yields a zero
// duration; callers should gate on Available first.
func (l Limits) Age(now time.Time) time.Duration {
	if l.CapturedAt <= 0 {
		return 0
	}
	d := now.Sub(time.Unix(l.CapturedAt, 0))
	if d < 0 {
		// Clock skew, or a reading written by a machine slightly ahead. "Just
		// now" beats rendering a negative age.
		return 0
	}
	return d
}

// LoadLimits reads a profile's cached windows. A missing or unreadable file is
// not an error — it is the ordinary state for a profile Claude Code has never
// run in, and every caller would otherwise have to special-case it.
func LoadLimits(profileDir string) Limits {
	var l Limits
	data, err := os.ReadFile(limitsPath(profileDir))
	if err != nil {
		return Limits{}
	}
	if err := json.Unmarshal(data, &l); err != nil {
		// A truncated or corrupt cache is indistinguishable from no cache for
		// our purposes, and losing a usage reading is not worth an error path.
		return Limits{}
	}
	if l.Version != limitsVersion {
		return Limits{}
	}
	return l
}

// SaveLimits writes a profile's windows atomically.
//
// Deliberately refuses to persist an empty reading: Claude Code omits
// `rate_limits` for API-key sessions, so a single such session in a profile
// that also has a Claude.ai account would otherwise wipe good data and blank
// the ring. Nothing to record means nothing to write.
func SaveLimits(profileDir string, l Limits) error {
	if len(l.Windows) == 0 {
		return nil
	}
	l.Version = limitsVersion
	if l.CapturedAt == 0 {
		l.CapturedAt = time.Now().Unix()
	}
	if l.Source == "" {
		l.Source = SourceStatusLine
	}
	if err := os.MkdirAll(Dir(profileDir), config.DirPerm); err != nil {
		return err
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(limitsPath(profileDir), data, config.FilePerm),
	})
}

// SourceStatusLine marks a reading borrowed from Claude Code's statusLine
// payload — the only source today.
const SourceStatusLine = "statusline"

// Window labels mirror Claude Code's own vocabulary so the rail, the status
// line, and the TUI never disagree about what a window is called.
const (
	KeyFiveHour = "five_hour"
	KeySevenDay = "seven_day"

	LabelFiveHour = "Current session"
	LabelSevenDay = "All models"
)

// LabelFor maps a window key to its display label, falling back to the raw key
// so a window Claude Code adds later still renders something truthful rather
// than being silently dropped.
func LabelFor(key string) string {
	switch key {
	case KeyFiveHour:
		return LabelFiveHour
	case KeySevenDay:
		return LabelSevenDay
	default:
		return key
	}
}
