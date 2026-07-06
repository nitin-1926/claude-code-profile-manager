package usage

import (
	"fmt"
	"strings"
	"time"
)

// amberRamp is the ascending-intensity xterm-256 amber/gold ramp for heatmap
// buckets 1..5 (matching the CCPM Desktop darkmatter theme — amber primary —
// so the CLI and GUI heatmaps read as one product). Bucket 0 (no activity) uses
// emptyColor. Kept as a var so the palette is easy to tweak.
var amberRamp = []int{94, 136, 172, 214, 220}

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
	if bucket > len(amberRamp) {
		bucket = len(amberRamp)
	}
	return amberRamp[bucket-1]
}

// cell renders one day as a 2-wide block (color) or shaded glyphs (plain).
func cell(bucket int, color bool) string {
	if color {
		return fmt.Sprintf("\033[38;5;%dm██\033[0m", cellColor(bucket))
	}
	if bucket < 0 {
		bucket = 0
	}
	if bucket >= len(plainGlyphs) {
		bucket = len(plainGlyphs) - 1
	}
	return plainGlyphs[bucket] + plainGlyphs[bucket]
}

// legendCell renders one 1-wide legend swatch.
func legendCell(bucket int, color bool) string {
	if color {
		return fmt.Sprintf("\033[38;5;%dm█\033[0m", cellColor(bucket))
	}
	return plainGlyphs[bucket]
}

// startOfWeek returns the Sunday (00:00) of t's week, matching GitHub's layout.
func startOfWeek(t time.Time) time.Time {
	y, m, d := t.Date()
	base := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	return base.AddDate(0, 0, -int(base.Weekday()))
}

// RenderHeatmap draws a GitHub-style contribution graph — weeks as columns, 7
// day-rows (Sun..Sat) — for the `weeks` weeks ending at `end`, using the purple
// ramp. Pure: the clock is injected via end and color toggles ANSI.
func RenderHeatmap(days map[string]*DailyRecord, end time.Time, weeks int, color bool) string {
	if weeks <= 0 {
		weeks = 26
	}
	end = end.In(bucketLocation)
	startSunday := startOfWeek(end).AddDate(0, 0, -7*(weeks-1))

	totalFor := func(d time.Time) int64 {
		if dr := days[d.Format("2006-01-02")]; dr != nil {
			return dr.Tokens.Total()
		}
		return 0
	}

	var maxDay int64
	for i := 0; i < weeks*7; i++ {
		d := startSunday.AddDate(0, 0, i)
		if d.After(end) {
			continue
		}
		if t := totalFor(d); t > maxDay {
			maxDay = t
		}
	}

	const gutter = "     " // 5 chars, aligns month header with day rows
	var b strings.Builder

	// Month label header — 2 chars per week column; print the month's first two
	// letters above the column where a new month begins.
	b.WriteString(gutter)
	prevMonth := ""
	for w := 0; w < weeks; w++ {
		mon := startSunday.AddDate(0, 0, w*7).Format("Jan")
		if mon != prevMonth {
			b.WriteString(mon[:2])
			prevMonth = mon
		} else {
			b.WriteString("  ")
		}
	}
	b.WriteByte('\n')

	// Seven day-rows.
	for dow := 0; dow < 7; dow++ {
		label := ""
		switch dow {
		case 1:
			label = "Mon"
		case 3:
			label = "Wed"
		case 5:
			label = "Fri"
		}
		b.WriteString(fmt.Sprintf("%-5s", label))
		for w := 0; w < weeks; w++ {
			d := startSunday.AddDate(0, 0, w*7+dow)
			if d.After(end) {
				b.WriteString("  ")
				continue
			}
			b.WriteString(cell(pickBucket(totalFor(d), maxDay), color))
		}
		b.WriteByte('\n')
	}

	// Legend.
	b.WriteString("\n" + gutter + "Less ")
	for bucket := 0; bucket <= 5; bucket++ {
		b.WriteString(legendCell(bucket, color))
	}
	b.WriteString(" More\n")

	return b.String()
}
