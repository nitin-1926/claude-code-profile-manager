//go:build darwin

package services

import "testing"

// TestCascadeLive runs the cascade computation against the real ~/.ccpm and logs
// a provenance summary. Structural assertions only, so it passes on any machine.
func TestCascadeLive(t *testing.T) {
	ps, err := NewProfiles().List()
	if err != nil || len(ps) == 0 {
		t.Skip("no profiles to exercise cascade")
	}
	name := ps[0].Name
	c, err := NewCascade().Get(name)
	if err != nil {
		t.Fatalf("Cascade.Get(%q): %v", name, err)
	}
	byLayer := map[Layer]int{}
	for _, a := range c.Assets {
		if a.Name == "" || a.Layer == "" {
			t.Errorf("bad asset: %+v", a)
		}
		byLayer[a.Layer]++
	}
	t.Logf("profile %q: %d assets (host=%d global=%d profile=%d), %d settings keys",
		name, len(c.Assets), byLayer[LayerHost], byLayer[LayerGlobal], byLayer[LayerProfile], len(c.Settings))
	shadowed := 0
	for _, a := range c.Assets {
		if len(a.ShadowedIn) > 0 {
			shadowed++
		}
	}
	t.Logf("  %d shadowed assets", shadowed)
	for _, s := range c.Settings {
		if len(s.Contributors) > 1 {
			t.Logf("  setting %-24s winner=%s contributors=%v merged=%v", s.Key, s.Layer, s.Contributors, s.Merged)
		}
	}
}
