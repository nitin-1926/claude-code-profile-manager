package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// A real SessionEnd payload leaves the marker the desktop app reacts to; the same
// command run any other way must not, or a manual sync would read as a session
// ending.
func TestUsageSyncMarksOnlyARealSessionEnd(t *testing.T) {
	scratchConfig(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dir := cfg.Profiles["work"].Dir

	run := func(stdin string) {
		t.Helper()
		c := &cobra.Command{}
		c.SetIn(strings.NewReader(stdin))
		if err := runUsageSync(c, []string{"work"}); err != nil {
			t.Fatal(err)
		}
	}

	run("")
	if got := usage.LoadSessionEnd(dir); got != (usage.SessionEnd{}) {
		t.Errorf("a manual sync recorded %+v", got)
	}

	run(`{"session_id":"abc","transcript_path":"/x/abc.jsonl","hook_event_name":"SessionEnd","reason":"other"}`)
	got := usage.LoadSessionEnd(dir)
	if got.SessionID != "abc" || got.TranscriptPath != "/x/abc.jsonl" || got.EndedAt == 0 {
		t.Errorf("hook run recorded %+v", got)
	}
	// Written in the watched store, where the watcher can see it.
	if _, err := os.Stat(filepath.Join(usage.Dir(dir), "session-end.json")); err != nil {
		t.Error(err)
	}
}

func limitsFixture(t *testing.T, now time.Time) (*config.Config, []string) {
	t.Helper()
	profiles := map[string]config.ProfileConfig{}
	for _, name := range []string{"apikey", "stale", "work"} {
		profiles[name] = config.ProfileConfig{Name: name, Dir: t.TempDir()}
	}
	if err := usage.SaveLimits(profiles["work"].Dir, usage.Limits{
		CapturedAt: now.Add(-3 * time.Minute).Unix(),
		Windows: []usage.LimitWindow{
			{Key: usage.KeyFiveHour, UsedPercentage: 82.9, ResetsAt: now.Add(time.Minute).Unix()},
			{Key: usage.KeySevenDay, UsedPercentage: 41, ResetsAt: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := usage.SaveLimits(profiles["stale"].Dir, usage.Limits{
		CapturedAt: now.Add(-48 * time.Hour).Unix(),
		Windows: []usage.LimitWindow{
			{Key: usage.KeyFiveHour, UsedPercentage: 99, ResetsAt: now.Add(-time.Hour).Unix()},
			{Key: usage.KeySevenDay, UsedPercentage: 104, ResetsAt: now.Add(time.Minute).Unix()},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return &config.Config{Profiles: profiles}, []string{"apikey", "stale", "work"}
}

func TestRenderLimitsTable(t *testing.T) {
	now := time.Now()
	cfg, names := limitsFixture(t, now)

	var buf bytes.Buffer
	renderLimitsTable(&buf, cfg, names, now)

	rows := map[string][]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			rows[f[0]] = f
		}
	}
	renews := usage.FormatRenewal(now.Add(time.Minute).Unix(), now)

	if got := strings.Join(rows["apikey"], " "); got != "apikey unknown unknown unknown unknown" {
		t.Errorf("no reading: %q", got)
	}
	// Floored, the reset clock is shown, 7-day has none, and the age is visible.
	if got, want := strings.Join(rows["work"], " "), "work 82%, renews "+renews+" 41% 5-hour 3m ago"; got != want {
		t.Errorf("reading:\n got %q\nwant %q", got, want)
	}
	// A passed reset reads unknown, overage is real, and an old reading says so.
	if got, want := strings.Join(rows["stale"], " "), "stale unknown 104%, renews "+renews+" 7-day 2d ago"; got != want {
		t.Errorf("stale:\n got %q\nwant %q", got, want)
	}
}

// A profile whose token store cannot be read fails its report, but must not take
// the limits table down with it: that view is promised never to error.
func TestUsageAllPrintsLimitsWhenAReportFails(t *testing.T) {
	scratchConfig(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := usage.Dir(cfg.Profiles["work"].Dir)
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "sessions.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	usageAll = true
	t.Cleanup(func() { usageAll = false })

	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&buf)
	if err := runUsage(c, nil); err == nil {
		t.Fatal("want the report's failure back, or this does not test the failure path")
	}
	if !strings.Contains(buf.String(), "Plan limits") || !strings.Contains(buf.String(), "unknown") {
		t.Errorf("no limits table:\n%s", buf.String())
	}
}

// --json extends `--all` additively: still an array of the existing objects, each
// gaining limits, in the encoding limits.json stores.
func TestUsageAllJSONCarriesLimits(t *testing.T) {
	now := time.Now()
	cfg, names := limitsFixture(t, now)
	usageAll = true
	t.Cleanup(func() { usageAll = false })

	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&buf)
	if err := emitUsageJSON(c, cfg, names, ""); err != nil {
		t.Fatal(err)
	}
	var out []usageJSONOut
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not an array: %v\n%s", err, buf.String())
	}
	if len(out) != 3 {
		t.Fatalf("got %d rows", len(out))
	}

	none := out[0].Limits
	if none == nil || none.CapturedAt != 0 || none.Closest != "" || len(none.Windows) != 2 || none.Windows[0].Status != "unknown" {
		t.Errorf("no reading: %+v", none)
	}
	work := out[2].Limits
	if work == nil || work.Closest != usage.KeyFiveHour || work.AgeSeconds < 180 {
		t.Fatalf("reading: %+v", work)
	}
	if w := work.Windows[0]; w.Status != "ok" || w.UsedPercentage != 82.9 || w.ResetsAt != now.Add(time.Minute).Unix() {
		t.Errorf("five_hour must keep the stored encoding: %+v", w)
	}
}

func TestHumanTokens(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{175, "175"},
		{1500, "1.5K"},
		{4_200_000, "4.20M"},
		{1_100_000_000, "1.10B"},
	}
	for _, c := range cases {
		if got := humanTokens(c.in); got != c.want {
			t.Errorf("humanTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanInt(t *testing.T) {
	if got := humanInt(3307); got != "3,307" {
		t.Errorf("humanInt(3307) = %q, want 3,307", got)
	}
	if got := humanInt(42); got != "42" {
		t.Errorf("humanInt(42) = %q, want 42", got)
	}
}

// TestBuildUsageJSONContract guards the stable snake_case scripting contract.
func TestBuildUsageJSONContract(t *testing.T) {
	view := usage.View{
		Totals:   usage.Tokens{Input: 10, Output: 20, CacheCreation: 5, CacheRead: 65},
		Messages: 3,
		ByModel:  []usage.NamedTotal{{Name: "claude-opus-4-8", Tokens: usage.Tokens{Input: 10, Output: 20}}},
		ByDay:    []usage.DayTotal{{Date: "2026-06-27", Tokens: usage.Tokens{Input: 10}, Messages: 1}},
		Sessions: []*usage.SessionRecord{{SessionID: "s1", Cwd: "/r", LastTS: "2026-06-27T10:00:00Z", Tokens: usage.Tokens{Input: 10}, Messages: 1}},
	}
	out := buildUsageJSON("work", view, "2026-06-01")

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"profile", "since", "totals", "messages", "by_model", "by_project", "by_day", "sessions"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing top-level key %q in JSON contract", k)
		}
	}
	// totals.total must be the sum of all four kinds.
	var totals tokensJSON
	if err := json.Unmarshal(m["totals"], &totals); err != nil {
		t.Fatal(err)
	}
	if totals.Total != 100 {
		t.Errorf("totals.total = %d, want 100", totals.Total)
	}
	if out.Profile != "work" || out.Since != "2026-06-01" {
		t.Errorf("profile/since wrong: %q / %q", out.Profile, out.Since)
	}
}

// TestBuildUsageJSONNeverEmitsNullArrays pins the --json contract for a profile
// with no usage: the four list fields must serialize as [] rather than null, so
// scripts can iterate without a nil check. The desktop side already guarantees
// this (services.emptyUsage); the CLI did not.
func TestBuildUsageJSONNeverEmitsNullArrays(t *testing.T) {
	out := buildUsageJSON("fresh", usage.View{}, "")

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"by_model", "by_project", "by_day", "sessions"} {
		if got := string(m[k]); got != "[]" {
			t.Errorf("%s = %s, want []", k, got)
		}
	}
}
