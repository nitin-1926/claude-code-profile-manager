// Package trust manages the list of project directories whose .claude/settings.json
// is allowed to contribute keys beyond a small allowlist of inert ones (hooks,
// permissions, apiKeyHelper and the other command helpers, env, …) to the
// profile merge.
//
// A cloned git repo can drop a .claude/settings.json with arbitrary hooks or
// command helpers; merging those silently would mean `git clone + ccpm run`
// is enough for an attacker-controlled repo to register shell commands. ccpm
// therefore treats every project as untrusted by default: every key not on
// SafeProjectKeys is stripped, and the user is told how to opt in. An explicit `ccpm trust add
// <path>` is required to let a project's settings contribute those keys.
package trust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

const trustFilename = "trusted-projects.json"

// SafeProjectKeys is the allowlist of top-level settings keys an UNTRUSTED
// project's settings.json / settings.local.json may contribute: display,
// editor and model preferences that can't run a command, widen permissions,
// load code or redirect credentials. Everything else is stripped. It is an
// allowlist, not a blocklist, because Claude Code keeps adding keys that
// execute a shell command (apiKeyHelper, awsAuthRefresh, awsCredentialExport,
// gcpAuthRefresh, otelHeadersHelper, fileSuggestion, subagentStatusLine,
// processWrapper, …) and a blocklist lets each new one through until someone
// notices. Add a key here only after checking Claude Code's settings reference
// shows it is inert.
var SafeProjectKeys = map[string]bool{
	"$schema":                    true,
	"alwaysThinkingEnabled":      true,
	"attribution":                true,
	"autoCompactEnabled":         true,
	"cleanupPeriodDays":          true,
	"companyAnnouncements":       true,
	"editorMode":                 true,
	"effortLevel":                true,
	"emojiCompletionEnabled":     true,
	"fastMode":                   true,
	"includeCoAuthoredBy":        true,
	"includeGitInstructions":     true,
	"keybindingFlavor":           true,
	"language":                   true,
	"maxProseWidth":              true,
	"model":                      true,
	"outputStyle":                true,
	"prefersReducedMotion":       true,
	"promptSuggestionEnabled":    true,
	"respectGitignore":           true,
	"showThinkingSummaries":      true,
	"showTurnDuration":           true,
	"spinnerTipsEnabled":         true,
	"spinnerVerbs":               true,
	"syntaxHighlightingDisabled": true,
	"teammateMode":               true,
	"terminalProgressBarEnabled": true,
	"theme":                      true,
	"timeFormat":                 true,
	"timeZone":                   true,
	"verbose":                    true,
	"viewMode":                   true,
}

// Record is one entry in the trust list.
type Record struct {
	Path      string `json:"path"`
	GrantedAt string `json:"granted_at"`
}

// List is the on-disk shape of the trusted-projects file.
type List struct {
	Version  string   `json:"version"`
	Projects []Record `json:"projects"`
}

const listVersion = "1"

// listPath returns the on-disk location of trusted-projects.json.
func listPath() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, trustFilename), nil
}

// Load reads the trust list from disk. Missing file returns an empty list.
func Load() (*List, error) {
	path, err := listPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &List{Version: listVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading trust list: %w", err)
	}
	var l List
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing trust list: %w", err)
	}
	if l.Version == "" {
		l.Version = listVersion
	}
	return &l, nil
}

// Save writes the trust list atomically with 0600 perms — the list discloses
// which project directories the user has granted shell-exec consent to, so
// we keep it readable only by the invoking user.
func Save(l *List) error {
	path, err := listPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating ccpm base directory: %w", err)
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling trust list: %w", err)
	}
	if err := atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(path, data, config.FilePerm),
	}); err != nil {
		return fmt.Errorf("saving trust list: %w", err)
	}
	return nil
}

// canonicalPath resolves p to an absolute, symlink-free form. Trust grants
// and checks must compare canonical paths: comparing plain Abs() output lets
// a symlink at a trusted location be re-pointed at a hostile directory (or a
// symlinked cwd spoof a trusted path) without invalidating the grant. Falls
// back to the absolute form when the path doesn't exist.
func canonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

// IsTrusted reports whether projectRoot appears in the trust list. An empty
// projectRoot (no project context) is treated as not-applicable and returns
// true so the caller doesn't unnecessarily strip keys that aren't there.
func IsTrusted(projectRoot string) bool {
	if projectRoot == "" {
		return true
	}
	abs, err := canonicalPath(projectRoot)
	if err != nil {
		return false
	}
	l, err := Load()
	if err != nil {
		return false
	}
	// Stored paths are canonical at grant time and deliberately NOT
	// re-resolved here: if the granted directory has since been replaced by a
	// symlink, resolving the stored entry would follow it and re-grant trust
	// to wherever it now points.
	for _, r := range l.Projects {
		if r.Path == abs {
			return true
		}
	}
	return false
}

// MarkTrusted adds projectRoot to the trust list. Idempotent.
func MarkTrusted(projectRoot string) error {
	abs, err := canonicalPath(projectRoot)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", projectRoot, err)
	}
	l, err := Load()
	if err != nil {
		return err
	}
	for _, r := range l.Projects {
		if r.Path == abs {
			return nil
		}
	}
	l.Projects = append(l.Projects, Record{
		Path:      abs,
		GrantedAt: time.Now().UTC().Format(time.RFC3339),
	})
	sort.SliceStable(l.Projects, func(i, j int) bool { return l.Projects[i].Path < l.Projects[j].Path })
	return Save(l)
}

