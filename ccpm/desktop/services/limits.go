//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// Reasons a profile has no limit reading. These are machine-readable so the UI
// can explain the gap in words instead of drawing a ring at 0% — a profile that
// has never reported limits is not a profile with plenty of headroom.
const (
	ReasonOK            = ""
	ReasonNoData        = "no-data"
	ReasonNotSubscribed = "not-subscription-account"
)

// LimitWindowDTO is one rate-limit window as the frontend sees it.
type LimitWindowDTO struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	UsedPercentage float64 `json:"usedPercentage"`
	// ResetsAt is Unix seconds; 0 means Claude Code reported no reset clock.
	ResetsAt int64 `json:"resetsAt"`
}

// ProfileLimits is the per-profile payload behind one ring on the rail.
type ProfileLimits struct {
	Profile string `json:"profile"`
	// Account and Plan come from the profile's own .claude.json, so a row can
	// read "nitin@rocketium.com · Max 5x" rather than just a profile name.
	Account string `json:"account"`
	Plan    string `json:"plan"`
	// Available gates the whole reading. False means render the unavailable
	// state and show Reason, never a percentage.
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	// CapturedAt is Unix seconds; 0 when there is no reading.
	CapturedAt int64            `json:"capturedAt"`
	Windows    []LimitWindowDTO `json:"windows"`
}

// LimitsService surfaces the per-profile subscription windows cached by
// `ccpm statusline` (see internal/usage/limits.go for why that is the source).
type LimitsService struct{}

func NewLimits() *LimitsService { return &LimitsService{} }

// Get returns one profile's windows. An unknown profile is not an error — it
// returns a populated empty DTO, matching every other service here.
func (s *LimitsService) Get(profile string) (ProfileLimits, error) {
	cfg, err := config.Load()
	if err != nil {
		return emptyLimits(profile, ReasonNoData), err
	}
	pc, ok := cfg.Profiles[profile]
	if !ok {
		return emptyLimits(profile, ReasonNoData), nil
	}
	return limitsFor(profile, pc.Dir), nil
}

// All returns every configured profile's windows, name-sorted so the rail's
// ring order is stable across refreshes rather than following Go's random map
// iteration.
func (s *LimitsService) All() ([]ProfileLimits, error) {
	cfg, err := config.Load()
	if err != nil {
		return []ProfileLimits{}, err
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ProfileLimits, 0, len(names))
	for _, name := range names {
		out = append(out, limitsFor(name, cfg.Profiles[name].Dir))
	}
	return out, nil
}

func limitsFor(profile, dir string) ProfileLimits {
	account, plan := accountAndPlan(dir)
	l := usage.LoadLimits(dir)
	if !l.Available() {
		out := emptyLimits(profile, reasonForMissing(plan))
		out.Account, out.Plan = account, plan
		return out
	}

	windows := make([]LimitWindowDTO, 0, len(l.Windows))
	for _, w := range l.Windows {
		windows = append(windows, LimitWindowDTO{
			Key:            w.Key,
			Label:          w.Label,
			UsedPercentage: w.UsedPercentage,
			ResetsAt:       w.ResetsAt,
		})
	}
	return ProfileLimits{
		Profile:    profile,
		Account:    account,
		Plan:       plan,
		Available:  true,
		Reason:     ReasonOK,
		CapturedAt: l.CapturedAt,
		Windows:    windows,
	}
}

// reasonForMissing distinguishes "we have not seen a reading yet" from "this
// account will never produce one", so the UI can tell the user to run Claude
// Code once instead of waiting forever on a profile that cannot report.
func reasonForMissing(plan string) string {
	if plan != "" && !planReportsLimits(plan) {
		return ReasonNotSubscribed
	}
	return ReasonNoData
}

// planReportsLimits reports whether a rate-limit tier is one Claude Code sends
// `rate_limits` for. Claude.ai subscription tiers carry a claude_max / claude_pro
// marker; API-key and other tiers do not.
func planReportsLimits(tier string) bool {
	return strings.Contains(tier, "claude_max") || strings.Contains(tier, "claude_pro")
}

func emptyLimits(profile, reason string) ProfileLimits {
	// Non-nil slice: a nil marshals to null and the frontend's .map would throw.
	return ProfileLimits{
		Profile:   profile,
		Available: false,
		Reason:    reason,
		Windows:   []LimitWindowDTO{},
	}
}

// claudeAccount is the slice of a profile's .claude.json we need for the
// identity line. Mirrors the shape internal/credentials reads.
type claudeAccount struct {
	OAuthAccount *struct {
		EmailAddress              string `json:"emailAddress"`
		OrganizationRateLimitTier string `json:"organizationRateLimitTier"`
	} `json:"oauthAccount"`
}

// accountAndPlan reads the profile's own .claude.json. Both values are
// best-effort decoration — a missing file just yields empty strings.
func accountAndPlan(dir string) (account, plan string) {
	data, err := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if err != nil {
		return "", ""
	}
	var cj claudeAccount
	if json.Unmarshal(data, &cj) != nil || cj.OAuthAccount == nil {
		return "", ""
	}
	return cj.OAuthAccount.EmailAddress, cj.OAuthAccount.OrganizationRateLimitTier
}

// PlanLabel turns a raw rate-limit tier ("default_claude_max_5x") into
// something a person reads ("Max 5x"). Unknown tiers fall through unchanged
// rather than being labelled with a plan the user does not have.
func PlanLabel(tier string) string {
	switch {
	case strings.Contains(tier, "claude_max_20x"):
		return "Max 20x"
	case strings.Contains(tier, "claude_max_5x"):
		return "Max 5x"
	case strings.Contains(tier, "claude_max"):
		return "Max"
	case strings.Contains(tier, "claude_pro"):
		return "Pro"
	case tier == "":
		return ""
	default:
		return tier
	}
}

// Age is exposed so the frontend and the rail format freshness identically.
func Age(capturedAt int64, now time.Time) time.Duration {
	return usage.Limits{CapturedAt: capturedAt}.Age(now)
}
