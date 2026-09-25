//go:build darwin

package rail

import "math"

// The rail is a native panel, so it cannot inherit the frontend's CSS custom
// properties. This file is the one place those tokens are mirrored into Go.
//
// The oklch triples below are copied verbatim from
// desktop/frontend/src/globals.css — the `[data-theme="..."]` blocks are the
// source of truth, and this table cites them rather than replacing them.
// They are converted here rather than pre-baked as hex so a drift between the
// two files is a visible diff on a recognisable value, not an opaque colour
// constant nobody can check by eye.

// RGB is an 8-bit-per-channel colour, packed 0xRRGGBB for the Objective-C side.
type RGB uint32

// Palette is the small set of tokens the rail actually draws with. Deliberately
// not the whole theme: every token here has a mark on screen, and one that does
// not is one more thing to keep in sync for nothing.
type Palette struct {
	// Foreground is the percentage text.
	Foreground RGB `json:"foreground"`
	// MutedForeground is the profile label under each ring.
	MutedForeground RGB `json:"mutedForeground"`
	// Track is the unfilled part of a ring.
	Track RGB `json:"track"`
	// Border separates slots.
	Border RGB `json:"border"`
	// Surface tints the vibrancy view so the rail reads as part of the app.
	Surface RGB `json:"surface"`
}

// Theme names, matching the `data-theme` values the frontend sets on <html>.
const (
	ThemeGraphite = "graphite"
	ThemeMidnight = "midnight"
	ThemeLight    = "light"
)

// oklch holds a colour exactly as globals.css writes it, so the two files can
// be diffed by eye: lightness 0..1, chroma, hue in degrees.
type oklch struct{ L, C, H float64 }

// palettes mirrors globals.css. Keep the field order and the comment on each
// line matching the CSS custom property it came from.
var palettes = map[string]struct {
	foreground, mutedForeground, muted, border, card oklch
}{
	ThemeGraphite: {
		foreground:      oklch{0.9150, 0.0040, 65}, // --foreground
		mutedForeground: oklch{0.6650, 0.0070, 65}, // --muted-foreground
		muted:           oklch{0.3150, 0.0050, 65}, // --muted
		border:          oklch{0.3300, 0.0050, 65}, // --border
		card:            oklch{0.2760, 0.0050, 65}, // --card
	},
	ThemeMidnight: {
		foreground:      oklch{0.8109, 0, 0}, // --foreground
		mutedForeground: oklch{0.6268, 0, 0}, // --muted-foreground
		muted:           oklch{0.2520, 0, 0}, // --muted
		border:          oklch{0.2520, 0, 0}, // --border
		card:            oklch{0.1822, 0, 0}, // --card
	},
	ThemeLight: {
		foreground:      oklch{0.2400, 0.0250, 264.6645}, // --foreground
		mutedForeground: oklch{0.4500, 0.0200, 264.3637}, // --muted-foreground
		muted:           oklch{0.9600, 0.0040, 80},       // --muted
		border:          oklch{0.8600, 0.0050, 80},       // --border
		card:            oklch{1.0000, 0, 0},             // --card
	},
}

// PaletteFor resolves a theme name, falling back to graphite — the app's own
// default — for anything unrecognised, so a hand-edited preferences file cannot
// leave the rail drawing in zero-value black.
func PaletteFor(theme string) Palette {
	p, ok := palettes[theme]
	if !ok {
		p = palettes[ThemeGraphite]
	}
	return Palette{
		Foreground:      p.foreground.rgb(),
		MutedForeground: p.mutedForeground.rgb(),
		Track:           p.muted.rgb(),
		Border:          p.border.rgb(),
		Surface:         p.card.rgb(),
	}
}

// Headroom grading. These are the CLI's own colours, not lookalikes: xterm-256
// 42, 215 and 203, the values ccpm/cmd/statusline.go paints the statusline
// percentage with. Sharing the numbers as well as the thresholds means the rail
// and the terminal cannot drift into disagreeing about what "nearly out" looks
// like.
const (
	ColorHealthy    RGB = 0x00D787 // xterm 42  — cGreen
	ColorTightening RGB = 0xFFAF5F // xterm 215 — cAmber
	ColorNearLimit  RGB = 0xFF5F5F // xterm 203 — cRed
)

// HeadroomColor grades a window by how much is LEFT, mirroring headroomColor in
// ccpm/cmd/statusline.go: healthy at >=50% remaining, tightening at >=20%,
// near-limit below that. Takes remaining rather than used so the comparison
// against that function is direct.
func HeadroomColor(remaining int) RGB {
	switch {
	case remaining >= 50:
		return ColorHealthy
	case remaining >= 20:
		return ColorTightening
	default:
		return ColorNearLimit
	}
}

// The notch's own palette: codenotch's hues (MIT, vinzdg/codenotch), which are
// tuned to read on its always-black body. The notch is black whatever the app
// theme, so these are the dark-appearance values and never vary with it.
const (
	NotchAmple    RGB = 0x00FF88
	NotchWatch    RGB = 0xF2FF00
	NotchCritical RGB = 0xFF3F00
)

// NotchColor grades a window for the notch.
//
// The THRESHOLDS are HeadroomColor's, deliberately not codenotch's (which band
// at 50% and 70% used). The notch and the terminal status line describe the
// same windows, and a profile that reads amber in one and red in the other
// would be two different answers to the same question. Only the hue is
// codenotch's.
func NotchColor(remaining int) RGB {
	switch HeadroomColor(remaining) {
	case ColorHealthy:
		return NotchAmple
	case ColorTightening:
		return NotchWatch
	default:
		return NotchCritical
	}
}

// rgb converts oklch to packed 8-bit sRGB, via Oklab and linear sRGB.
// Coefficients are Björn Ottosson's, as used by the CSS Color 4 definition
// browsers implement for oklch() — the same numbers that produce what the user
// sees in the app window.
func (c oklch) rgb() RGB {
	h := c.H * math.Pi / 180
	a := c.C * math.Cos(h)
	b := c.C * math.Sin(h)

	// Oklab to cone responses.
	l := cube(c.L + 0.3963377774*a + 0.2158037573*b)
	m := cube(c.L - 0.1055613458*a - 0.0638541728*b)
	s := cube(c.L - 0.0894841775*a - 1.2914855480*b)

	// Cone responses to linear sRGB.
	lr := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	lg := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	lb := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s

	return RGB(channel(lr))<<16 | RGB(channel(lg))<<8 | RGB(channel(lb))
}

func cube(v float64) float64 { return v * v * v }

// channel applies the sRGB transfer function and clamps. Out-of-gamut inputs
// are clipped per channel, which is what browsers do for oklch() colours that
// fall outside sRGB, so the rail matches the window beside it.
func channel(linear float64) int {
	var v float64
	if linear <= 0.0031308 {
		v = 12.92 * linear
	} else {
		v = 1.055*math.Pow(linear, 1/2.4) - 0.055
	}
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 255
	}
	return int(math.Round(v * 255))
}
