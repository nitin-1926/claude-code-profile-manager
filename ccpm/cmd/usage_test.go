package cmd

import (
	"encoding/json"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

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
