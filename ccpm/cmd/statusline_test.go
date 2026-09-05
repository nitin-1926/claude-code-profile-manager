package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedNow is a deterministic clock for reset-time formatting.
var fixedNow = time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)

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
		want    []string
	}{
		{
			name:    "subscription splits identity from usage",
			in:      subscription(),
			profile: "work",
			want: []string{
				"⬢ work",
				// Windows show percent USED (matching Claude's /usage), not remaining.
				"Sonnet 4.6 · ctx 34% · effort high · 5h 42% ↺" + wantReset + " · 7d 12% · $1.23",
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
			want:    []string{"⬢ personal", "Opus 4.8 · $0.12"},
		},
		{
			name: "falls back to model id when no display name",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.ID = "claude-opus-4-8"
				return in
			}(),
			profile: "work",
			want:    []string{"⬢ work", "claude-opus-4-8"},
		},
		{
			name:    "nothing to show prints nothing at all",
			in:      statusLineInput{},
			profile: "",
			want:    []string{},
		},
		{
			name: "usage row alone when no profile or workspace resolves",
			in: func() statusLineInput {
				var in statusLineInput
				in.Model.DisplayName = "Haiku 4.5"
				return in
			}(),
			profile: "",
			want:    []string{"Haiku 4.5"},
		},
		{
			name: "identity row alone when there is no usage yet",
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
			want:    []string{"⬢ ci", "Haiku 4.5 · ctx 12%"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// color=false keeps the assertions on plain text; coloring is
			// covered separately in TestRenderStatusLineColorized.
			got := renderStatusLine(tc.in, tc.profile, fixedNow, false)
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

// TestRenderStatusLineWorkspaceRow covers the identity row's repo/directory and
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

	rows := renderStatusLine(in, "work", fixedNow, false)
	if len(rows) != 1 {
		t.Fatalf("want only the identity row, got %q", rows)
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

	got := strings.Join(renderStatusLine(in, "work", fixedNow, true), "\n")
	for _, want := range []string{cProfile, cOrange, cRed, cReset} {
		if !strings.Contains(got, want) {
			t.Fatalf("colored output missing %q\n got %q", want, got)
		}
	}
	plain := strings.Join(renderStatusLine(in, "work", fixedNow, false), "\n")
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

// TestResetClockNamesTheDayWhenNotToday guards the seven-day window's clock: a
// bare "08:25" for a reset two days out reads as this morning.
func TestResetClockNamesTheDayWhenNotToday(t *testing.T) {
	cases := []struct {
		name  string
		reset time.Time
		want  string
	}{
		{"later today stays a plain clock", fixedNow.Add(2 * time.Hour), "16:00"},
		{"tomorrow names the weekday", fixedNow.Add(26 * time.Hour), "Fri 16:00"},
		{"a few days out names the weekday", fixedNow.Add(3 * 24 * time.Hour), "Sun 14:00"},
		{"beyond a week gives a date", fixedNow.Add(9 * 24 * time.Hour), "4 Jul 14:00"},
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
