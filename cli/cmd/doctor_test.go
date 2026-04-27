package cmd

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.1.56", "2.1.56", 0},
		{"2.1.56", "2.1.55", 1},
		{"2.1.55", "2.1.56", -1},
		{"2.1.56", "2.1.5", 1},   // numeric compare, not lexicographic
		{"v2.1.56", "2.1.56", 0}, // "v" prefix tolerated
		{"2.1.56 (claude-code)", "2.1.56", 0},
		{"2.0.0", "2.1.0", -1},
		{"2.10.0", "2.9.0", 1},
		{"", "2.1.0", -1},    // empty parses as 0; 0 < 2
		{"2.1.0", "", 1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestExtractNumericPrefix(t *testing.T) {
	cases := map[string]string{
		"v2.1.56":              "2.1.56",
		"2.1.56 (claude-code)": "2.1.56",
		"   v0.3.2  ":          "0.3.2",
		"abc":                  "",
		"":                     "",
		"1.2.3-beta":           "1.2.3",
	}
	for in, want := range cases {
		if got := extractNumericPrefix(in); got != want {
			t.Errorf("extractNumericPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
