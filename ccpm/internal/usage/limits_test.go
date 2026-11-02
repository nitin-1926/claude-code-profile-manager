package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleLimits(capturedAt int64) Limits {
	return Limits{
		CapturedAt: capturedAt,
		Source:     SourceStatusLine,
		Windows: []LimitWindow{
			{Key: KeyFiveHour, Label: LabelFiveHour, UsedPercentage: 73.4, ResetsAt: 1_800_000_000},
			{Key: KeySevenDay, Label: LabelSevenDay, UsedPercentage: 7, ResetsAt: 1_800_500_000},
		},
	}
}

func TestSaveLoadLimitsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleLimits(1_700_000_000)
	if err := SaveLimits(dir, want); err != nil {
		t.Fatalf("SaveLimits: %v", err)
	}

	got := LoadLimits(dir)
	if !got.Available() {
		t.Fatal("round-tripped limits report Available() == false")
	}
	if got.CapturedAt != want.CapturedAt || got.Source != SourceStatusLine {
		t.Errorf("captured_at/source lost: got %d/%q", got.CapturedAt, got.Source)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("want 2 windows, got %d", len(got.Windows))
	}
	// Percentages drive a ring the user reads as truth — they must survive the
	// JSON round trip exactly, not approximately.
	if got.Windows[0].UsedPercentage != 73.4 || got.Windows[0].ResetsAt != 1_800_000_000 {
		t.Errorf("five_hour window corrupted: %+v", got.Windows[0])
	}
	if got.Windows[1].Key != KeySevenDay || got.Windows[1].Label != LabelSevenDay {
		t.Errorf("seven_day window corrupted: %+v", got.Windows[1])
	}
}

// The core regression this cache must never hit: Claude Code omits rate_limits
// for API-key sessions, so an empty reading must leave an existing good reading
// alone. Persisting it would blank a Pro/Max profile's ring the first time the
// user ran an API-key session in it.
func TestSaveLimitsRefusesEmptyAndPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	good := sampleLimits(1_700_000_000)
	if err := SaveLimits(dir, good); err != nil {
		t.Fatalf("seed SaveLimits: %v", err)
	}

	if err := SaveLimits(dir, Limits{CapturedAt: 1_700_009_999}); err != nil {
		t.Fatalf("empty SaveLimits should be a no-op, got error: %v", err)
	}

	got := LoadLimits(dir)
	if len(got.Windows) != 2 || got.CapturedAt != good.CapturedAt {
		t.Fatalf("empty save clobbered the cached reading: %+v", got)
	}
}

func TestSaveLimitsEmptyWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	if err := SaveLimits(dir, Limits{}); err != nil {
		t.Fatalf("SaveLimits: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Dir(dir), "limits.json")); !os.IsNotExist(err) {
		t.Errorf("an empty reading created a file; want none (stat err: %v)", err)
	}
}

