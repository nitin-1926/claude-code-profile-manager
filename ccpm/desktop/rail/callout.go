//go:build darwin

package rail

import (
	"fmt"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
)

// Callout is the detail panel shown while a ring is hovered: who this profile
// is and how fresh the reading is, then one block per limit window.
type Callout struct {
	Profile string `json:"profile"`
	// Account and Plan identify which login this ring is actually about — the
	// whole point of per-profile limits is that they are different accounts.
	Account string `json:"account"`
	Plan    string `json:"plan"`
	// Freshness is always shown. A number with no age on it invites the reader
	// to assume it is live, and it very often is not.
	Freshness string `json:"freshness"`
	Stale     bool   `json:"stale"`
	// Explanation replaces the windows when there is no reading, saying why in
	// words rather than leaving an unexplained blank.
	Explanation string          `json:"explanation"`
	Windows     []CalloutWindow `json:"windows"`
}

// CalloutWindow is one limit window's block: label, reset time, bar, percentage.
type CalloutWindow struct {
	Label    string  `json:"label"`
	Reset    string  `json:"reset"`
	Percent  string  `json:"percent"`
	Fraction float64 `json:"fraction"`
	RGB      RGB     `json:"rgb"`
}

// StaleAfter is when a cached reading stops being presented as current.
//
// The cache only refreshes when that profile's statusline renders, so a profile
// nobody has used today legitimately has an old reading. Half an hour is short
// enough that a stale label means something and long enough not to cry wolf
// during an ordinary session.
const StaleAfter = 30 * time.Minute

// relativeResetCutoff is how far out a reset switches from a relative form
// ("Resets in 51 min") to an absolute one ("Resets Thu 12:00 AM"). Past half a
// day "in 19 hr" stops being something a person can act on, and a weekday and
// clock time is what they would actually plan around.
const relativeResetCutoff = 12 * time.Hour

// BuildCallout renders one profile's detail panel.
func BuildCallout(l services.ProfileLimits, now time.Time) Callout {
	c := Callout{
		Profile:   l.Profile,
		Account:   l.Account,
		Plan:      l.Plan,
		Freshness: FormatFreshness(l.CapturedAt, now),
	}
	if l.CapturedAt > 0 {
		c.Stale = now.Sub(time.Unix(l.CapturedAt, 0)) >= StaleAfter
	}
	if !l.Available {
		c.Explanation = ExplainUnavailable(l.Reason)
		return c
	}
	for _, w := range l.Windows {
		c.Windows = append(c.Windows, CalloutWindow{
			Label:    w.Label,
			Reset:    FormatReset(w.ResetsAt, now),
			Percent:  fmt.Sprintf("%d%% used", int(w.UsedPercentage+0.5)),
			Fraction: FillFraction(w.UsedPercentage),
			RGB:      NotchColor(int(100 - w.UsedPercentage)),
		})
	}
	return c
}

// FormatReset renders when a window rolls over: relative while that is
// actionable, absolute once it is not.
func FormatReset(resetsAt int64, now time.Time) string {
	if resetsAt <= 0 {
		return "Reset time unknown"
	}
	// time.Unix returns a local-zone time; render it in the caller's zone so
	// the weekday and clock read the way the user's own clock does (and so a
	// test can pin them).
	at := time.Unix(resetsAt, 0).In(now.Location())
	d := at.Sub(now)

	// A reset in the past is a window that has already rolled over. Never
	// render a negative duration — "Resets in -8 min" is nonsense on screen.
	if d <= 0 {
		return "Reset — refreshing"
	}
	if d >= relativeResetCutoff {
		return "Resets " + at.Format("Mon 3:04 PM")
	}
	if d < time.Minute {
		return "Resets in under a minute"
	}
	if d < time.Hour {
		return fmt.Sprintf("Resets in %d min", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	if m == 0 {
		return fmt.Sprintf("Resets in %d hr", h)
	}
	return fmt.Sprintf("Resets in %d hr %d min", h, m)
}

// FormatFreshness renders how old a reading is.
//
// The cache is only written when a profile's statusline renders, so "how old"
// is load-bearing rather than decorative: without it a reading from Tuesday
// looks exactly like one from a second ago.
func FormatFreshness(capturedAt int64, now time.Time) string {
	if capturedAt <= 0 {
		return "never updated"
	}
	d := now.Sub(time.Unix(capturedAt, 0))
	// Clock skew, or a cache written by a machine a moment ahead of this one.
	// Reporting "updated in 3 min" would be worse than rounding to now.
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "updated just now"
	case d < time.Hour:
		return fmt.Sprintf("updated %d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("updated %d hr ago", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	if days == 1 {
		return "updated yesterday"
	}
	return fmt.Sprintf("updated %d days ago", days)
}

// ExplainUnavailable turns a machine-readable reason into something that tells
// the reader what to do about it, rather than leaving the panel blank.
func ExplainUnavailable(reason string) string {
	switch reason {
	case services.ReasonNotSubscribed:
		return "Claude does not report usage limits for this plan."
	case services.ReasonNoData:
		return "No reading yet — run Claude Code on this profile once."
	default:
		return "No usage data available for this profile."
	}
}
