//go:build darwin

package services

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// alertRig is a registered profile, a clock, and a fake delivery function, so
// every step below goes through the real limits.json and the real Check.
type alertRig struct {
	t     *testing.T
	name  string
	dir   string
	now   time.Time
	prefs DesktopPrefs
	a     *Alerts

	mu   sync.Mutex
	sent []string
}

func newAlertRig(t *testing.T) *alertRig {
	t.Helper()
	name := syntheticProfile(t)
	loc := time.FixedZone("test", 0)
	r := &alertRig{
		t:     t,
		name:  name,
		dir:   profileDir(name),
		now:   time.Date(2026, 11, 4, 12, 0, 0, 0, loc),
		prefs: DesktopPrefs{LimitAlerts: true, NotifySessionEnd: true, LimitAlertThreshold: 80},
	}
	r.a = NewAlerts(func(title, body string) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.sent = append(r.sent, body)
	})
	return r
}

// write stores a reading on the same disk shape the status line produces, the
// 7-day window parked well clear so only five_hour is under test unless given.
func (r *alertRig) write(fiveHour float64, resetsAt int64) {
	r.t.Helper()
	r.writeBoth(fiveHour, resetsAt, 1, r.now.Add(72*time.Hour).Unix())
}

func (r *alertRig) writeBoth(fiveHour float64, fiveAt int64, sevenDay float64, sevenAt int64) {
	r.t.Helper()
	if err := usage.SaveLimits(r.dir, usage.Limits{
		CapturedAt: r.now.Unix(),
		Windows: []usage.LimitWindow{
			{Key: usage.KeyFiveHour, Label: usage.LabelFiveHour, UsedPercentage: fiveHour, ResetsAt: fiveAt},
			{Key: usage.KeySevenDay, Label: usage.LabelSevenDay, UsedPercentage: sevenDay, ResetsAt: sevenAt},
		},
	}); err != nil {
		r.t.Fatal(err)
	}
}

// expect runs one check and asserts exactly these notifications came out of it.
func (r *alertRig) expect(step string, want ...string) {
	r.t.Helper()
	r.a.Check(r.prefs, r.now)
	r.mu.Lock()
	got := r.sent
	r.sent = nil
	r.mu.Unlock()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		r.t.Errorf("%s:\n got %q\nwant %q", step, got, want)
	}
}

func TestLimitAlertLifecycle(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(3*time.Hour + 45*time.Minute).Unix() // 15:45

	r.write(50, resets)
	r.expect("seed below threshold")

	r.write(85, resets)
	r.expect("crossing", "synthetic is at 85% of its 5-hour limit, renews 15:45")

	r.write(86, resets)
	r.expect("still above stays quiet")

	r.write(100, resets)
	r.expect("reaching the limit", "synthetic has reached its 5-hour limit, renews 15:45")

	r.write(104, resets)
	r.expect("overage is not a second crossing")

	r.write(70, resets)
	r.expect("dropping below re-arms silently")

	r.write(82, resets)
	r.expect("re-armed crossing fires again", "synthetic is at 82% of its 5-hour limit, renews 15:45")

	r.write(100, resets)
	r.expect("and so does the limit", "synthetic has reached its 5-hour limit, renews 15:45")
}

// A jump past the threshold in one reading says the stronger thing, once.
func TestLimitAlertJumpStraightToTheLimit(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()
	r.write(50, resets)
	r.expect("seed")
	r.write(100, resets)
	r.expect("jump", "synthetic has reached its 5-hour limit, renews 13:00")
	r.write(99, resets)
	r.expect("99 after 100 has already been warned about")
}

// Windows reset with no file write, so only the clock can notice.
func TestLimitAlertRenewalByClock(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()

	r.write(50, resets)
	r.expect("seed")
	r.write(90, resets)
	r.expect("crossing", "synthetic is at 90% of its 5-hour limit, renews 13:00")

	r.now = r.now.Add(time.Hour)
	r.expect("clock passes the reset", "synthetic's 5-hour limit has renewed")
	r.now = r.now.Add(time.Minute)
	r.expect("next tick stays quiet")

	// The next status line write brings the new window. That is not a second
	// renewal, and a low reading in it is nothing to say.
	r.write(4, r.now.Add(5*time.Hour).Unix())
	r.expect("new window arriving by file")
}

