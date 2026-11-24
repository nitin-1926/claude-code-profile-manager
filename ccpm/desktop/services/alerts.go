//go:build darwin

package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/transcript"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

type alertKind int

const (
	alertThreshold alertKind = iota
	alertAtLimit
	alertRenewed
)

// resetJitter is how far resets_at may wander and still be the same window. Real
// windows are hours apart; this machine has no reading to measure the upstream
// value's stability on, and treating a one-second wobble as a new window would
// re-fire the threshold alert on every status line write.
const resetJitter = 60 // seconds

// windowState is what one window of one profile last looked like.
type windowState struct {
	above    bool  // at or past the threshold, so that crossing has been counted
	atLimit  bool  // at or past 100, likewise
	resetsAt int64 // last reported reset; 0 until one is reported
}

// step moves the state on by one reading and returns the alerts it earned. It
// always runs, even when nothing will be delivered, so that turning alerts on or
// restarting at 85% starts from the truth rather than from a pending backlog.
func (st *windowState) step(w usage.LimitWindow, threshold float64, now time.Time) []alertKind {
	var fired []alertKind

	// resets_at of 0 means unreported, so going to or from it is not a move.
	moved := st.resetsAt != 0 && w.ResetsAt != 0 && max(st.resetsAt-w.ResetsAt, w.ResetsAt-st.resetsAt) > resetJitter
	if moved || w.Expired(now) {
		// Only a window that was being warned about is worth saying renewed.
		// Clearing above here is also what stops the clock-driven renewal firing
		// again when the next write brings the new resets_at.
		if st.above {
			fired = append(fired, alertRenewed)
		}
		st.above, st.atLimit = false, false
	}

	used := w.UsedAt(now)
	if used < threshold {
		st.above = false
	}
	if used < 100 {
		st.atLimit = false
	}
	switch {
	case used >= 100 && !st.atLimit:
		// A jump straight past the threshold says the stronger thing once.
		fired = append(fired, alertAtLimit)
		st.above, st.atLimit = true, true
	case used >= threshold && !st.above:
		fired = append(fired, alertThreshold)
		st.above = true
	}

	if w.ResetsAt != 0 {
		st.resetsAt = w.ResetsAt
	}
	return fired
}

// Alerts turns changes in the profiles' cached files into notifications.
//
// All its state is in memory and the whole path only reads: it runs off the
// watcher, so anything it wrote under ~/.ccpm would wake it again. A restart
// therefore forgets everything, which the first-sight seeding below makes safe.
type Alerts struct {
	notify func(title, body string)

	mu        sync.Mutex
	threshold int
	seen      map[string]bool // profiles read at least once
	windows   map[string]*windowState
	ended     map[string]usage.SessionEnd
}

// NewAlerts delivers through notify, which must not block for long and must
// swallow its own failures: a denied notification is not an error here.
func NewAlerts(notify func(title, body string)) *Alerts {
	return &Alerts{
		notify:  notify,
		seen:    map[string]bool{},
		windows: map[string]*windowState{},
		ended:   map[string]usage.SessionEnd{},
	}
}

// Check reads every profile's limits and session marker and notifies about what
// changed since the last call. It is called on file changes and on a slow tick,
// since a window resetting writes no file, from several goroutines.
func (a *Alerts) Check(prefs DesktopPrefs, now time.Time) {
	cfg, err := config.Load()
	if err != nil {
		return
	}

	type notice struct{ title, body string }
	var notices []notice
	type endedSession struct {
		profile, dir string
		end          usage.SessionEnd
	}
	var ended []endedSession

	a.mu.Lock()
	// A settings change is not a crossing, so a new threshold re-seeds silently.
	thresholdChanged := a.threshold != prefs.LimitAlertThreshold
	a.threshold = prefs.LimitAlertThreshold

	for name, pc := range cfg.Profiles {
		first := !a.seen[name]
		a.seen[name] = true

		// Unavailable leaves the state alone: losing a reading is not a change.
		for _, w := range usage.LoadLimits(pc.Dir).Windows {
			if w.Key != usage.KeyFiveHour && w.Key != usage.KeySevenDay {
				continue
			}
			key := name + "/" + w.Key
			st := a.windows[key]
			if st == nil {
				st = &windowState{}
				a.windows[key] = st
			}
			for _, kind := range st.step(w, float64(prefs.LimitAlertThreshold), now) {
				if prefs.LimitAlerts && !first && !thresholdChanged {
					title, body := limitMessage(name, w, kind, now)
					notices = append(notices, notice{title, body})
				}
			}
		}

		end := usage.LoadSessionEnd(pc.Dir)
		if end != a.ended[name] {
			a.ended[name] = end
			if prefs.NotifySessionEnd && !first && end.SessionID != "" {
				ended = append(ended, endedSession{name, pc.Dir, end})
			}
		}
	}
	a.mu.Unlock()

	for _, n := range notices {
		a.notify(n.title, n.body)
	}
	// After the lock: this reads a whole transcript.
	for _, e := range ended {
		a.notify("Session ended", sessionEndMessage(e.profile, e.dir, e.end))
	}
}

func limitMessage(profile string, w usage.LimitWindow, kind alertKind, now time.Time) (title, body string) {
	name := usage.WindowName(w.Key)
	renews := ""
	if at := usage.FormatRenewal(w.ResetsAt, now); at != "" {
		renews = ", renews " + at
	}
	switch kind {
	case alertRenewed:
		return "Limit renewed", fmt.Sprintf("%s's %s limit has renewed", profile, name)
	case alertAtLimit:
		return "Limit reached", fmt.Sprintf("%s has reached its %s limit%s", profile, name, renews)
	default:
		// Floored so a reading that fired at 80 can never display below it, and
		// 99.6 does not claim 100.
		return "Limit warning", fmt.Sprintf("%s is at %d%% of its %s limit%s", profile, int(w.UsedPercentage), name, renews)
	}
}

// sessionEndMessage still says the session ended when the figures are out of
// reach. Cost is an API-rate estimate, the same one the Usage tab shows: what a
// subscription really costs is unrelated.
func sessionEndMessage(profile, dir string, end usage.SessionEnd) string {
	body := profile + " session ended"
	e, err := transcript.SessionUsage(dir, end.TranscriptPath)
	if err != nil {
		return body
	}
	tokens := e.Tokens().Total()
	if tokens == 0 {
		return body
	}
	body += ": " + usage.HumanTokens(tokens) + " tokens"
	if cost := e.Cost(); cost >= 0.01 {
		body += fmt.Sprintf(", about $%.2f at API rates", cost)
	}
	return body
}
