package usage

import (
	"strings"
	"testing"
	"time"
)

func TestPickBucket(t *testing.T) {
	cases := []struct {
		day, max int64
		want     int
	}{
		{0, 100, 0},
		{1, 0, 0},
		{5, 100, 1},   // 5% -> 1
		{10, 100, 1},  // 10% -> 1
		{20, 100, 2},  // 25% boundary band
		{40, 100, 3},  // <=50 -> 3
		{60, 100, 4},  // <=75 -> 4
		{100, 100, 5}, // max -> 5
	}
	for _, c := range cases {
		if got := pickBucket(c.day, c.max); got != c.want {
			t.Errorf("pickBucket(%d,%d) = %d, want %d", c.day, c.max, got, c.want)
		}
	}
}

func TestRenderHeatmapPlain(t *testing.T) {
	prev := bucketLocation
	bucketLocation = time.UTC
	defer func() { bucketLocation = prev }()

	end := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	days := map[string]*DailyRecord{
		"2026-06-25": {Tokens: Tokens{Input: 1000}},
		"2026-06-20": {Tokens: Tokens{Input: 50}},
	}
	out := RenderHeatmap(days, end, 6, false)

	if strings.Contains(out, "\033") {
		t.Fatal("plain heatmap contains ANSI escape")
	}
	// 1 month header + 7 day rows + blank + legend = at least 9 lines.
	if n := strings.Count(out, "\n"); n < 9 {
		t.Fatalf("expected >=9 lines, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "Less ") || !strings.Contains(out, " More") {
		t.Fatal("legend missing")
	}
	if !strings.Contains(out, "Jun") && !strings.Contains(out, "Ju") {
		t.Fatalf("month label missing:\n%s", out)
	}
	// The busiest day (1000) must reach the top bucket glyph.
	if !strings.Contains(out, plainGlyphs[5]) {
		t.Fatalf("expected a max-intensity glyph for the busy day:\n%s", out)
	}
}

func TestRenderHeatmapColor(t *testing.T) {
	prev := bucketLocation
	bucketLocation = time.UTC
	defer func() { bucketLocation = prev }()

	end := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	days := map[string]*DailyRecord{"2026-06-25": {Tokens: Tokens{Input: 1000}}}
	out := RenderHeatmap(days, end, 4, true)
	if !strings.Contains(out, "\033[38;5;171m") { // top purple
		t.Fatalf("expected top purple color code:\n%q", out)
	}
}
