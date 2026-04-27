package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

var sessionsAll bool

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

	var targetSubdir string
	if !sessionsAll {
		cwd, err := os.Getwd()
		if err == nil {
			targetSubdir = encodeCwdForClaude(cwd)
		}
	}

	var sessions []sessionRecord
	walkErr := filepath.WalkDir(projectsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// When not in --all mode, prune other project subdirs.
			if !sessionsAll && targetSubdir != "" {
				rel, _ := filepath.Rel(projectsRoot, path)
				if rel != "." && rel != targetSubdir && !strings.HasPrefix(rel, targetSubdir+string(filepath.Separator)) {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		rec, rerr := readSessionHeader(path)
		if rerr != nil {
			// Skip unreadable files silently — native claude may be mid-write.
			return nil
		}
		sessions = append(sessions, rec)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("scanning sessions: %w", walkErr)
	}

	if len(sessions) == 0 {
		if sessionsAll {
			fmt.Printf("No sessions found for profile %q.\n", profileName)
		} else {
			fmt.Printf("No sessions found for profile %q in the current project. Use --all to list every project.\n", profileName)
		}
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ModTime.After(sessions[j].ModTime) })

	bold := color.New(color.Bold).SprintFunc()
	fmt.Printf("  %-36s %-19s %-40s %s\n", bold("SESSION ID"), bold("STARTED"), bold("PROJECT"), bold("FIRST PROMPT"))
	fmt.Printf("  %s\n", strings.Repeat("─", 110))
	for _, s := range sessions {
		started := s.ModTime.Local().Format("2006-01-02 15:04:05")
		project := truncate(s.Cwd, 40)
		firstPrompt := truncate(s.FirstPrompt, 60)
		fmt.Printf("  %-36s %-19s %-40s %s\n", s.SessionID, started, project, firstPrompt)
	}
	return nil
}

// sessionRecord is the minimal shape we surface. Native claude's .jsonl files
// hold many more fields per line; we only peek the first line for ID, cwd,
// and the first user prompt if it's nearby.
type sessionRecord struct {
	SessionID   string
	Cwd         string
	FirstPrompt string
	ModTime     time.Time
}

// readSessionHeader opens <path>, reads the first ~8 lines, and fishes out a
// session ID, cwd, and the first user prompt. Limits how many lines we read so
// a long session doesn't become an O(file) operation here.
func readSessionHeader(path string) (sessionRecord, error) {
	info, err := os.Stat(path)
	if err != nil {
		return sessionRecord{}, err
	}
	rec := sessionRecord{
		SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		ModTime:   info.ModTime(),
	}

	file, err := os.Open(path)
	if err != nil {
		return rec, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 12 {
			break
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if id, ok := entry["sessionId"].(string); ok && rec.SessionID == "" {
			rec.SessionID = id
		}
		if cwd, ok := entry["cwd"].(string); ok && rec.Cwd == "" {
			rec.Cwd = cwd
		}
		if rec.FirstPrompt == "" {
			if prompt := extractUserPrompt(entry); prompt != "" {
				rec.FirstPrompt = prompt
			}
		}
	}
	return rec, nil
}

// extractUserPrompt pulls a human-readable preview from a session line. The
// shape varies across Claude Code versions, so we probe a few known spots.
func extractUserPrompt(entry map[string]interface{}) string {
	if role, _ := entry["role"].(string); role != "user" && entry["role"] != nil {
		return ""
	}
	if s, ok := entry["content"].(string); ok {
		return strings.TrimSpace(s)
	}
	// Claude Code v2 stores messages under entry["message"]["content"] as a
	// list of typed blocks. Grab the first "text" block.
	if msg, ok := entry["message"].(map[string]interface{}); ok {
		if role, _ := msg["role"].(string); role != "user" && msg["role"] != nil {
			return ""
		}
		switch content := msg["content"].(type) {
		case string:
			return strings.TrimSpace(content)
		case []interface{}:
			for _, blk := range content {
				if bm, ok := blk.(map[string]interface{}); ok {
					if t, _ := bm["type"].(string); t == "text" {
						if text, ok := bm["text"].(string); ok {
							return strings.TrimSpace(text)
						}
					}
				}
			}
		}
	}
	return ""
}

// encodeCwdForClaude mirrors native Claude Code's cwd encoding used in
// <profileDir>/projects/<encoded>/ — every run of non-alphanumeric characters
// (and leading separators) turns into a single "-". Mirrors ~/.claude/projects
// conventions.
func encodeCwdForClaude(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	prevDash := false
	for _, r := range cwd {
		if isSessionAlnum(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := b.String()
	return strings.Trim(out, "-")
}

func isSessionAlnum(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
