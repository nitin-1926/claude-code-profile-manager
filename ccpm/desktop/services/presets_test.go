//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// A key on argv is readable by every local user through `ps`.
func TestCreateFromPresetSendsKeyOnStdinOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeCCPM(t, `printf '%s\n' "$@" > "$HOME/argv"; /bin/cat > "$HOME/stdin"`)

	const key = "sk-or-v1-secret"
	if r := NewMutate().CreateFromPreset("router", "openrouter", key); !r.OK {
		t.Fatalf("CreateFromPreset: %+v", r)
	}
	argv, err := os.ReadFile(filepath.Join(home, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(argv)), []string{"add", "--preset", "openrouter", "--", "router"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("argv = %v, want %v", got, want)
	}
	if strings.Contains(string(argv), key) {
		t.Error("key reached argv")
	}
	stdin, err := os.ReadFile(filepath.Join(home, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != key+"\n" {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestCreateFromPresetRejectsBadName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeCCPM(t, `exit 0`)
	if r := NewMutate().CreateFromPreset("../evil", "openrouter", "k"); r.OK || r.Error == "" {
		t.Fatalf("bad name accepted: %+v", r)
	}
}

// setBaseURL points the synthetic profile at another endpoint.
func setBaseURL(t *testing.T, name string) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles[name]
	p.Env = map[string]string{"ANTHROPIC_BASE_URL": "https://provider.example"}
	cfg.Profiles[name] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

// The fixture's transcripts name a Claude model, the shape a provider that maps
// Claude names produces, so the price table would answer confidently and wrongly.
func TestCostIsUnknownOnCustomEndpoint(t *testing.T) {
	name, sessRel, _, _ := historyFixture(t)
	// Blocks only cover recent activity, and the fixture's transcripts are old.
	now := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	recent := filepath.Join(profileDir(name), "projects", filepath.Dir(sessRel), "recent.jsonl")
	if err := os.WriteFile(recent, []byte(asstLine(t, "r1", "", "recent", "msg_r1", "hi", now, false)), 0o600); err != nil {
		t.Fatal(err)
	}

	sum := func() (usageCost, blockCost, historyCost float64, unknown bool) {
		u, err := NewUsage().Get(name, "all")
		if err != nil {
			t.Fatal(err)
		}
		usageCost, unknown = u.Cost, u.CostUnknown
		for _, m := range u.ByModel {
			usageCost += m.Cost
		}
		for _, d := range u.ByDay {
			usageCost += d.Cost
		}
		for _, p := range u.ByProject {
			usageCost += p.Cost
		}
		blocks, err := NewUsage().Blocks(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range blocks {
			blockCost += b.Cost + b.CostPerHour + b.ProjectedCost
		}
		rows, err := NewHistory().Sessions(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			historyCost += r.Cost
		}
		return usageCost, blockCost, historyCost, unknown
	}

	// Control: without this the assertions below pass on a fixture with no cost.
	if u, b, h, unknown := sum(); u == 0 || b == 0 || h == 0 || unknown {
		t.Fatalf("Anthropic profile: usage=%v blocks=%v history=%v unknown=%v", u, b, h, unknown)
	}

	setBaseURL(t, name)
	if u, b, h, unknown := sum(); u != 0 || b != 0 || h != 0 || !unknown {
		t.Errorf("custom endpoint: usage=%v blocks=%v history=%v unknown=%v", u, b, h, unknown)
	}
}

// An API-key profile holds no subscription tier and will never report limits, so
// it reads "does not report limits", not "no reading yet" or an error.
func TestLimitsOnAPIKeyProfileSayItWillNotReport(t *testing.T) {
	name := syntheticProfile(t)
	if l, err := NewLimits().Get(name); err != nil || l.Reason != ReasonNoData {
		t.Fatalf("oauth profile: reason=%q err=%v", l.Reason, err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles[name]
	p.AuthMethod = "api_key"
	cfg.Profiles[name] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	l, err := NewLimits().Get(name)
	if err != nil {
		t.Fatal(err)
	}
	if l.Available || l.Reason != ReasonNotSubscribed {
		t.Errorf("api-key profile: %+v", l)
	}
	assertNoNullArrays(t, l, "windows")
	all, err := NewLimits().All()
	if err != nil || len(all) != 1 || all[0].Reason != ReasonNotSubscribed {
		t.Errorf("All: %+v, %v", all, err)
	}
}
