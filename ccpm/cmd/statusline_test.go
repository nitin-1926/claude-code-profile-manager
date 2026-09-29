package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// fixedNow is a deterministic clock for reset-time formatting.
var fixedNow = time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)

func TestRenderStatusLine(t *testing.T) {
	// resetsAt one hour after fixedNow, in UTC, so the formatted clock is 15:00
	// regardless of the host timezone (time.Unix renders in local time, so build
	// the expectation from the same conversion).
	resetAt := fixedNow.Add(time.Hour).Unix()
	wantReset := time.Unix(resetAt, 0).Format("15:04")

	subscription := func() statusLineInput {
		var in statusLineInput
		in.Model.DisplayName = "Sonnet 4.6"
		in.ContextWindow.UsedPercentage = 34
		in.Cost.TotalCostUSD = 1.234
		in.RateLimits = &struct {
			FiveHour *rateWindow `json:"five_hour"`
			SevenDay *rateWindow `json:"seven_day"`
		}{
			FiveHour: &rateWindow{UsedPercentage: 42, ResetsAt: resetAt},
			SevenDay: &rateWindow{UsedPercentage: 12},
		}
		return in
	}

	cases := []struct {
		name    string
		in      statusLineInput
		profile string
		want    string
	}{
		{
			name:    "subscription full line",
			in:      subscription(),
			profile: "work",
			// Windows show percent USED (matching Claude's /usage), not remaining.
			want: "⬢ work · Sonnet 4.6 · ctx 34% · 5h 42% ↺" + wantReset + " · 7d 12% · $1.23",
		},
		{
			name: "api key profile drops rate-limit segments",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Opus 4.8"
				in.Cost.TotalCostUSD = 0.12
				return in
			}(),
			profile: "personal",
			want:    "⬢ personal · Opus 4.8 · $0.12",
		},
		{
			name: "falls back to model id when no display name",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.ID = "claude-opus-4-8"
				return in
			}(),
			profile: "work",
			want:    "⬢ work · claude-opus-4-8",
		},
		{
			name:    "no profile resolved omits the glyph",
			in:      statusLineInput{},
			profile: "",
			want:    "",
		},
		{
			name: "zero cost is omitted",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Haiku 4.5"
				return in
			}(),
			profile: "ci",
			want:    "⬢ ci · Haiku 4.5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// color=false keeps the assertions on plain text; coloring is
			// covered separately in TestRenderStatusLineColorized.
			got := renderStatusLine(tc.in, tc.profile, fixedNow, false)
			if got != tc.want {
				t.Fatalf("renderStatusLine:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestRenderStatusLineColorized verifies that enabling color wraps segments in
// ANSI codes (profile, orange window label, headroom-coloured percent) and
// always resets, while plain mode emits none of it.
func TestRenderStatusLineColorized(t *testing.T) {
	var in statusLineInput
	in.Model.DisplayName = "Opus 4.8"
	in.RateLimits = &struct {
		FiveHour *rateWindow `json:"five_hour"`
		SevenDay *rateWindow `json:"seven_day"`
	}{FiveHour: &rateWindow{UsedPercentage: 90}} // 10% left → red percent

	got := renderStatusLine(in, "work", fixedNow, true)
	for _, want := range []string{cProfile, cOrange, cRed, cReset} {
		if !strings.Contains(got, want) {
			t.Fatalf("colored output missing %q\n got %q", want, got)
		}
	}
	if plain := renderStatusLine(in, "work", fixedNow, false); strings.Contains(plain, "\033") {
		t.Fatalf("plain output unexpectedly contains ANSI: %q", plain)
	}
}

// TestFormatWindowPastResetDropsClock verifies a window whose reset is already
// in the past renders the percent without a stale clock.
func TestFormatWindowPastResetDropsClock(t *testing.T) {
	w := &rateWindow{UsedPercentage: 90, ResetsAt: fixedNow.Add(-time.Hour).Unix()}
	got := formatWindow("5h", w, fixedNow, false)
	if got != "5h 90%" { // percent USED, no stale clock
		t.Fatalf("got %q, want %q", got, "5h 90%")
	}
}

// End-to-end for the caching side effect added alongside the rendered line:
// a realistic payload must both print the usual status line AND leave a
// limits.json the desktop rail can read.
func TestRunStatusLineRenderPersistsRateLimits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows

	profileDir := filepath.Join(home, "profiles", "work")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, home, "work", profileDir)
	t.Setenv("CCPM_ACTIVE_PROFILE", "work")
	t.Setenv("NO_COLOR", "1")

	resetAt := time.Now().Add(51 * time.Minute).Unix()
	payload := fmt.Sprintf(`{
	  "model": {"display_name": "Sonnet 4.6"},
	  "context_window": {"used_percentage": 34},
	  "rate_limits": {
	    "five_hour": {"used_percentage": 73, "resets_at": %d},
	    "seven_day": {"used_percentage": 7, "resets_at": %d}
	  }
	}`, resetAt, resetAt+86400)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(payload))
	cmd.SetOut(&out)
	if err := runStatusLineRender(cmd, nil); err != nil {
		t.Fatalf("runStatusLineRender: %v", err)
	}

	// The line itself is the contract with the TUI and must be unaffected.
	if !strings.Contains(out.String(), "work") {
		t.Errorf("status line lost the profile name: %q", out.String())
	}

	got := usage.LoadLimits(profileDir)
	if !got.Available() {
		t.Fatalf("no limits cached after a payload carrying rate_limits: %+v", got)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("want 2 cached windows, got %d", len(got.Windows))
	}
	if got.Windows[0].UsedPercentage != 73 || got.Windows[0].Label != usage.LabelFiveHour {
		t.Errorf("five_hour cached wrong: %+v", got.Windows[0])
	}
	if got.Source != usage.SourceStatusLine {
		t.Errorf("source = %q, want %q", got.Source, usage.SourceStatusLine)
	}
}

