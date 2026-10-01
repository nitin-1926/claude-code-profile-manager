package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/transcript"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

var (
	sessionsAll   bool
	sessionsJSON  bool
	sessionsLimit int
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Inspect Claude Code sessions stored inside a profile",
}

var sessionsListCmd = &cobra.Command{
	Use:   "list <profile>",
	Short: "List Claude Code sessions for a profile",
	Long: `List sessions Claude Code has stored inside a profile directory.

By default ccpm only shows sessions whose cwd matches the current working
directory — that matches how native ` + "`claude --resume`" + ` scopes its picker. Use
--all to surface sessions from every project the profile has worked on.

Session metadata is read from <profileDir>/projects/<encoded-cwd>/*.jsonl, the
same files native Claude Code writes; ccpm does not mutate them.`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsList,
}

func init() {
	sessionsListCmd.Flags().BoolVar(&sessionsAll, "all", false, "list sessions across every project in this profile")
	sessionsListCmd.Flags().BoolVar(&sessionsJSON, "json", false, "machine-readable JSON output")
	sessionsListCmd.Flags().IntVar(&sessionsLimit, "limit", 20, "show at most N most-recent sessions (0 = no limit)")

	sessionsCmd.AddCommand(sessionsListCmd)
	rootCmd.AddCommand(sessionsCmd)
}

func runSessionsList(cmd *cobra.Command, args []string) error {
	profileName := args[0]
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	p, exists := cfg.Profiles[profileName]
	if !exists {
		return fmt.Errorf("profile %q not found", profileName)
	}

	projectsRoot := filepath.Join(p.Dir, "projects")
	info, err := os.Stat(projectsRoot)
	if err != nil || !info.IsDir() {
		fmt.Printf("No sessions found for profile %q (no %s).\n", profileName, projectsRoot)
		return nil
	}

	// The session index the desktop History tab uses: one entry per real
	// session (subagent transcripts fold into their parent instead of becoming
	// rows), titled by Claude Code's ai-title or the first real prompt. It is
	// the same incremental history.json sidecar the desktop writes; a failed
	// save still returns a usable index, so a read-only profile still lists.
	ix, _ := transcript.BuildIndex(p.Dir)

	// Claude Code names the directory after the PHYSICAL cwd (/private/tmp),
	// while os.Getwd returns the logical $PWD (/tmp) — accept either.
	var dirs map[string]bool
	if !sessionsAll {
		if cwd, err := os.Getwd(); err == nil {
			dirs = map[string]bool{encodeCwdForClaude(cwd): true}
			if phys, err := filepath.EvalSymlinks(cwd); err == nil {
				dirs[encodeCwdForClaude(phys)] = true
			}
		}
	}

	var sessions []*transcript.Entry
	for _, e := range ix.Entries {
		if dirs != nil {
			dir, _, _ := strings.Cut(e.RelPath, "/")
			if !dirs[dir] {
				continue
			}
		}
		// The index keeps sessions whose transcript Claude Code has pruned;
		// those cannot be resumed, so they are not listed here.
		if fi, err := os.Lstat(filepath.Join(projectsRoot, filepath.FromSlash(e.RelPath))); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		sessions = append(sessions, e)
	}

	if len(sessions) == 0 {
		if sessionsJSON {
			fmt.Println("[]")
			return nil
		}
		if sessionsAll {
			fmt.Printf("No sessions found for profile %q.\n", profileName)
		} else {
			fmt.Printf("No sessions found for profile %q in the current project. Use --all to list every project.\n", profileName)
		}
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].ModTime != sessions[j].ModTime {
			return sessions[i].ModTime > sessions[j].ModTime
		}
		return sessions[i].SessionID < sessions[j].SessionID
	})
	total := len(sessions)
	if sessionsLimit > 0 && total > sessionsLimit {
		sessions = sessions[:sessionsLimit]
	}

	if sessionsJSON {
		type sessionJSON struct {
			SessionID   string `json:"session_id"`
			Started     string `json:"started"`
			LastActive  string `json:"last_active"`
			Cwd         string `json:"cwd,omitempty"`
			FirstPrompt string `json:"first_prompt,omitempty"`
		}
		out := make([]sessionJSON, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionJSON{
				SessionID:   s.SessionID,
				Started:     sessionStarted(s).UTC().Format(time.RFC3339),
				LastActive:  time.Unix(s.ModTime, 0).UTC().Format(time.RFC3339),
				Cwd:         s.Cwd,
				FirstPrompt: s.Title,
			})
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	bold := color.New(color.Bold).SprintFunc()
	// The column is the sort key: rows are most-recently-active first (what
	// `claude --resume` offers), so a first-timestamp column here read as
	// out of order whenever an old session had been resumed recently.
	fmt.Printf("  %-36s %-19s %-40s %s\n", bold("SESSION ID"), bold("LAST ACTIVE"), bold("PROJECT"), bold("FIRST PROMPT"))
	fmt.Printf("  %s\n", strings.Repeat("─", 110))
	for _, s := range sessions {
		active := time.Unix(s.ModTime, 0).Local().Format("2006-01-02 15:04:05")
		fmt.Printf("  %-36s %-19s %-40s %s\n",
			terminalSafe(s.SessionID), active, truncate(terminalSafe(s.Cwd), 40), truncate(terminalSafe(s.Title), 60))
	}
	if shown := len(sessions); shown < total {
		color.New(color.Faint).Printf("  (showing %d of %d — use --limit 0 for all)\n", shown, total)
	}
	return nil
}

// sessionStarted is the session's first transcript timestamp, falling back to
// the transcript's mtime for a file with no timestamped line.
func sessionStarted(e *transcript.Entry) time.Time {
	if t, err := time.Parse(time.RFC3339, e.FirstTS); err == nil {
		return t
	}
	return time.Unix(e.ModTime, 0)
}

// terminalSafe makes transcript-derived text safe to print into a terminal.
// Titles and cwds come from transcript content, which can carry anything a
// pasted log or fetched page did: an OSC 52 sequence writes the clipboard, a
// CSI one repaints the screen. Line breaks and tabs become spaces so a row stays
// one row; every other C0, DEL, and C1 control is dropped. (Invalid UTF-8 — a
// raw single-byte 0x9b CSI — cannot arrive here: these strings come out of
// encoding/json, which replaces invalid bytes with U+FFFD.)
func terminalSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			return -1
		}
		return r
	}, s)
}

// encodeCwdForClaude mirrors native Claude Code's cwd encoding used in
// <profileDir>/projects/<encoded>/. It delegates to usage.EncodeCwd so the
// `sessions` and `usage` commands share one encoder and can never drift.
func encodeCwdForClaude(cwd string) string {
	return usage.EncodeCwd(cwd)
}

// truncate clips s to at most max runes, marking a cut with "…". It counts
// runes, not bytes: a byte cut split multi-byte characters into invalid UTF-8.
func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max <= 1 {
		return transcript.ClipRunes(s, max)
	}
	return transcript.ClipRunes(s, max-1) + "…"
}
