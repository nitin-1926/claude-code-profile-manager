//go:build darwin

package services

import "testing"

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
