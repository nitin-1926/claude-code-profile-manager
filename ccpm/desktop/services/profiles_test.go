//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProfilesListLive exercises the read path against the machine's real ~/.ccpm.
// It asserts only structural invariants (so it passes on any machine, including
// one with zero profiles) and logs the live data for inspection.
func TestProfilesListLive(t *testing.T) {
	s := NewProfiles()
	got, err := s.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	for _, p := range got {
		if p.Name == "" {
			t.Errorf("profile with empty name: %+v", p)
		}
		if p.Dir == "" {
			t.Errorf("profile %q has empty dir", p.Name)
		}
	}
	t.Logf("List() returned %d profile(s)", len(got))
	for _, p := range got {
		t.Logf("  %-8s default=%-5v auth=%-7s assets[sk=%d ag=%d cmd=%d rule=%d hook=%d plug=%d]",
			p.Name, p.IsDefault, p.AuthMethod,
			p.Counts.Skills, p.Counts.Agents, p.Counts.Commands,
			p.Counts.Rules, p.Counts.Hooks, p.Counts.Plugins)
	}
}

// The overview's asset counts and the Assets tab's rows skipped different
// entries — the count took "_"-prefixed ones the tab hides — so a profile
// could claim three skills and list two.
func TestAssetCountsMatchTheAssetsTab(t *testing.T) {
	name := syntheticProfile(t)
	skills := filepath.Join(os.Getenv("HOME"), ".ccpm", "profiles", name, "skills")
	for _, d := range []string{"demo", ".hidden", "_internal"} {
		if err := os.MkdirAll(filepath.Join(skills, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p, err := NewProfiles().Get(name)
	if err != nil || p == nil {
		t.Fatalf("Get: %v", err)
	}
	c, err := NewCascade().Get(name)
	if err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, a := range c.Assets {
		if a.Kind == "skill" {
			listed++
		}
	}
	if p.Counts.Skills != listed || listed != 1 {
		t.Errorf("counted %d skills, Assets tab lists %d, want 1 each", p.Counts.Skills, listed)
	}
}
