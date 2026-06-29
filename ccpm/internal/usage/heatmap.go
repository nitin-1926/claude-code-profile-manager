package usage

import (
	"fmt"
	"strings"
	"time"
)

// purpleRamp is the ascending-intensity xterm-256 purple ramp for heatmap
// buckets 1..5 (instead of GitHub's green). Bucket 0 (no activity) uses
// emptyColor. Kept as a var so the palette is easy to tweak.
var purpleRamp = []int{53, 55, 91, 133, 171}

const emptyColor = 238 // dim grey for days with no activity

// plainGlyphs renders the same six buckets without color (NO_COLOR / dumb term).
var plainGlyphs = []string{"·", "░", "▒", "▓", "█", "█"}

// pickBucket maps a day's token total to 0..5 given the window's busiest day.
// 0 = no activity; 1..5 grade by fraction of the max so busy days are brightest.
// Pure and deterministic.
func pickBucket(dayTotal, maxDay int64) int {
	if dayTotal <= 0 || maxDay <= 0 {
		return 0
	}
	switch frac := float64(dayTotal) / float64(maxDay); {
	case frac <= 0.10:
		return 1
	case frac <= 0.25:
		return 2
	case frac <= 0.50:
		return 3
	case frac <= 0.75:
		return 4
	default:
		return 5
	}
}

func cellColor(bucket int) int {
	if bucket <= 0 {
		return emptyColor
	}
	if bucket > len(purpleRamp) {
		bucket = len(purpleRamp)
	}
	return purpleRamp[bucket-1]
}

