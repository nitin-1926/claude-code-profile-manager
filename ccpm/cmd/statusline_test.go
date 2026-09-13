package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

// fixedNow is a deterministic clock for reset-time formatting.
//
// time.Local, not time.UTC: the code under test compares a reset built by
// time.Unix (always local) against time.Now (also local). Pinning the test's
// clock to UTC while the expectation rendered in local time made the suite fail
// for anyone east of about UTC+12, where the reset crosses local midnight and
// resetClock correctly prefixes a date the expectation did not carry.
var fixedNow = time.Date(2026, 6, 25, 14, 0, 0, 0, time.Local)

func TestRenderStatusLine(t *testing.T) {
	// resetsAt one hour after fixedNow, in UTC, so the formatted clock is 15:00
	// regardless of the host timezone (time.Unix renders in local time, so build
	// the expectation from the same conversion).
	resetAt := fixedNow.Add(time.Hour).Unix()
	wantReset := time.Unix(resetAt, 0).Format("15:04")

	subscription := func() statusLineInput {
		var in statusLineInput
		in.Model.DisplayName = "Sonnet 4.6"
		in.ContextWindow.UsedPercentage = 34
		in.Cost.TotalCostUSD = 1.234
		in.Effort = &struct {
			Level string `json:"level"`
		}{Level: "high"}
		in.RateLimits = &struct {
			FiveHour *rateWindow `json:"five_hour"`
			SevenDay *rateWindow `json:"seven_day"`
		}{
			FiveHour: &rateWindow{UsedPercentage: 42, ResetsAt: resetAt},
			SevenDay: &rateWindow{UsedPercentage: 12},
		}
		return in
	}

	cases := []struct {
		name    string
		in      statusLineInput
		profile string
		// layout is the zero Layout for most cases, meaning "use the shipped
		// default"; cases that exercise a user's own choices set it explicitly.
		layout statusline.Layout
		want   []string
	}{
		{
			name:    "subscription splits the session from its budget",
			in:      subscription(),
			profile: "work",
			want: []string{
				"⬢ work · Sonnet 4.6 · ctx 34%",
				// Windows show percent USED (matching Claude's /usage), not remaining.
				"effort high · 5h 42% ↺" + wantReset + " · 7d 12% · $1.23",
			},
		},
		{
			name: "api key profile drops rate-limit segments",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Opus 4.8"
				in.Cost.TotalCostUSD = 0.12
				return in
			}(),
			profile: "personal",
			want:    []string{"⬢ personal · Opus 4.8", "$0.12"},
		},
		{
			name: "falls back to model id when no display name",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.ID = "claude-opus-4-8"
				return in
			}(),
			profile: "work",
			want:    []string{"⬢ work · claude-opus-4-8"},
		},
		{
			name:    "nothing to show prints nothing at all",
			in:      statusLineInput{},
			profile: "",
			want:    []string{},
		},
		{
			name: "session row alone when no profile or workspace resolves",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Haiku 4.5"
				return in
			}(),
			profile: "",
			want:    []string{"Haiku 4.5"},
		},
		{
			name: "budget row alone when the session row has nothing to say",
			in: func() statusLineInput {
				var in statusLineInput
				in.Cost.TotalCostUSD = 4.5
				return in
			}(),
			profile: "",
			want:    []string{"$4.50"},
		},
		{
			name: "session row alone when there is no budget yet",
			in: func() statusLineInput {
				var in statusLineInput
				in.Workspace.CurrentDir = "/tmp/nowhere"
				in.Workspace.ProjectDir = "/tmp/nowhere"
				return in
			}(),
			profile: "ci",
			want:    []string{"⬢ ci · nowhere"},
		},
		{
			name: "effort is omitted for models that do not report it",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Haiku 4.5"
				in.ContextWindow.UsedPercentage = 12
				return in
			}(),
			profile: "ci",
			want:    []string{"⬢ ci · Haiku 4.5 · ctx 12%"},
		},
		{
			name:    "a switched-off segment is not rendered even though its data is present",
			in:      subscription(),
			profile: "work",
			layout: statusline.Layout{
				Row1: []string{statusline.Profile, statusline.Model},
				Row2: []string{statusline.Cost},
				Off:  []string{statusline.Context, statusline.Effort, statusline.FiveHour, statusline.SevenDay},
			},
			want: []string{"⬢ work · Sonnet 4.6", "$1.23"},
		},
		{
			name:    "a segment moved to the other row renders there",
			in:      subscription(),
			profile: "work",
			layout: statusline.Layout{
				Row1: []string{statusline.Profile, statusline.Model, statusline.Effort},
				Row2: []string{statusline.Context, statusline.Cost},
			},
			want: []string{"⬢ work · Sonnet 4.6 · effort high", "ctx 34% · $1.23"},
		},
		{
			name:    "layout order wins over catalog order",
			in:      subscription(),
			profile: "work",
			layout: statusline.Layout{
				Row1: []string{statusline.Cost, statusline.Model, statusline.Profile},
			},
			want: []string{"$1.23 · Sonnet 4.6 · ⬢ work"},
		},
		{
			name:    "emptying row 2 collapses to a single row",
			in:      subscription(),
			profile: "work",
			layout: statusline.Layout{
				Row1: []string{statusline.Profile, statusline.Model},
				Off:  []string{statusline.Context, statusline.Effort, statusline.FiveHour, statusline.SevenDay, statusline.Cost},
			},
			want: []string{"⬢ work · Sonnet 4.6"},
		},
		{
			name:    "everything on row 2 leaves row 1 unprinted rather than blank",
			in:      subscription(),
			profile: "work",
			layout: statusline.Layout{
				Row2: []string{statusline.Profile, statusline.Model},
			},
			want: []string{"⬢ work · Sonnet 4.6"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layout := tc.layout
			if len(layout.Row1) == 0 && len(layout.Row2) == 0 {
				layout = statusline.Default()
			}
			// color=false keeps the assertions on plain text; coloring is
			// covered separately in TestRenderStatusLineColorized.
			got := renderStatusLine(tc.in, tc.profile, fixedNow, false, layout)
			if len(got) != len(tc.want) {
				t.Fatalf("renderStatusLine rows:\n got %q\nwant %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("row %d:\n got %q\nwant %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// writeStatusLineHome points $HOME at a scratch config holding one profile,
// with an optional global layout and per-profile override, and returns the
// profile name. The JSON is written by hand because that is the on-disk shape
// the command must actually read.
func writeStatusLineHome(t *testing.T, global, override map[string][]string) string {
	t.Helper()
	home := t.TempDir()
	// os.UserHomeDir reads $HOME on unix and %USERPROFILE% on Windows, so both
	// have to move or the test silently reads the real ~/.ccpm. That is not
	// hypothetical: setting only HOME made this pass on macOS and Linux while
	// on Windows it fell through to the default layout — and two of the four
	// subtests passed anyway, because "no config found" and "no layout
	// configured" produce the same output. Hence the assertion below.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const name = "render-test"
	dir := filepath.Join(home, ".ccpm", "profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	prof := map[string]any{"name": name, "dir": dir, "auth_method": "oauth"}
	if override != nil {
		prof["statusline"] = override
	}
	settings := map[string]any{}
	if global != nil {
		settings["statusline"] = global
	}
	b, err := json.Marshal(map[string]any{
		"version": "1", "default_profile": name,
		"profiles": map[string]any{name: prof},
		"settings": settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	// Fail loudly if the config the test just wrote is not the one the code
	// will read. Without this the suite degrades into testing the default
	// layout and still reports green.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if _, ok := cfg.Profiles[name]; !ok {
		t.Fatalf("the fixture config is not visible to config.Load — $HOME redirection did not take (profiles: %v)", cfg.Profiles)
	}
	return name
}

// renderVia runs the real command end to end: stdin payload in, rows out.
func renderVia(t *testing.T, profile, payload string) string {
	t.Helper()
	t.Setenv("CCPM_ACTIVE_PROFILE", profile)
	t.Setenv("NO_COLOR", "1")
	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetIn(strings.NewReader(payload))
	c.SetOut(&out)
	if err := runStatusLineRender(c, nil); err != nil {
		t.Fatalf("runStatusLineRender: %v", err)
	}
	return out.String()
}

// TestStatusLineCommandResolvesTheConfiguredLayout is the end-to-end wiring
// test the command had none of. renderStatusLine and statusline.Resolve were
// both well covered in isolation, but nothing checked that the command joins
// them: replacing `statusline.Resolve(cfg, profile)` with `statusline.Default()`
// — i.e. ignoring every layout any user ever configures — passed the whole
// suite.
func TestStatusLineCommandResolvesTheConfiguredLayout(t *testing.T) {
	const payload = `{"model":{"display_name":"Opus 5"},"context_window":{"used_percentage":34},"cost":{"total_cost_usd":1.23}}`

	t.Run("global layout is applied", func(t *testing.T) {
		name := writeStatusLineHome(t, map[string][]string{
			"row1": {statusline.Cost},
			"row2": {statusline.Model},
			"off":  {statusline.Profile, statusline.Workspace, statusline.Branch, statusline.Context, statusline.Effort, statusline.FiveHour, statusline.SevenDay},
		}, nil)

		got := renderVia(t, name, payload)
		want := "$1.23\nOpus 5\n"
		if got != want {
			t.Errorf("got %q, want %q — the configured layout was not used", got, want)
		}
	})

	t.Run("a profile override beats the global", func(t *testing.T) {
		name := writeStatusLineHome(t,
			map[string][]string{ // global: cost only
				"row1": {statusline.Cost},
				"off":  {statusline.Profile, statusline.Workspace, statusline.Branch, statusline.Model, statusline.Context, statusline.Effort, statusline.FiveHour, statusline.SevenDay},
			},
			map[string][]string{ // override: model only
				"row1": {statusline.Model},
				"off":  {statusline.Profile, statusline.Workspace, statusline.Branch, statusline.Context, statusline.Effort, statusline.FiveHour, statusline.SevenDay, statusline.Cost},
			})

		got := renderVia(t, name, payload)
		if got != "Opus 5\n" {
			t.Errorf("got %q, want %q — the profile override was ignored", got, "Opus 5\n")
		}
	})

	t.Run("no layout configured falls back to the built-in", func(t *testing.T) {
		name := writeStatusLineHome(t, nil, nil)
		got := renderVia(t, name, payload)
		want := "⬢ " + name + " · Opus 5 · ctx 34%\n$1.23\n"
		if got != want {
			t.Errorf("got %q, want the default layout %q", got, want)
		}
	})

	t.Run("a malformed payload prints nothing and does not fail", func(t *testing.T) {
		name := writeStatusLineHome(t, nil, nil)
		if got := renderVia(t, name, "{not json"); got != "⬢ "+name+"\n" {
			t.Errorf("got %q, want just the profile row", got)
		}
	})
}

// TestRenderStatusLineEverythingOffPrintsNothing covers the layout a user can
// reach by switching all nine segments off. Claude Code renders whatever the
// command prints, so emitting a blank line here would leave a permanently empty
// row pinned to the bottom of their session.
func TestRenderStatusLineEverythingOffPrintsNothing(t *testing.T) {
	var in statusLineInput
	in.Model.DisplayName = "Opus 5"
	in.Cost.TotalCostUSD = 9.99
	in.ContextWindow.UsedPercentage = 50

	got := renderStatusLine(in, "work", fixedNow, false, statusline.Layout{Off: allSegmentKeys()})
	if len(got) != 0 {
		t.Fatalf("want no rows at all, got %q", got)
	}
}

// TestRenderStatusLineSkipsBranchLookupWhenOff pins the behaviour that makes
// switching `branch` off worth doing: the segment reads .git off disk on every
// assistant message, so it must not be resolved when the layout omits it.
func TestRenderStatusLineSkipsBranchLookupWhenOff(t *testing.T) {
	repo := t.TempDir()
	writeGit(t, repo, "ref: refs/heads/should-not-appear\n")

	var in statusLineInput
	in.Workspace.ProjectDir = repo
	in.Workspace.CurrentDir = repo

	// Branch present: the on-disk HEAD is read and rendered.
	with := renderStatusLine(in, "", fixedNow, false, statusline.Layout{
		Row1: []string{statusline.Branch},
	})
	if len(with) != 1 || !strings.Contains(with[0], "should-not-appear") {
		t.Fatalf("branch segment did not render its on-disk branch: %q", with)
	}

	// Branch off: nothing to render, so nothing reads the repo.
	without := renderStatusLine(in, "", fixedNow, false, statusline.Layout{
		Row1: []string{statusline.Workspace},
		Off:  []string{statusline.Branch},
	})
	for _, row := range without {
		if strings.Contains(row, "should-not-appear") {
			t.Errorf("branch leaked into a layout that switched it off: %q", row)
		}
	}
}

// allSegmentKeys is every key in the catalog, for tests that need to account
// for all of them without restating the list as it grows.
func allSegmentKeys() []string {
	keys := make([]string, 0, len(statusline.Segments))
	for _, s := range statusline.Segments {
		keys = append(keys, s.Key)
	}
	return keys
}

// TestRenderStatusLineWorkspaceRow covers the session row's repo/directory and
// branch resolution against a real .git on disk.
func TestRenderStatusLineWorkspaceRow(t *testing.T) {
	repo := t.TempDir()
	writeGit(t, repo, "ref: refs/heads/feat/history-tab\n")
	// sub is built with the native separator, but the rendered label is always
	// forward-slashed, so the expectation below holds on every OS.
	sub := filepath.Join(repo, "ccpm", "internal")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	var in statusLineInput
	in.Workspace.ProjectDir = repo
	in.Workspace.CurrentDir = sub
	in.Workspace.Repo = &struct {
		Host  string `json:"host"`
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}{Host: "github.com", Owner: "nitin-1926", Name: "claude-code-profile-manager"}

	rows := renderStatusLine(in, "work", fixedNow, false, statusline.Default())
	if len(rows) != 1 {
		t.Fatalf("want only the session row, got %q", rows)
	}
	want := "⬢ work · claude-code-profile-manager/ccpm/internal · ⎇ feat/history-tab"
	if rows[0] != want {
		t.Fatalf("\n got %q\nwant %q", rows[0], want)
	}
}

func TestWorkspaceLabel(t *testing.T) {
	repoName := func(n string) *struct {
		Host  string `json:"host"`
		Owner string `json:"owner"`
		Name  string `json:"name"`
	} {
		return &struct {
			Host  string `json:"host"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		}{Name: n}
	}

	cases := []struct {
		name string
		set  func(*statusLineInput)
		want string
	}{
		{"repo name at its root", func(in *statusLineInput) {
			in.Workspace.Repo = repoName("ccpm")
			in.Workspace.ProjectDir = "/w/ccpm"
			in.Workspace.CurrentDir = "/w/ccpm"
		}, "ccpm"},
		{"repo name plus subdirectory", func(in *statusLineInput) {
			in.Workspace.Repo = repoName("ccpm")
			in.Workspace.ProjectDir = "/w/ccpm"
			in.Workspace.CurrentDir = "/w/ccpm/desktop/frontend"
		}, "ccpm/desktop/frontend"},
		{"no repo falls back to the launch directory name", func(in *statusLineInput) {
			in.Workspace.ProjectDir = "/w/scratch"
			in.Workspace.CurrentDir = "/w/scratch"
		}, "scratch"},
		{"no workspace at all falls back to cwd", func(in *statusLineInput) {
			in.Cwd = "/w/loose"
		}, "loose"},
		{"cwd outside the project keeps the repo name only", func(in *statusLineInput) {
			in.Workspace.Repo = repoName("ccpm")
			in.Workspace.ProjectDir = "/w/ccpm"
			in.Workspace.CurrentDir = "/elsewhere/tmp"
		}, "ccpm"},
		{"nothing known renders nothing", func(in *statusLineInput) {}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in statusLineInput
			tc.set(&in)
			if got := workspaceLabel(in); got != tc.want {
				t.Errorf("workspaceLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// writeGit creates a .git directory containing head.
func writeGit(t *testing.T, dir, head string) {
	t.Helper()
	g := filepath.Join(dir, ".git")
	if err := os.MkdirAll(g, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitBranchAt(t *testing.T) {
	t.Run("branch from a parent directory", func(t *testing.T) {
		repo := t.TempDir()
		writeGit(t, repo, "ref: refs/heads/main\n")
		deep := filepath.Join(repo, "a", "b", "c")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := gitBranchAt(deep); got != "main" {
			t.Errorf("got %q, want main", got)
		}
	})

	t.Run("slashes inside a branch name survive", func(t *testing.T) {
		repo := t.TempDir()
		writeGit(t, repo, "ref: refs/heads/feat/history-tab\n")
		if got := gitBranchAt(repo); got != "feat/history-tab" {
			t.Errorf("got %q, want feat/history-tab", got)
		}
	})

	t.Run("detached HEAD shows a short sha", func(t *testing.T) {
		repo := t.TempDir()
		writeGit(t, repo, "9fceb02d0ae598e95dc970b74767f19372d61af8\n")
		if got := gitBranchAt(repo); got != "9fceb02" {
			t.Errorf("got %q, want 9fceb02", got)
		}
	})

	t.Run("dot-git file points at the real git dir", func(t *testing.T) {
		// How linked worktrees and submodules are laid out.
		real := t.TempDir()
		if err := os.WriteFile(filepath.Join(real, "HEAD"), []byte("ref: refs/heads/wt\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		wt := t.TempDir()
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+real+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := gitBranchAt(wt); got != "wt" {
			t.Errorf("got %q, want wt", got)
		}
	})

	t.Run("no repository yields nothing", func(t *testing.T) {
		if got := gitBranchAt(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
		if got := gitBranchAt(""); got != "" {
			t.Errorf("empty dir got %q, want empty", got)
		}
	})

	t.Run("unreadable HEAD yields nothing rather than erroring", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := gitBranchAt(repo); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestBranchFromHead(t *testing.T) {
	cases := map[string]string{
		"ref: refs/heads/main\n":                     "main",
		"ref: refs/heads/feat/a/b\n":                 "feat/a/b",
		"  ref:   refs/heads/spaced  \n":             "spaced",
		"9fceb02d0ae598e95dc970b74767f19372d61af8\n": "9fceb02",
		"ref: refs/tags/v1\n":                        "v1",
		"":                                           "",
		"garbage\n":                                  "",
	}
	for head, want := range cases {
		if got := branchFromHead(head); got != want {
			t.Errorf("branchFromHead(%q) = %q, want %q", head, got, want)
		}
	}
}

// TestStatusLineBranchPrefersWorktreePayload verifies the payload's own branch
// wins over reading disk — inside a Claude Code worktree the checked-out branch
// is the authoritative one.
func TestStatusLineBranchPrefersWorktreePayload(t *testing.T) {
	repo := t.TempDir()
	writeGit(t, repo, "ref: refs/heads/on-disk\n")
	var in statusLineInput
	in.Workspace.CurrentDir = repo
	in.Worktree = &struct {
		Branch string `json:"branch"`
	}{Branch: "worktree-my-feature"}
	if got := statusLineBranch(in); got != "worktree-my-feature" {
		t.Errorf("got %q, want the worktree branch", got)
	}
}

// TestRenderStatusLineColorized verifies that enabling color wraps segments in
// ANSI codes (profile, orange window label, headroom-coloured percent) and
// always resets, while plain mode emits none of it.
func TestRenderStatusLineColorized(t *testing.T) {
	var in statusLineInput
	in.Model.DisplayName = "Opus 4.8"
	in.RateLimits = &struct {
		FiveHour *rateWindow `json:"five_hour"`
		SevenDay *rateWindow `json:"seven_day"`
	}{FiveHour: &rateWindow{UsedPercentage: 90}} // 10% left → red percent

	got := strings.Join(renderStatusLine(in, "work", fixedNow, true, statusline.Default()), "\n")
	for _, want := range []string{cProfile, cOrange, cRed, cReset} {
		if !strings.Contains(got, want) {
			t.Fatalf("colored output missing %q\n got %q", want, got)
		}
	}
	plain := strings.Join(renderStatusLine(in, "work", fixedNow, false, statusline.Default()), "\n")
	if strings.Contains(plain, "\033") {
		t.Fatalf("plain output unexpectedly contains ANSI: %q", plain)
	}
}

// TestFormatWindowPastResetDropsClock verifies a window whose reset is already
// in the past renders the percent without a stale clock.
func TestFormatWindowPastResetDropsClock(t *testing.T) {
	w := &rateWindow{UsedPercentage: 90, ResetsAt: fixedNow.Add(-time.Hour).Unix()}
	got := formatWindow("5h", w, fixedNow, false)
	if got != "5h 90%" { // percent USED, no stale clock
		t.Fatalf("got %q, want %q", got, "5h 90%")
	}
}

// TestResetClockDatesTheRenewalWhenNotToday guards the seven-day window's
// clock. A bare "08:25" for a reset four days out reads as this morning, and a
// bare weekday still leaves you counting forward from today to work out the
// date it renews on.
func TestResetClockDatesTheRenewalWhenNotToday(t *testing.T) {
	cases := []struct {
		name  string
		reset time.Time
		want  string
	}{
		{"later today stays a plain clock", fixedNow.Add(2 * time.Hour), "16:00"},
		{"tomorrow gets a date", fixedNow.Add(26 * time.Hour), "Fri 26 Jun 16:00"},
		{"a few days out gets a date", fixedNow.Add(3 * 24 * time.Hour), "Sun 28 Jun 14:00"},
		{"beyond a week gets a date", fixedNow.Add(9 * 24 * time.Hour), "Sat 4 Jul 14:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resetClock(tc.reset, fixedNow); got != tc.want {
				t.Errorf("resetClock = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestWorkspaceLabelIsAlwaysForwardSlashed pins the label's separator. It is a
// display string in the shape "repo/subdir", so it must read identically on
// every OS; joining with filepath.Separator rendered repo\sub on Windows and
// broke this on the CI leg that catches exactly this class of thing.
func TestWorkspaceLabelIsAlwaysForwardSlashed(t *testing.T) {
	var in statusLineInput
	in.Workspace.ProjectDir = filepath.Join("w", "ccpm")
	in.Workspace.CurrentDir = filepath.Join("w", "ccpm", "desktop", "frontend")
	got := workspaceLabel(in)
	if strings.Contains(got, `\`) {
		t.Errorf("label contains a backslash: %q", got)
	}
	if got != "ccpm/desktop/frontend" {
		t.Errorf("got %q, want ccpm/desktop/frontend", got)
	}
}

// TestBranchFromHeadRejectsTerminalEscapes is the regression for an escape
// injection. .git/HEAD is read off disk and printed into a terminal that
// renders ANSI, on every assistant message — and findGitEntry follows a `gitdir:`
// pointer to an arbitrary path, so the HEAD need not even be in the repo. A
// crafted branch name could retitle the window, clear lines, or repaint the rows
// above, which is where permission prompts live.
func TestBranchFromHeadRejectsTerminalEscapes(t *testing.T) {
	hostile := map[string]string{
		"OSC title set":    "ref: refs/heads/main\x1b]0;OWNED\x07",
		"CSI erase + home": "ref: refs/heads/main\x1b[2K\x1b[1G$ ",
		"single-byte CSI":  "ref: refs/heads/main\x9b2K",
		"embedded newline": "ref: refs/heads/main\nFAKE ROW",
		"carriage return":  "ref: refs/heads/main\roverwrite",
		"DEL":              "ref: refs/heads/main\x7f",
		"NUL":              "ref: refs/heads/main\x00",
		"absurdly long":    "ref: refs/heads/" + strings.Repeat("x", 5000),
		"escapes via base": "ref: refs/tags/v1\x1b[31m",
	}
	for name, head := range hostile {
		t.Run(name, func(t *testing.T) {
			if got := branchFromHead(head); got != "" {
				t.Errorf("accepted a hostile HEAD: %q", got)
			}
		})
	}
	// Legitimate names must still come through, including slashes and unicode.
	for head, want := range map[string]string{
		"ref: refs/heads/main\n":             "main",
		"ref: refs/heads/feat/history-tab\n": "feat/history-tab",
		"ref: refs/heads/fix-café\n":         "fix-café",
	} {
		if got := branchFromHead(head); got != want {
			t.Errorf("branchFromHead(%q) = %q, want %q", head, got, want)
		}
	}
}

// TestEverySegmentRejectsTerminalEscapes closes the hole the per-label guards
// left. safeLabel covered the branch and the workspace, but the profile name
// (read from ~/.ccpm/config.json) and the model and effort strings (read from
// the status JSON) reached the terminal untouched — verified before the fix, an
// OSC title-set and a CSI erase both came out raw.
//
// It matters because Claude Code renders this with ANSI interpreted on every
// assistant message, immediately above where permission prompts are drawn. The
// guard now lives in paint, so this walks every segment rather than the two
// that happened to be hardened.
func TestEverySegmentRejectsTerminalEscapes(t *testing.T) {
	hostile := map[string]string{
		"OSC title set":    "\x1b]0;OWNED\x07",
		"CSI erase + home": "\x1b[2K\x1b[1G",
		"single-byte CSI":  "\x9b2K",
		"newline":          "\nFAKE ROW",
		"carriage return":  "\roverwrite",
		"DEL":              "\x7f",
		"NUL":              "\x00",
	}

	for name, evil := range hostile {
		t.Run(name, func(t *testing.T) {
			// Feed the hostile string through every free-form input at once.
			var in statusLineInput
			in.Model.DisplayName = "Opus" + evil
			in.Effort = &struct {
				Level string `json:"level"`
			}{Level: "high" + evil}
			in.Workspace.Repo = &struct {
				Host  string `json:"host"`
				Owner string `json:"owner"`
				Name  string `json:"name"`
			}{Name: "repo" + evil}
			in.Worktree = &struct {
				Branch string `json:"branch"`
			}{Branch: "main" + evil}
			in.ContextWindow.UsedPercentage = 34

			rows := renderStatusLine(in, "work"+evil, fixedNow, false, statusline.Default())
			for _, row := range rows {
				for i := 0; i < len(row); i++ {
					if b := row[i]; b < 0x20 || b == 0x7f || b == 0x9b {
						t.Fatalf("segment leaked control byte %#x: %q", b, row)
					}
				}
			}
			// The clean segment must survive — rejecting a poisoned segment
			// must not take the whole status line down with it.
			joined := strings.Join(rows, "|")
			if !strings.Contains(joined, "ctx 34%") {
				t.Errorf("a hostile neighbour removed the clean segment too: %q", rows)
			}
		})
	}
}

// TestPaintRejectsControlCharacters pins the funnel directly, so a future
// segment that forgets its own guard is still covered.
func TestPaintRejectsControlCharacters(t *testing.T) {
	if got := paint(false, cModel, "clean"); got != "clean" {
		t.Errorf("paint mangled a clean string: %q", got)
	}
	if got := paint(true, cModel, "clean"); !strings.Contains(got, "clean") {
		t.Errorf("colored paint dropped a clean string: %q", got)
	}
	// Unicode the status line actually uses must pass.
	for _, ok := range []string{"⬢ work", "⎇ feat/x", "↺16:15", "$1.23", "café"} {
		if paint(false, cModel, ok) != ok {
			t.Errorf("paint rejected legitimate text %q", ok)
		}
	}
	for _, bad := range []string{"a\x1bb", "a\x00b", "a\nb", "a\x7fb", "a\x9bb", "a\xffb"} {
		if got := paint(false, cModel, bad); got != "" {
			t.Errorf("paint(%q) = %q, want it dropped", bad, got)
		}
	}
}

// TestRenderStatusLineNeverEmitsEscapesFromDisk is the end-to-end guard: no
// matter what a crafted repo puts on disk, plain mode must contain no ESC.
func TestRenderStatusLineNeverEmitsEscapesFromDisk(t *testing.T) {
	repo := t.TempDir()
	writeGit(t, repo, "ref: refs/heads/main\x1b]0;OWNED\x07\x1b[2K")
	var in statusLineInput
	in.Workspace.ProjectDir = repo
	in.Workspace.CurrentDir = repo
	in.Model.DisplayName = "Opus 5"

	// Byte-wise, not strings.ContainsAny: 0x9b alone is not valid UTF-8, and
	// ContainsAny requires valid UTF-8 in both arguments.
	for _, row := range renderStatusLine(in, "work", fixedNow, false, statusline.Default()) {
		for i := 0; i < len(row); i++ {
			if b := row[i]; b < 0x20 || b == 0x7f || b == 0x9b {
				t.Errorf("row leaked control byte %#x: %q", b, row)
				break
			}
		}
	}
}