func TestLoadLimitsAbsentAndCorrupt(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		got := LoadLimits(t.TempDir())
		if got.Available() {
			t.Error("a profile with no cache reported Available() == true")
		}
		if got.Windows != nil {
			t.Errorf("want nil windows for an absent cache, got %+v", got.Windows)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(Dir(dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(Dir(dir), "limits.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		// A truncated cache must read as "no data", never panic and never
		// surface a half-parsed percentage.
		if got := LoadLimits(dir); got.Available() {
			t.Errorf("corrupt cache reported available: %+v", got)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		dir := t.TempDir()
		if err := SaveLimits(dir, sampleLimits(1_700_000_000)); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(Dir(dir), "limits.json"))
		if err != nil {
			t.Fatal(err)
		}
		bumped := []byte(string(raw[:len(`{"version":`)]) + "99" + string(raw[len(`{"version":1`):]))
		if err := os.WriteFile(filepath.Join(Dir(dir), "limits.json"), bumped, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := LoadLimits(dir); got.Available() {
			t.Errorf("a future schema version was accepted: %+v", got)
		}
	})
}

func TestLimitsAvailable(t *testing.T) {
	cases := []struct {
		name string
		in   Limits
		want bool
	}{
		{"windows and timestamp", sampleLimits(1_700_000_000), true},
		{"windows but no timestamp", Limits{Windows: sampleLimits(0).Windows}, false},
		{"timestamp but no windows", Limits{CapturedAt: 1_700_000_000}, false},
		{"zero value", Limits{}, false},
	}
	for _, c := range cases {
		if got := c.in.Available(); got != c.want {
			t.Errorf("%s: Available() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLimitsAge(t *testing.T) {
	now := time.Unix(1_700_003_600, 0)

	if got := (Limits{CapturedAt: 1_700_000_000}).Age(now); got != time.Hour {
		t.Errorf("Age = %v, want 1h", got)
	}
	if got := (Limits{}).Age(now); got != 0 {
		t.Errorf("Age of a zero value = %v, want 0", got)
	}
	// A reading stamped slightly in the future (clock skew between the writing
	// session and the reader) must read as "just now", not as a negative age
	// that would format as nonsense.
	if got := (Limits{CapturedAt: now.Unix() + 300}).Age(now); got != 0 {
		t.Errorf("future-stamped Age = %v, want 0", got)
	}
}

func TestLabelFor(t *testing.T) {
	if got := LabelFor(KeyFiveHour); got != LabelFiveHour {
		t.Errorf("five_hour label = %q", got)
	}
	if got := LabelFor(KeySevenDay); got != LabelSevenDay {
		t.Errorf("seven_day label = %q", got)
	}
	// A window Claude Code adds later must still render something truthful
	// rather than being dropped or labelled with another window's name.
	if got := LabelFor("seven_day_opus"); got != "seven_day_opus" {
		t.Errorf("unknown key = %q, want the raw key back", got)
	}
}

// The encoding trap: the stored scale is 0-100 and is never sniffed, a value
// over 100 is real, resets_at 0 is "unreported" rather than "resets now", and a
// reset that already passed means the figure describes a dead window.
func TestWindowNormalisation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name    string
		w       LimitWindow
		used    float64
		expired bool
	}{
		{"ordinary", LimitWindow{UsedPercentage: 82, ResetsAt: now.Unix() + 60}, 82, false},
		{"a fraction is a small percentage, not 80%", LimitWindow{UsedPercentage: 0.8, ResetsAt: now.Unix() + 60}, 0.8, false},
		{"overage", LimitWindow{UsedPercentage: 104, ResetsAt: now.Unix() + 60}, 104, false},
		{"unreported reset", LimitWindow{UsedPercentage: 82, ResetsAt: 0}, 82, false},
		{"reset is exactly now", LimitWindow{UsedPercentage: 82, ResetsAt: now.Unix()}, 0, true},
		{"stale reset", LimitWindow{UsedPercentage: 82, ResetsAt: now.Unix() - 3600}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.w.UsedAt(now); got != c.used {
				t.Errorf("UsedAt = %v, want %v", got, c.used)
			}
			if got := c.w.Expired(now); got != c.expired {
				t.Errorf("Expired = %v, want %v", got, c.expired)
			}
		})
	}
}

func TestOverview(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	future := now.Unix() + 3600

	t.Run("no reading is both windows unknown", func(t *testing.T) {
		o := Limits{}.Overview(now)
		if len(o.Windows) != 2 || o.Windows[0].Known || o.Windows[1].Known || o.Closest != "" {
			t.Errorf("got %+v", o)
		}
	})
	t.Run("closest is the higher window", func(t *testing.T) {
		o := Limits{Windows: []LimitWindow{
			{Key: KeyFiveHour, UsedPercentage: 40, ResetsAt: future},
			{Key: KeySevenDay, UsedPercentage: 90, ResetsAt: future},
		}}.Overview(now)
		if o.Closest != KeySevenDay {
			t.Errorf("Closest = %q", o.Closest)
		}
	})
	t.Run("an expired window is unknown and cannot be closest", func(t *testing.T) {
		o := Limits{Windows: []LimitWindow{
			{Key: KeyFiveHour, UsedPercentage: 99, ResetsAt: now.Unix() - 1},
			{Key: KeySevenDay, UsedPercentage: 10, ResetsAt: future},
		}}.Overview(now)
		if o.Windows[0].Known || o.Closest != KeySevenDay {
			t.Errorf("got %+v", o)
		}
	})
	t.Run("a window Claude Code did not report is still listed", func(t *testing.T) {
		o := Limits{Windows: []LimitWindow{{Key: KeySevenDay, UsedPercentage: 10, ResetsAt: future}}}.Overview(now)
		if len(o.Windows) != 2 || o.Windows[0].Key != KeyFiveHour || o.Windows[0].Known {
			t.Errorf("got %+v", o)
		}
	})
}

func TestFormatRenewal(t *testing.T) {
	loc := time.FixedZone("test", 5*3600)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, loc) // a Wednesday
	if got := FormatRenewal(0, now); got != "" {
		t.Errorf("unreported = %q", got)
	}
	if got := FormatRenewal(time.Date(2026, 11, 4, 15, 45, 0, 0, loc).Unix(), now); got != "15:45" {
		t.Errorf("same day = %q", got)
	}
	if got := FormatRenewal(time.Date(2026, 11, 7, 9, 5, 0, 0, loc).Unix(), now); got != "Sat 09:05" {
		t.Errorf("another day = %q", got)
	}
}
