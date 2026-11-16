//go:build darwin

package rail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

func TestBuildMenuRows(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	future := now.Add(time.Hour).Unix()
	past := now.Add(-time.Minute).Unix()

	five := func(used float64, resets int64) services.LimitWindowDTO {
		return services.LimitWindowDTO{Key: usage.KeyFiveHour, Label: "5-hour", UsedPercentage: used, ResetsAt: resets}
	}
	seven := func(used float64, resets int64) services.LimitWindowDTO {
		return services.LimitWindowDTO{Key: usage.KeySevenDay, Label: "7-day", UsedPercentage: used, ResetsAt: resets}
	}

	tests := []struct {
		name string
		in   services.ProfileLimits
		want string
	}{
		{
			name: "both windows",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{five(42.4, future), seven(12.6, future)}},
			want: "work  5h 42%  7d 13%",
		},
		{
			// The order is the menu's, not the reading's, so rows line up.
			name: "windows listed backwards",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{seven(10, future), five(20, future)}},
			want: "work  5h 20%  7d 10%",
		},
		{
			name: "no reading shows the bare name, never 0%",
			in:   services.ProfileLimits{Profile: "api", Available: false, Reason: services.ReasonNoData, Windows: []services.LimitWindowDTO{}},
			want: "api",
		},
		{
			// A window the reading does not have is left out, not shown as 0%.
			name: "one window known",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{seven(30, future)}},
			want: "work  7d 30%",
		},
		{
			name: "over 100 prints as reported, as the notch does",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{five(120, future)}},
			want: "work  5h 120%",
		},
		{
			// Rolled over: the notch reads this as empty until the next reading.
			name: "reading past its reset",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{five(90, past), seven(40, future)}},
			want: "work  5h 0%  7d 40%",
		},
		{
			name: "no reset clock is not a reset",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{five(55, 0)}},
			want: "work  5h 55%",
		},
		{
			name: "unknown window key is skipped",
			in:   services.ProfileLimits{Profile: "work", Available: true, Windows: []services.LimitWindowDTO{{Key: "opus_weekly", UsedPercentage: 77}}},
			want: "work",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := BuildMenuRows([]services.ProfileLimits{tt.in}, now)
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1", len(rows))
			}
			if rows[0].Title != tt.want {
				t.Errorf("title = %q, want %q", rows[0].Title, tt.want)
			}
			if rows[0].Profile != tt.in.Profile {
				t.Errorf("profile = %q, want %q", rows[0].Profile, tt.in.Profile)
			}
		})
	}
}

// The C layer decodes this JSON, and a nil slice would arrive as null. Both
// the empty list and a failed limits read reach here.
func TestBuildMenuRowsEmptyEncodesAsArray(t *testing.T) {
	for _, in := range [][]services.ProfileLimits{nil, {}} {
		b, err := json.Marshal(BuildMenuRows(in, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "[]" {
			t.Errorf("BuildMenuRows(%v) encodes as %s, want []", in, b)
		}
	}
}

// The whole feed from real files on disk, in the shape `ccpm statusline` writes,
// so the fixtures above cannot drift from what LimitsService actually returns.
func TestMenuAndDockFromARealLimitsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now()
	profile := func(name string) string { return filepath.Join(home, ".ccpm", "profiles", name) }

	if err := os.MkdirAll(filepath.Join(profile("work"), "usage"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(profile("api"), 0o700); err != nil {
		t.Fatal(err)
	}
	limits := fmt.Sprintf(`{"version":1,"captured_at":%d,"source":"statusline","windows":[
		{"key":"five_hour","label":"5-hour","used_percentage":58,"resets_at":%d},
		{"key":"seven_day","label":"7-day","used_percentage":13,"resets_at":%d}]}`,
		now.Unix(), now.Add(time.Hour).Unix(), now.Add(24*time.Hour).Unix())
	if err := os.WriteFile(filepath.Join(profile("work"), "usage", "limits.json"), []byte(limits), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := json.Marshal(map[string]any{
		"version": "1",
		"profiles": map[string]any{
			"api":  map[string]any{"name": "api", "dir": profile("api"), "auth_method": "api_key"},
			"work": map[string]any{"name": "work", "dir": profile("work"), "auth_method": "oauth"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := services.NewLimits().All()
	if err != nil {
		t.Fatal(err)
	}

	rows := BuildMenuRows(got, now)
	want := []MenuRow{{Profile: "api", Title: "api"}, {Profile: "work", Title: "work  5h 58%  7d 13%"}}
	if !slices.Equal(rows, want) {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}

	// api sorts first but has no reading, so the meter is work's.
	meter := BuildDockMeter(got, "", services.RailMainFiveHour, now)
	if wantMeter := (DockMeter{Shown: true, Remaining: 42, RGB: NotchWatch}); meter != wantMeter {
		t.Errorf("meter = %+v, want %+v", meter, wantMeter)
	}
}

func TestBuildMenuRowsKeepsCallerOrder(t *testing.T) {
	rows := BuildMenuRows([]services.ProfileLimits{{Profile: "b"}, {Profile: "a"}}, time.Now())
	if rows[0].Profile != "b" || rows[1].Profile != "a" {
		t.Errorf("rows reordered: %+v", rows)
	}
}

// Identity crosses to Objective-C as data. Whatever a name contains has to come
// back byte-identical for the click, not be interpreted on the way.
func TestMenuRowIdentitySurvivesEncoding(t *testing.T) {
	const hostile = `a"; rm -rf ~; $(x) ü`
	b, err := json.Marshal(BuildMenuRows([]services.ProfileLimits{{Profile: hostile}}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var back []MenuRow
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back[0].Profile != hostile || back[0].Title != hostile {
		t.Errorf("round trip changed the name: %+v", back[0])
	}
}