// An API-key session sends no rate_limits. It must render normally and write
// nothing, rather than caching an empty reading.
func TestRunStatusLineRenderWithoutRateLimitsWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows

	profileDir := filepath.Join(home, "profiles", "apikey")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, home, "apikey", profileDir)
	t.Setenv("CCPM_ACTIVE_PROFILE", "apikey")
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"model":{"display_name":"Sonnet 4.6"},"context_window":{"used_percentage":10}}`))
	cmd.SetOut(&out)
	if err := runStatusLineRender(cmd, nil); err != nil {
		t.Fatalf("runStatusLineRender: %v", err)
	}

	if _, err := os.Stat(filepath.Join(usage.Dir(profileDir), "limits.json")); !os.IsNotExist(err) {
		t.Errorf("a payload with no rate_limits created a cache file (stat err: %v)", err)
	}
}

// An unregistered profile must never cause a write outside a ccpm-owned dir.
func TestPersistRateLimitsIgnoresUnknownProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeTestConfig(t, home, "work", filepath.Join(home, "profiles", "work"))

	var in statusLineInput
	in.RateLimits = &struct {
		FiveHour *rateWindow `json:"five_hour"`
		SevenDay *rateWindow `json:"seven_day"`
	}{FiveHour: &rateWindow{UsedPercentage: 50}}

	// Must not panic and must not write anywhere.
	persistRateLimits(in, "does-not-exist", time.Now())

	if dir := statusLineProfileDir("does-not-exist"); dir != "" {
		t.Errorf("resolved a directory for an unregistered profile: %q", dir)
	}
}

func writeTestConfig(t *testing.T, home, profile, dir string) {
	t.Helper()
	base := filepath.Join(home, ".ccpm")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"version":         "1",
		"default_profile": profile,
		"profiles": map[string]any{
			profile: map[string]any{"name": profile, "dir": dir, "auth_method": "oauth"},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
