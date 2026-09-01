//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

func writeClaudeJSON(t *testing.T, dir, email, tier string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"oauthAccount": map[string]any{
			"emailAddress":              email,
			"organizationRateLimitTier": tier,
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLimitsForAvailableProfile(t *testing.T) {
	dir := t.TempDir()
	writeClaudeJSON(t, dir, "nitin@rocketium.com", "default_claude_max_5x")
	if err := usage.SaveLimits(dir, usage.Limits{
		CapturedAt: 1_700_000_000,
		Windows: []usage.LimitWindow{
			{Key: usage.KeyFiveHour, Label: usage.LabelFiveHour, UsedPercentage: 73, ResetsAt: 1_700_003_060},
			{Key: usage.KeySevenDay, Label: usage.LabelSevenDay, UsedPercentage: 7, ResetsAt: 1_700_500_000},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got := limitsFor("work", dir)
	if !got.Available || got.Reason != ReasonOK {
		t.Fatalf("want available with no reason, got available=%v reason=%q", got.Available, got.Reason)
	}
	if got.Account != "nitin@rocketium.com" || got.Plan != "default_claude_max_5x" {
		t.Errorf("identity not carried through: account=%q plan=%q", got.Account, got.Plan)
	}
	if len(got.Windows) != 2 || got.Windows[0].UsedPercentage != 73 {
		t.Errorf("windows wrong: %+v", got.Windows)
	}
	if got.CapturedAt != 1_700_000_000 {
		t.Errorf("capturedAt = %d, want the stored value", got.CapturedAt)
	}
}

// The honesty requirement: a profile with no reading must never look like a
// profile at 0%. It reports unavailable, with a reason the UI can explain.
func TestLimitsForMissingReadingIsUnavailable(t *testing.T) {
	t.Run("subscription account, no reading yet", func(t *testing.T) {
		dir := t.TempDir()
		writeClaudeJSON(t, dir, "labs@rocketium.com", "default_claude_max_5x")

		got := limitsFor("labs", dir)
		if got.Available {
			t.Fatal("a profile with no limits.json reported available")
		}
		if got.Reason != ReasonNoData {
			t.Errorf("reason = %q, want %q — this profile can report, it just has not yet", got.Reason, ReasonNoData)
		}
		if got.Account != "labs@rocketium.com" {
			t.Errorf("identity should still render for an unavailable profile, got %q", got.Account)
		}
		if got.Windows == nil {
			t.Error("windows is nil; marshals to null and breaks .map in the frontend")
		}
	})

	t.Run("non-subscription account never reports", func(t *testing.T) {
		dir := t.TempDir()
		writeClaudeJSON(t, dir, "someone@example.com", "default_raven")

		got := limitsFor("cin", dir)
		if got.Available {
			t.Fatal("a non-subscription profile reported available")
		}
		// The distinction matters: telling this user to "run Claude Code once"
		// would be a lie — their tier never sends rate_limits at all.
		if got.Reason != ReasonNotSubscribed {
			t.Errorf("reason = %q, want %q", got.Reason, ReasonNotSubscribed)
		}
	})

	t.Run("no .claude.json at all", func(t *testing.T) {
		got := limitsFor("bare", t.TempDir())
		if got.Available {
			t.Fatal("an empty profile dir reported available")
		}
		if got.Reason != ReasonNoData {
			t.Errorf("reason = %q, want %q when the plan is unknown", got.Reason, ReasonNoData)
		}
	})
}

func TestLimitsGetUnknownProfileIsSafe(t *testing.T) {
	var s LimitsService
	got, err := s.Get("definitely-not-a-profile")
	if err != nil {
		t.Fatalf("unknown profile returned an error: %v", err)
	}
	if got.Available {
		t.Error("unknown profile reported available")
	}
	if got.Windows == nil {
		t.Error("unknown profile returned nil windows; must be [] not null")
	}
	if got.Profile != "definitely-not-a-profile" {
		t.Errorf("profile name not echoed back: %q", got.Profile)
	}
}

func TestLimitsAllIsSortedAndNonNil(t *testing.T) {
	var s LimitsService
	got, err := s.All()
	if err != nil {
		t.Skipf("no readable ccpm config on this machine: %v", err)
	}
	if got == nil {
		t.Fatal("All returned nil; must be an empty slice so JSON is [] not null")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Profile > got[i].Profile {
			t.Fatalf("All is not name-sorted; ring order would shuffle between refreshes: %q before %q",
				got[i-1].Profile, got[i].Profile)
		}
	}
	for _, p := range got {
		if p.Windows == nil {
			t.Errorf("profile %q has nil windows", p.Profile)
		}
		if !p.Available && len(p.Windows) != 0 {
			t.Errorf("profile %q is unavailable but carries windows: %+v", p.Profile, p.Windows)
		}
	}
}

func TestPlanLabel(t *testing.T) {
	cases := map[string]string{
		"default_claude_max_5x":  "Max 5x",
		"default_claude_max_20x": "Max 20x",
		"default_claude_max":     "Max",
		"default_claude_pro":     "Pro",
		"":                       "",
		// An unrecognised tier must pass through, not be guessed at.
		"default_raven": "default_raven",
	}
	for in, want := range cases {
		if got := PlanLabel(in); got != want {
			t.Errorf("PlanLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanReportsLimits(t *testing.T) {
	for _, tier := range []string{"default_claude_max_5x", "default_claude_max_20x", "default_claude_pro"} {
		if !planReportsLimits(tier) {
			t.Errorf("%q should be treated as a limit-reporting tier", tier)
		}
	}
	for _, tier := range []string{"default_raven", "", "some_api_tier"} {
		if planReportsLimits(tier) {
			t.Errorf("%q should not be treated as a limit-reporting tier", tier)
		}
	}
}

func TestAgeMatchesUsageEngine(t *testing.T) {
	now := time.Unix(1_700_003_600, 0)
	if got := Age(1_700_000_000, now); got != time.Hour {
		t.Errorf("Age = %v, want 1h", got)
	}
	if got := Age(0, now); got != 0 {
		t.Errorf("Age of an absent reading = %v, want 0", got)
	}
}
