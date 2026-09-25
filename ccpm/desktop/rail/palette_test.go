//go:build darwin

package rail

import "testing"

// The oklch conversion is the part of the palette that can be silently wrong —
// a plausible-looking but incorrect colour still renders. Anchor it on values
// with known answers before trusting it on theme tokens.
func TestOklchConversionAnchors(t *testing.T) {
	// The three sRGB primaries and their oklch coordinates. These are the
	// round-trip values browsers produce for oklch(), so agreeing with them is
	// the same as agreeing with what the app window renders. Allowing one unit
	// of slack per channel absorbs the rounding in the published coordinates,
	// not in the conversion.
	cases := []struct {
		name string
		in   oklch
		want RGB
	}{
		{"white", oklch{1, 0, 0}, 0xFFFFFF},
		{"black", oklch{0, 0, 0}, 0x000000},
		{"red", oklch{0.6280, 0.2577, 29.2338}, 0xFF0000},
		{"green", oklch{0.8664, 0.2948, 142.4953}, 0x00FF00},
		{"blue", oklch{0.4520, 0.3132, 264.0520}, 0x0000FF},
	}
	for _, c := range cases {
		got := c.in.rgb()
		for shift, ch := range map[int]string{16: "R", 8: "G", 0: "B"} {
			g := int(got>>shift) & 0xFF
			w := int(c.want>>shift) & 0xFF
			if g-w > 1 || w-g > 1 {
				t.Errorf("%s: %v -> %#06x, want ~%#06x (%s channel %d vs %d)", c.name, c.in, got, c.want, ch, g, w)
				break
			}
		}
	}
}

// Zero chroma must stay neutral at every hue — a hue-dependent grey means the
// a/b terms are being applied when they should cancel.
func TestZeroChromaIsAlwaysNeutral(t *testing.T) {
	for _, h := range []float64{0, 65, 180, 264.6645, 359} {
		c := oklch{0.6, 0, h}.rgb()
		r, g, b := (c>>16)&0xFF, (c>>8)&0xFF, c&0xFF
		if r != g || g != b {
			t.Errorf("hue %v with zero chroma gave %#06x (r=%d g=%d b=%d)", h, c, r, g, b)
		}
	}
}

// Every theme must resolve a full token set. A zero value here is black, which
// on a dark panel is invisible rather than obviously broken — exactly the kind
// of bug that ships.
func TestEveryThemeResolvesDistinctNonZeroTokens(t *testing.T) {
	for _, theme := range []string{ThemeGraphite, ThemeMidnight, ThemeLight} {
		p := PaletteFor(theme)
		tokens := map[string]RGB{
			"Foreground":      p.Foreground,
			"MutedForeground": p.MutedForeground,
			"Track":           p.Track,
			"Border":          p.Border,
			"Surface":         p.Surface,
		}
		for name, v := range tokens {
			if v == 0 {
				t.Errorf("%s: %s is zero-value black", theme, name)
			}
		}
		// Text must not be the same colour as what it sits on.
		if p.Foreground == p.Surface {
			t.Errorf("%s: foreground and surface are both %#06x", theme, p.Foreground)
		}
		if p.Foreground == p.Track {
			t.Errorf("%s: foreground and ring track are both %#06x", theme, p.Foreground)
		}
	}
}

// Light and dark themes must actually differ in the direction they claim: text
// light-on-dark for graphite and midnight, dark-on-light for light.
func TestThemePolarity(t *testing.T) {
	for _, theme := range []string{ThemeGraphite, ThemeMidnight} {
		p := PaletteFor(theme)
		if luma(p.Foreground) <= luma(p.Surface) {
			t.Errorf("%s: foreground %#06x is not lighter than surface %#06x", theme, p.Foreground, p.Surface)
		}
	}
	p := PaletteFor(ThemeLight)
	if luma(p.Foreground) >= luma(p.Surface) {
		t.Errorf("light: foreground %#06x is not darker than surface %#06x", p.Foreground, p.Surface)
	}
}

func TestPaletteForFallsBackToGraphite(t *testing.T) {
	want := PaletteFor(ThemeGraphite)
	for _, s := range []string{"", "GRAPHITE", "solarized", "midnight "} {
		if got := PaletteFor(s); got != want {
			t.Errorf("PaletteFor(%q) = %+v, want the graphite fallback", s, got)
		}
	}
}

// Grading must agree with headroomColor in ccpm/cmd/statusline.go at every
// boundary. The two live in different packages and nothing but this test stops
// them drifting into disagreeing about what "nearly out" means.
func TestHeadroomColorMatchesStatuslineThresholds(t *testing.T) {
	cases := []struct {
		remaining int
		want      RGB
	}{
		{100, ColorHealthy},
		{51, ColorHealthy},
		{50, ColorHealthy}, // boundary: >=50 is healthy
		{49, ColorTightening},
		{21, ColorTightening},
		{20, ColorTightening}, // boundary: >=20 is tightening
		{19, ColorNearLimit},
		{0, ColorNearLimit},
		{-5, ColorNearLimit}, // overage
	}
	for _, c := range cases {
		if got := HeadroomColor(c.remaining); got != c.want {
			t.Errorf("HeadroomColor(%d) = %#06x, want %#06x", c.remaining, got, c.want)
		}
	}
}

func luma(c RGB) float64 {
	r, g, b := float64((c>>16)&0xFF), float64((c>>8)&0xFF), float64(c&0xFF)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// TestNotchColorSharesTheStatuslineThresholds pins the one rule that matters
// about the notch palette: it may look like codenotch, but it must band at
// exactly the same points as the terminal status line.
func TestNotchColorSharesTheStatuslineThresholds(t *testing.T) {
	for remaining := 0; remaining <= 100; remaining++ {
		want := map[RGB]RGB{ColorHealthy: NotchAmple, ColorTightening: NotchWatch, ColorNearLimit: NotchCritical}[HeadroomColor(remaining)]
		if got := NotchColor(remaining); got != want {
			t.Errorf("NotchColor(%d) = %06x, want %06x — the notch and the status line would disagree", remaining, got, want)
		}
	}
}
