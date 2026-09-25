//go:build darwin

package rail

import (
	"strings"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// A fixed instant, so the weekday and clock forms are assertable rather than
// dependent on when the suite happens to run.
var now = time.Date(2026, 9, 2, 14, 30, 0, 0, time.UTC)

func in(d time.Duration) int64 { return now.Add(d).Unix() }

func TestFormatResetRelativeAndAbsolute(t *testing.T) {
	cases := []struct {
		name string
		at   int64
		want string
	}{
		{"under a minute", in(30 * time.Second), "Resets in under a minute"},
		{"minutes", in(51 * time.Minute), "Resets in 51 min"},
		{"exactly an hour", in(time.Hour), "Resets in 1 hr"},
		{"hours and minutes", in(2*time.Hour + 15*time.Minute), "Resets in 2 hr 15 min"},
		// Past the cutoff a relative form stops being actionable.
		{"days out", in(3 * 24 * time.Hour), "Resets Sat 2:30 PM"},
		// Never a negative duration.
		{"already passed", in(-8 * time.Minute), "Reset — refreshing"},
		{"exactly now", now.Unix(), "Reset — refreshing"},
		{"missing", 0, "Reset time unknown"},
		{"negative", -1, "Reset time unknown"},
	}
	for _, c := range cases {
		if got := FormatReset(c.at, now); got != c.want {
			t.Errorf("%s: FormatReset = %q, want %q", c.name, got, c.want)
		}
	}
}

// The whole point of the cutoff is that no reset ever renders as a huge
// relative number a person cannot act on.
func TestFormatResetNeverRendersAnAbsurdRelativeForm(t *testing.T) {
	for _, d := range []time.Duration{13 * time.Hour, 2 * 24 * time.Hour, 30 * 24 * time.Hour} {
		got := FormatReset(in(d), now)
		if strings.Contains(got, "in ") {
			t.Errorf("%v out rendered relatively as %q", d, got)
		}
	}
}

func TestFormatFreshness(t *testing.T) {
	cases := []struct {
		name string
		at   int64
		want string
	}{
		{"seconds", in(-20 * time.Second), "updated just now"},
		{"minutes", in(-5 * time.Minute), "updated 5 min ago"},
		{"hours", in(-3 * time.Hour), "updated 3 hr ago"},
		{"yesterday", in(-26 * time.Hour), "updated yesterday"},
		{"days", in(-72 * time.Hour), "updated 3 days ago"},
		{"never", 0, "never updated"},
		// Clock skew must not produce "updated in 3 min".
		{"future", in(3 * time.Minute), "updated just now"},
	}
	for _, c := range cases {
		if got := FormatFreshness(c.at, now); got != c.want {
			t.Errorf("%s: FormatFreshness = %q, want %q", c.name, got, c.want)
		}
	}
}

// Stale data must look stale. The cache only refreshes when that profile's
// statusline renders, so a number with no age on it invites the reader to
// assume it is live when it very often is not.
func TestStaleFlagCrossesAtTheThreshold(t *testing.T) {
	fresh := BuildCallout(availableLimits("work", in(-1*time.Minute)), now)
	if fresh.Stale {
		t.Error("a one-minute-old reading was marked stale")
	}
	old := BuildCallout(availableLimits("work", in(-StaleAfter-time.Minute)), now)
	if !old.Stale {
		t.Errorf("a reading older than %v was not marked stale", StaleAfter)
	}
	// Never-captured is not "stale", it is a different state entirely, and the
	// explanation covers it.
	never := BuildCallout(services.ProfileLimits{Profile: "cin", Reason: services.ReasonNoData}, now)
	if never.Stale {
		t.Error("a profile with no reading at all was marked stale rather than unavailable")
	}
}

func availableLimits(profile string, capturedAt int64) services.ProfileLimits {
	return services.ProfileLimits{
		Profile:    profile,
		Account:    "nitin@example.com",
		Plan:       "Max 5x",
		Available:  true,
		CapturedAt: capturedAt,
		Windows: []services.LimitWindowDTO{
			{Key: usage.KeyFiveHour, Label: "Current session", UsedPercentage: 80, ResetsAt: in(51 * time.Minute)},
			{Key: usage.KeySevenDay, Label: "All models", UsedPercentage: 30, ResetsAt: in(3 * 24 * time.Hour)},
		},
	}
}

func TestBuildCalloutRendersEveryWindow(t *testing.T) {
	c := BuildCallout(availableLimits("work", in(-2*time.Minute)), now)

	if c.Account == "" || c.Plan == "" {
		t.Error("callout dropped the account identity — which login a ring is about is the point of per-profile limits")
	}
	if c.Explanation != "" {
		t.Errorf("an available profile carried an explanation: %q", c.Explanation)
	}
	if len(c.Windows) != 2 {
		t.Fatalf("got %d windows, want 2", len(c.Windows))
	}
	if c.Windows[0].Percent != "80% used" {
		t.Errorf("percent = %q", c.Windows[0].Percent)
	}
	if c.Windows[0].Reset != "Resets in 51 min" {
		t.Errorf("reset = %q", c.Windows[0].Reset)
	}
	// 80% used is 20% left: tightening. 30% used is 70% left: healthy.
	if c.Windows[0].RGB != NotchWatch || c.Windows[1].RGB != NotchAmple {
		t.Errorf("window colours = %#06x/%#06x", c.Windows[0].RGB, c.Windows[1].RGB)
	}
}

// One window must render without a dangling separator — i.e. the model must
// not fabricate a second empty block to pad the layout.
func TestBuildCalloutWithASingleWindow(t *testing.T) {
	l := availableLimits("labs", in(-time.Minute))
	l.Windows = l.Windows[:1]
	c := BuildCallout(l, now)
	if len(c.Windows) != 1 {
		t.Fatalf("got %d windows, want exactly the one that was reported", len(c.Windows))
	}
}

// An unavailable profile must say why, and must never show a percentage.
func TestUnavailableCalloutExplainsAndShowsNoNumbers(t *testing.T) {
	for _, reason := range []string{services.ReasonNoData, services.ReasonNotSubscribed, "something-new"} {
		c := BuildCallout(services.ProfileLimits{Profile: "cin", Reason: reason}, now)
		if c.Explanation == "" {
			t.Errorf("%s: no explanation, leaving an unexplained blank", reason)
		}
		if len(c.Windows) != 0 {
			t.Errorf("%s: unavailable profile rendered %d windows", reason, len(c.Windows))
		}
	}
	// The two known reasons must not share wording — they call for different
	// actions from the reader.
	if ExplainUnavailable(services.ReasonNoData) == ExplainUnavailable(services.ReasonNotSubscribed) {
		t.Error("no-data and not-subscribed explain themselves identically")
	}
	// One tells the user to do something; the other tells them not to bother.
	if !strings.Contains(ExplainUnavailable(services.ReasonNoData), "run Claude Code") {
		t.Errorf("the no-data explanation does not say how to fix it: %q", ExplainUnavailable(services.ReasonNoData))
	}
}