func TestLimitAlertRenewalByMovedReset(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()

	r.write(50, resets)
	r.expect("seed")
	r.write(90, resets)
	r.expect("crossing", "synthetic is at 90% of its 5-hour limit, renews 13:00")

	r.write(90, resets+1)
	r.expect("a one-second wobble is the same window")

	r.write(5, resets+5*3600)
	r.expect("reset moved", "synthetic's 5-hour limit has renewed")

	r.write(5, resets+10*3600)
	r.expect("moved again with nothing being warned about")
}

func TestLimitAlertStartupSeedsSilently(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()

	r.write(85, resets)
	r.expect("restart at 85% does not re-fire")
	r.write(86, resets)
	r.expect("nor does the next reading")

	// But it is armed: the reset that follows is worth saying.
	r.now = r.now.Add(time.Hour)
	r.expect("renewal after a silent seed", "synthetic's 5-hour limit has renewed")
}

// First sight is per profile, not per app run: one added or renamed while the app
// is up may arrive already at 85%, and that is not a crossing either.
func TestLimitAlertProfileAddedLaterSeedsSilently(t *testing.T) {
	r := newAlertRig(t)
	r.expect("first check")

	// "synthetic" is seen and has no reading, so its first reading is a real one.
	resets := r.now.Add(time.Hour).Unix()
	r.write(85, resets)
	r.expect("first reading of a known profile", "synthetic is at 85% of its 5-hour limit, renews 13:00")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	cfg.AddProfile("other", other, "oauth")
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := usage.SaveLimits(other, usage.Limits{
		CapturedAt: r.now.Unix(),
		Windows:    []usage.LimitWindow{{Key: usage.KeyFiveHour, UsedPercentage: 85, ResetsAt: resets}},
	}); err != nil {
		t.Fatal(err)
	}
	r.expect("a profile added while the app runs")
}

func TestLimitAlertStaleReadingAtStartupIsSilent(t *testing.T) {
	r := newAlertRig(t)
	r.write(95, r.now.Add(-time.Hour).Unix())
	r.expect("95% whose reset already passed")
	r.expect("and stays quiet")
}

// resets_at 0 means unreported: no clock to check, no renewal clause, and it
// must not read as "resets now" and swallow the alert.
func TestLimitAlertWithoutResetClock(t *testing.T) {
	r := newAlertRig(t)
	r.write(50, 0)
	r.expect("seed")
	r.write(85, 0)
	r.expect("crossing with no clock", "synthetic is at 85% of its 5-hour limit")
	r.now = r.now.Add(24 * time.Hour)
	r.expect("no clock, no renewal")
}

func TestLimitAlertWindowsAreIndependent(t *testing.T) {
	r := newAlertRig(t)
	fiveAt, sevenAt := r.now.Add(time.Hour).Unix(), r.now.Add(72*time.Hour).Unix()

	r.writeBoth(10, fiveAt, 10, sevenAt)
	r.expect("seed")
	r.writeBoth(10, fiveAt, 85, sevenAt)
	r.expect("7-day alone", "synthetic is at 85% of its 7-day limit, renews Sat 12:00")
	r.writeBoth(85, fiveAt, 85, sevenAt)
	r.expect("5-hour alone", "synthetic is at 85% of its 5-hour limit, renews 13:00")
}

func TestLimitAlertOffStaysQuietAndTurningOnIsNotACrossing(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()
	r.prefs.LimitAlerts = false

	r.write(50, resets)
	r.expect("seed while off")
	r.write(90, resets)
	r.expect("a crossing while off says nothing")

	r.prefs.LimitAlerts = true
	r.expect("turning on at 90% is not a crossing")
}

func TestLimitAlertThresholdChangeIsNotACrossing(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()

	r.write(60, resets)
	r.expect("seed")

	r.prefs.LimitAlertThreshold = 50
	r.expect("lowering past the current reading")
	r.write(61, resets)
	r.expect("and it counts as already crossed")

	r.prefs.LimitAlertThreshold = 70
	r.expect("raising it re-arms")
	r.write(72, resets)
	r.expect("so the new level fires", "synthetic is at 72% of its 5-hour limit, renews 13:00")
}

