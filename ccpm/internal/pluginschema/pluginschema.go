// Package pluginschema holds the on-disk JSON shapes of Claude Code's plugin
// state files (installed_plugins.json v2). Both internal/plugins (which
// writes per-profile state) and internal/sync (which scans the host's
// ~/.claude/plugins) consume these — keeping the shapes here removes the
// duplicated local types sync previously declared to dodge a build cycle.
package pluginschema

// InstalledEntry is one record in installed_plugins.json's per-plugin list,
// mirroring what Claude Code itself writes.
type InstalledEntry struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

// InstalledDoc is the top-level shape of installed_plugins.json (v2 schema:
// plugin id "<name>@<marketplace>" → entry list).
type InstalledDoc struct {
	Version int                         `json:"version"`
	Plugins map[string][]InstalledEntry `json:"plugins"`
}
