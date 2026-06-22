package cmd

import (
	"testing"
	"time"
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
			want:    "⬢ work · Sonnet 4.6 · ctx 34% · 5h 58% ↺" + wantReset + " · 7d 88% · $1.23",
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
			got := renderStatusLine(tc.in, tc.profile, fixedNow)
			if got != tc.want {
				t.Fatalf("renderStatusLine:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestFormatWindowPastResetDropsClock verifies a window whose reset is already
// in the past renders the percent without a stale clock.
func TestFormatWindowPastResetDropsClock(t *testing.T) {
	w := &rateWindow{UsedPercentage: 90, ResetsAt: fixedNow.Add(-time.Hour).Unix()}
	got := formatWindow("5h", w, fixedNow)
	if got != "5h 10%" {
		t.Fatalf("got %q, want %q", got, "5h 10%")
	}
}
