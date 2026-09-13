package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

// full returns the nine catalog keys minus the ones named, so the cases below
// can vary one bucket without restating the other eight keys each time.
func full(exclude ...string) string {
	drop := map[string]bool{}
	for _, k := range exclude {
		drop[k] = true
	}
	var keep []string
	for _, s := range statusline.Segments {
		if !drop[s.Key] {
			keep = append(keep, s.Key)
		}
	}
	return strings.Join(keep, ",")
}

func TestLayoutFromFlags(t *testing.T) {
	got, err := layoutFromFlags(statusline.Profile, statusline.Cost, full(statusline.Profile, statusline.Cost))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.Row1, []string{statusline.Profile}) {
		t.Errorf("Row1 = %v", got.Row1)
	}
	if !reflect.DeepEqual(got.Row2, []string{statusline.Cost}) {
		t.Errorf("Row2 = %v", got.Row2)
	}
	if len(got.Off) != len(statusline.Segments)-2 {
		t.Errorf("Off = %v, want the remaining %d segments", got.Off, len(statusline.Segments)-2)
	}
}

func TestLayoutFromFlagsPreservesGivenOrder(t *testing.T) {
	got, err := layoutFromFlags(statusline.Cost+","+statusline.Profile, full(statusline.Cost, statusline.Profile), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{statusline.Cost, statusline.Profile}
	if !reflect.DeepEqual(got.Row1, want) {
		t.Errorf("Row1 = %v, want %v — the caller's order must survive", got.Row1, want)
	}
}

func TestLayoutFromFlagsToleratesWhitespaceAndEmptyEntries(t *testing.T) {
	got, err := layoutFromFlags(" "+statusline.Profile+" , ,"+statusline.Model, full(statusline.Profile, statusline.Model), " ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.Row1, []string{statusline.Profile, statusline.Model}) {
		t.Errorf("Row1 = %v", got.Row1)
	}
}

// TestLayoutFromFlagsRejectsIncompleteInput is the guard that makes the desktop
// app's write authoritative. Normalize deliberately treats an unmentioned
// segment as newly-introduced and places it at its default — correct for a
// config written by an older ccpm, wrong for a UI that dropped a field. The
// only way to tell those apart is to refuse the incomplete write here.
func TestLayoutFromFlagsRejectsIncompleteInput(t *testing.T) {
	cases := []struct {
		name             string
		row1, row2, off  string
		wantErrSubstring string
	}{
		{
			name:             "a missing segment is refused rather than defaulted",
			row1:             statusline.Profile,
			row2:             statusline.Cost,
			wantErrSubstring: "missing:",
		},
		{
			name:             "a segment on both rows is refused",
			row1:             full(),
			row2:             statusline.Profile,
			wantErrSubstring: "more than once",
		},
		{
			name:             "a segment both shown and hidden is refused",
			row1:             full(),
			off:              statusline.Cost,
			wantErrSubstring: "more than once",
		},
		{
			name:             "an unknown segment is refused",
			row1:             full() + ",weather",
			wantErrSubstring: `unknown segment "weather"`,
		},
		{
			name:             "a typo is refused rather than silently dropped",
			row1:             full(statusline.Profile) + ",profil",
			wantErrSubstring: `unknown segment "profil"`,
		},
		{
			name:             "entirely empty input is refused",
			wantErrSubstring: "missing:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := layoutFromFlags(tc.row1, tc.row2, tc.off)
			if err == nil {
				t.Fatalf("want an error containing %q, got nil", tc.wantErrSubstring)
			}
			if !strings.Contains(err.Error(), tc.wantErrSubstring) {
				t.Errorf("error %q does not contain %q", err, tc.wantErrSubstring)
			}
		})
	}
}

// TestLayoutFromFlagsAcceptsEverythingOff covers the layout a user can reach by
// unchecking all nine boxes. It must save, not error — the renderer already
// prints nothing for it.
func TestLayoutFromFlagsAcceptsEverythingOff(t *testing.T) {
	got, err := layoutFromFlags("", "", full())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Row1) != 0 || len(got.Row2) != 0 {
		t.Fatalf("want both rows empty, got %+v", got)
	}
	if rows := renderStatusLine(statusLinePreviewInput(), "work", fixedNow, false, got); len(rows) != 0 {
		t.Errorf("an all-off layout rendered %q, want nothing", rows)
	}
}

