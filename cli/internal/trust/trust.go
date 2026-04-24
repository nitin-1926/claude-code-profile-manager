// Package trust manages the list of project directories whose .claude/settings.json
// is allowed to contribute security-sensitive keys (hooks, permissions,
// statusLine, mcpServers, env, enabledPlugins) to the profile merge.
//
// A cloned git repo can drop a .claude/settings.json with arbitrary hooks or
// permission overrides; merging those silently would mean `git clone + ccpm run`
// is enough for an attacker-controlled repo to register shell commands. ccpm
// therefore treats every project as untrusted by default: dangerous keys are
// stripped, and the user is told how to opt in. An explicit `ccpm trust add
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

	"github.com/nitin-1926/ccpm/internal/config"
)

const trustFilename = "trusted-projects.json"

// DangerousKeys lists the top-level keys in a project's settings.json /
// settings.local.json / .mcp.json that can grant shell access or bypass safety
// rails. Project-scoped writes of these keys are dropped from the merge unless
// the project is in the trust list.
var DangerousKeys = []string{"hooks", "permissions", "statusLine", "mcpServers", "env", "enabledPlugins"}

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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing trust list: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving trust list: %w", err)
	}
	return nil
}

// IsTrusted reports whether projectRoot appears in the trust list. An empty
// projectRoot (no project context) is treated as not-applicable and returns
// true so the caller doesn't unnecessarily strip keys that aren't there.
func IsTrusted(projectRoot string) bool {
	if projectRoot == "" {
		return true
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return false
	}
	l, err := Load()