// A reading a profile no longer has must not look like a fall to zero.
func TestLimitAlertLostReadingKeepsState(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()
	r.write(85, resets)
	r.expect("seed")

	if err := os.Remove(filepath.Join(usage.Dir(r.dir), "limits.json")); err != nil {
		t.Fatal(err)
	}
	r.expect("gone")
	r.write(85, resets)
	r.expect("back at the same level is not a crossing")
}

// writeTranscript writes a session in the shape real transcripts have.
func writeTranscript(t *testing.T, dir, session string, in, out int64) string {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type": "assistant", "uuid": "a1", "requestId": "req_1",
		"sessionId": session, "cwd": "/repo", "timestamp": "2026-06-27T10:00:00Z",
		"message": map[string]any{
			"id": "msg_1", "role": "assistant", "model": "claude-opus-4-8",
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": in, "output_tokens": out},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, "projects", usage.EncodeCwd("/repo"))
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, session+".jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (r *alertRig) endSession(session, transcript, reason string) {
	r.t.Helper()
	payload := fmt.Sprintf(`{"session_id":%q,"transcript_path":%q,"hook_event_name":"SessionEnd","reason":%q}`, session, transcript, reason)
	if err := usage.MarkSessionEnd(r.dir, []byte(payload), r.now); err != nil {
		r.t.Fatal(err)
	}
}

func TestSessionEndNotification(t *testing.T) {
	r := newAlertRig(t)

	r.endSession("before", writeTranscript(t, r.dir, "before", 10, 10), "other")
	r.expect("a marker from before launch is not news")

	// Opus at $5/$25 per million: 200k in, 40k out is $1 + $1.
	r.now = r.now.Add(time.Minute)
	r.endSession("s1", writeTranscript(t, r.dir, "s1", 200_000, 40_000), "prompt_input_exit")
	r.expect("session end", "synthetic session ended: 240.0K tokens, about $2.00 at API rates")
	r.expect("seen once")

	// A figure we cannot reach must not cost the notification itself.
	r.now = r.now.Add(time.Minute)
	r.endSession("s2", "/etc/passwd", "other")
	r.expect("transcript outside the profile", "synthetic session ended")

	// /clear retires a session id but nothing has finished.
	r.now = r.now.Add(time.Minute)
	r.endSession("s3", writeTranscript(t, r.dir, "s3", 10, 10), "clear")
	r.expect("clear is not an ending")

	r.now = r.now.Add(time.Minute)
	r.prefs.NotifySessionEnd = false
	r.endSession("s4", writeTranscript(t, r.dir, "s4", 10, 10), "other")
	r.expect("switched off")
}

// The alert path runs off the watcher, so a single write under ~/.ccpm from it
// would wake it again, forever. It must also not take the index-building path,
// which writes history.json.
func TestAlertsWriteNothing(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()
	r.write(50, resets)
	r.expect("seed")

	r.write(90, resets)
	r.endSession("s1", writeTranscript(t, r.dir, "s1", 200_000, 40_000), "other")

	snapshot := func() []string {
		var out []string
		base := filepath.Join(os.Getenv("HOME"), ".ccpm")
		if err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("%s %d %d", p, fi.Size(), fi.ModTime().UnixNano()))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sort.Strings(out)
		return out
	}

	before := snapshot()
	r.a.Check(r.prefs, r.now)
	r.mu.Lock()
	sent := len(r.sent)
	r.mu.Unlock()
	if sent != 2 {
		t.Fatalf("the check under test delivered %d notifications, want 2", sent)
	}
	if after := snapshot(); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("alert path wrote to disk:\nbefore %v\nafter  %v", before, after)
	}
}

// Check is called from the watcher's timer, the minute tick, and a preference
// write, all on their own goroutines. Run with -race.
func TestAlertsCheckConcurrently(t *testing.T) {
	r := newAlertRig(t)
	resets := r.now.Add(time.Hour).Unix()
	r.write(50, resets)
	r.a.Check(r.prefs, r.now)
	r.write(90, resets)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.a.Check(r.prefs, r.now)
		}()
	}
	wg.Wait()
	if len(r.sent) != 1 {
		t.Errorf("one crossing delivered %d times: %q", len(r.sent), r.sent)
	}
}