// TestLayoutFromFlagsRoundTripsThroughNormalize — what the flags accept must
// survive storage untouched, or the desktop app would save one thing and read
// back another.
func TestLayoutFromFlagsRoundTripsThroughNormalize(t *testing.T) {
	built, err := layoutFromFlags(
		statusline.Model+","+statusline.Profile,
		statusline.Cost,
		full(statusline.Model, statusline.Profile, statusline.Cost),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	back := statusline.Normalize(built.Store())
	if !reflect.DeepEqual(built, back) {
		t.Errorf("round trip changed the layout:\n saved %+v\n read  %+v", built, back)
	}
}

// TestDocumentedExamplesActuallyRun parses the invocations out of the command's
// own help text and feeds them to the real validator.
//
// The help shipped an example — `--off branch,effort --profile work` — that
// layoutFromFlags rejects outright, because it named two of the nine segments
// and the flags require all of them. A user copying it got an error from the
// documentation. Nothing catches that class of bug except executing the docs,
// so this does.
func TestDocumentedExamplesActuallyRun(t *testing.T) {
	// Unwrap shell line continuations, then take each `ccpm statusline
	// configure ...` line as one invocation.
	help := strings.ReplaceAll(statusLineConfigureCmd.Long, "\\\n", " ")

	found := 0
	for _, line := range strings.Split(help, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ccpm statusline configure") {
			continue
		}
		found++
		t.Run(line, func(t *testing.T) {
			row1, row2, off, reset := parseExampleFlags(t, line)
			if reset {
				return // --reset takes no segment lists
			}
			if row1 == "" && row2 == "" && off == "" {
				return // the bare interactive form
			}
			if _, err := layoutFromFlags(row1, row2, off); err != nil {
				t.Errorf("the help text documents a command that fails:\n  %s\n  %v", line, err)
			}
		})
	}
	if found < 3 {
		t.Fatalf("only found %d example invocations in the help text — the parser has drifted from the docs", found)
	}
}

// parseExampleFlags pulls --row1/--row2/--off/--reset out of one example line.
func parseExampleFlags(t *testing.T, line string) (row1, row2, off string, reset bool) {
	t.Helper()
	fields := strings.Fields(line)
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--reset":
			reset = true
		case "--row1", "--row2", "--off", "--profile":
			if i+1 >= len(fields) {
				t.Fatalf("flag %s has no value in %q", fields[i], line)
			}
			switch fields[i] {
			case "--row1":
				row1 = fields[i+1]
			case "--row2":
				row2 = fields[i+1]
			case "--off":
				off = fields[i+1]
			}
			i++
		}
	}
	return row1, row2, off, reset
}

func TestSplitSegments(t *testing.T) {
	cases := map[string][]string{
		"":            {},
		"   ":         {},
		",,":          {},
		"a":           {"a"},
		"a,b":         {"a", "b"},
		" a , b ":     {"a", "b"},
		"a,,b,":       {"a", "b"},
		"a, ,b":       {"a", "b"},
		"multi word":  {"multi word"},
		"a\t,\tb\n":   {"a", "b"},
		"trailing,":   {"trailing"},
		",leading":    {"leading"},
		"dup,dup":     {"dup", "dup"}, // de-duping is layoutFromFlags's job, not this one's
		"UPPER,lower": {"UPPER", "lower"},
	}
	for in, want := range cases {
		if got := splitSegments(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitSegments(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestExceptAndIntersect(t *testing.T) {
	all := []string{"a", "b", "c", "d"}
	if got := except(all, []string{"b", "d", "zz"}); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("except = %v", got)
	}
	if got := except(all, nil); !reflect.DeepEqual(got, all) {
		t.Errorf("except with nothing removed = %v, want %v", got, all)
	}
	if got := intersect([]string{"d", "a", "zz"}, all); !reflect.DeepEqual(got, []string{"d", "a"}) {
		t.Errorf("intersect = %v, want the wanted order preserved", got)
	}
	if got := intersect(nil, all); len(got) != 0 {
		t.Errorf("intersect of nothing = %v", got)
	}
}

// TestPreviewInputCoversEverySegment keeps the picker's preview honest: a
// segment absent from the sample payload would render as nothing, reading as
// "this segment does not work" rather than "this sample has no data".
func TestPreviewInputCoversEverySegment(t *testing.T) {
	in := statusLinePreviewInput()
	for _, s := range statusline.Segments {
		layout := statusline.Layout{Row1: []string{s.Key}}
		rows := renderStatusLine(in, "work", fixedNow, false, layout)
		if len(rows) != 1 || strings.TrimSpace(rows[0]) == "" {
			t.Errorf("segment %q renders nothing from the preview payload — the preview would misreport it as broken", s.Key)
		}
	}
}