// Forget removes projectRoot from the trust list. Returns true if a record
// was actually removed.
func Forget(projectRoot string) (bool, error) {
	canon, err := canonicalPath(projectRoot)
	if err != nil {
		return false, fmt.Errorf("resolving %q: %w", projectRoot, err)
	}
	// Revoking trust should be easy: match the canonical form and the plain
	// absolute form so entries granted before canonicalization landed are
	// still removable.
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return false, fmt.Errorf("resolving %q: %w", projectRoot, err)
	}
	l, err := Load()
	if err != nil {
		return false, err
	}
	var kept []Record
	removed := false
	for _, r := range l.Projects {
		if r.Path == canon || r.Path == abs {
			removed = true
			continue
		}
		kept = append(kept, r)
	}
	if !removed {
		return false, nil
	}
	l.Projects = kept
	return true, Save(l)
}

// All returns the trust list entries.
func All() ([]Record, error) {
	l, err := Load()
	if err != nil {
		return nil, err
	}
	return l.Projects, nil
}

// FilterProjectLayer returns a copy of settings keeping only SafeProjectKeys
// when the project is untrusted. stripped reports (sorted) which keys were
// dropped so the caller can log them once.
func FilterProjectLayer(settings map[string]interface{}, projectRoot string) (filtered map[string]interface{}, stripped []string) {
	if IsTrusted(projectRoot) {
		return settings, nil
	}
	out := make(map[string]interface{}, len(settings))
	for k, v := range settings {
		if !SafeProjectKeys[k] {
			stripped = append(stripped, k)
			continue
		}
		out[k] = v
	}
	sort.Strings(stripped)
	return out, stripped
}

// dangerousEnvNames are env keys a project layer must never set, trusted or
// not: each one redirects code execution (loader paths, interpreter startup
// files, node flags) and turns a one-time trust grant into a persistent
// arbitrary-exec channel for anyone who can commit to the repo.
var dangerousEnvNames = map[string]bool{
	"PATH":          true,
	"NODE_OPTIONS":  true,
	"BASH_ENV":      true,
	"ENV":           true,
	"PYTHONSTARTUP": true,
	"PYTHONPATH":    true,
	"PERL5LIB":      true,
	"RUBYOPT":       true,
}

var dangerousEnvPrefixes = []string{"LD_", "DYLD_"}

func isDangerousEnvName(name string) bool {
	if dangerousEnvNames[name] {
		return true
	}
	for _, p := range dangerousEnvPrefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

// FilterEnvAlways strips dangerous variable names from a project layer's
// "env" map regardless of trust state. Returns the (possibly shallow-copied)
// settings and the env names removed. Settings without an env map pass
// through untouched.
func FilterEnvAlways(settings map[string]interface{}) (map[string]interface{}, []string) {
	rawEnv, ok := settings["env"].(map[string]interface{})
	if !ok || len(rawEnv) == 0 {
		return settings, nil
	}
	var stripped []string
	cleanEnv := make(map[string]interface{}, len(rawEnv))
	for k, v := range rawEnv {
		if isDangerousEnvName(k) {
			stripped = append(stripped, k)
			continue
		}
		cleanEnv[k] = v
	}
	if len(stripped) == 0 {
		return settings, nil
	}
	sort.Strings(stripped)
	out := make(map[string]interface{}, len(settings))
	for k, v := range settings {
		out[k] = v
	}
	if len(cleanEnv) == 0 {
		delete(out, "env")
	} else {
		out["env"] = cleanEnv
	}
	return out, stripped
}

// WarnStrippedEnv prints a one-time warning when dangerous env vars were
// dropped from a trusted project's settings layer.
func WarnStrippedEnv(projectRoot string, stripped []string) {
	if projectRoot == "" || len(stripped) == 0 {
		return
	}
	key := "env:" + projectRoot
	if _, already := warnedOnce.LoadOrStore(key, struct{}{}); already {
		return
	}
	fmt.Fprintf(os.Stderr, "Note: dropped unsafe env vars %v from project %q settings — PATH/loader/interpreter overrides are never applied from project layers.\n", stripped, projectRoot)
}

// warnedOnce prevents the "stripped dangerous keys" warning from firing on
// every Materialize call in a single process (e.g. if a caller materializes
// settings then MCP separately).
var (
	warnedOnce sync.Map // key: projectRoot, value: struct{}
)

// WarnUntrusted prints a one-time warning to stderr describing which keys were
// dropped from the project layer because the project is not in the trust list.
// No-op if projectRoot is empty or if the warning has already fired for this
// projectRoot in the current process.
func WarnUntrusted(projectRoot string, stripped []string) {
	if projectRoot == "" || len(stripped) == 0 {
		return
	}
	if _, already := warnedOnce.LoadOrStore(projectRoot, struct{}{}); already {
		return
	}
	// The project layer only feeds ccpm's effective-settings view (it is never
	// written into a profile); Claude Code applies project settings itself.
	fmt.Fprintf(os.Stderr, "Note: project %q is not trusted — ccpm's view leaves out %v from its .claude/settings.json. Run `ccpm trust add %q` to include them.\n", projectRoot, stripped, projectRoot)
}
