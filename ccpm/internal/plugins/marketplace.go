package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MarketplaceManifest is the shape of <marketplace>/.claude-plugin/marketplace.json.
// Only the fields ccpm needs to install plugins are decoded; everything else
// (icons, categories, descriptions) is preserved as raw json on the plugin
// entry but unused for now.
type MarketplaceManifest struct {
	Name    string                  `json:"name"`
	Plugins []MarketplacePluginSpec `json:"plugins"`
}

// MarketplacePluginSpec is one entry in the marketplace's plugin list. The
// "source" key in the JSON can be either a string (relative path inside the
// marketplace repo) or an object — we decode it into RawSource and resolve
// later.
type MarketplacePluginSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	RawSource   json.RawMessage `json:"source"`
}

// PluginSource is the resolved form of a marketplace.json "source" value.
type PluginSource struct {
	// Kind is one of:
	//
	//   "local"     — Path is relative to the marketplace clone root.
	//   "github"    — Repo is "<org>/<repo>", optionally with Ref / SHA.
	//   "git-subdir"— URL is a git URL; Path is a subdirectory inside the
	//                 cloned tree; Ref / SHA optional.
	//   "url"       — URL is a git URL; SHA optional.
	Kind string
	Path string
	URL  string
	Repo string
	Ref  string
	SHA  string
}

// ResolveSource decodes the polymorphic "source" field. Returns ("", nil) for
// an empty source, an error for shapes ccpm doesn't yet understand.
func (s MarketplacePluginSpec) ResolveSource() (PluginSource, error) {
	if len(s.RawSource) == 0 {
		return PluginSource{}, fmt.Errorf("plugin %q: missing source", s.Name)
	}
	// String form: "./plugins/<name>" — local path inside marketplace repo.
	var asString string
	if err := json.Unmarshal(s.RawSource, &asString); err == nil {
		src := PluginSource{Kind: "local", Path: strings.TrimPrefix(asString, "./")}
		if err := src.validate(); err != nil {
			return PluginSource{}, fmt.Errorf("plugin %q: %w", s.Name, err)
		}
		return src, nil
	}
	// Object form.
	var obj struct {
		Source string `json:"source"`
		URL    string `json:"url"`
		Path   string `json:"path"`
		Repo   string `json:"repo"`
		Ref    string `json:"ref"`
		SHA    string `json:"sha"`
	}
	if err := json.Unmarshal(s.RawSource, &obj); err != nil {
		return PluginSource{}, fmt.Errorf("plugin %q: parsing source: %w", s.Name, err)
	}
	var src PluginSource
	switch obj.Source {
	case "github":
		src = PluginSource{Kind: "github", Repo: obj.Repo, Ref: obj.Ref, SHA: obj.SHA}
	case "git-subdir":
		src = PluginSource{Kind: "git-subdir", URL: obj.URL, Path: obj.Path, Ref: obj.Ref, SHA: obj.SHA}
	case "url":
		src = PluginSource{Kind: "url", URL: obj.URL, SHA: obj.SHA}
	default:
		return PluginSource{}, fmt.Errorf("plugin %q: unknown source kind %q", s.Name, obj.Source)
	}
	if err := src.validate(); err != nil {
		return PluginSource{}, fmt.Errorf("plugin %q: %w", s.Name, err)
	}
	return src, nil
}

// validate enforces the manifest trust boundary on a resolved source. Every
// field here came from remote JSON and ends up in a path join or git argv.
func (s PluginSource) validate() error {
	switch s.Kind {
	case "local":
		return ValidateLocalPath(s.Path)
	case "github":
		if err := ValidateRepo(s.Repo); err != nil {
			return err
		}
	case "git-subdir":
		if err := ValidateGitURL(s.URL); err != nil {
			return err
		}
		if err := ValidateLocalPath(s.Path); err != nil {
			return err
		}
	case "url":
		if err := ValidateGitURL(s.URL); err != nil {
			return err
		}
	}
	if err := ValidateRef(s.Ref); err != nil {
		return err
	}
	return ValidateSHA(s.SHA)
}

// LoadMarketplaceManifest reads <marketplaceDir>/.claude-plugin/marketplace.json.
func LoadMarketplaceManifest(marketplaceDir string) (*MarketplaceManifest, error) {
	path := filepath.Join(marketplaceDir, ".claude-plugin", "marketplace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m MarketplaceManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}

// FindPluginInManifest returns the plugin entry with the given name, or nil.
func (m *MarketplaceManifest) FindPlugin(name string) *MarketplacePluginSpec {
	for i := range m.Plugins {
		if m.Plugins[i].Name == name {
			return &m.Plugins[i]
		}
	}
	return nil
}

// gitCmd builds a hardened git invocation. Marketplace URLs/refs come from
// remote JSON, so every git child runs with prompts disabled, remote helpers
// and the ext/file transports off, repo-local hooks neutralized, and
// credential helpers disabled — a hostile remote must not be able to trigger
// local command execution or credential prompts through git.
func gitCmd(args ...string) *exec.Cmd {
	hardened := []string{
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.file.allow=never",
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "credential.helper=",
	}
	cmd := exec.Command("git", append(hardened, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
	)
	return cmd
}

// CloneRepo runs `git clone --depth 1 <url> <dest>`, optionally checking out
// ref. ssh controls whether to clone via SSH (true) or HTTPS (false). HTTPS
// avoids the SSH-keys-required failure mode that bit ccpm during the notion
// plugin marketplace install on 2026-05-02.
func CloneRepo(url, dest, ref string, ssh bool) error {
	url = normalizeGitURL(url, ssh)
	if err := ValidateGitURL(url); err != nil {
		return err
	}
	if err := ValidateRef(ref); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	args := []string{"clone"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--depth", "1", "--", url, dest)
	out, err := gitCmd(args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %s: %w\n%s", url, err, string(out))
	}
	return nil
}

// CheckoutSHA does `git -C dest checkout <sha>` after a shallow clone has
// been deepened (or done as a full clone). Used when we want to pin to a
// specific commit rather than a branch tip.
func CheckoutSHA(dest, sha string) error {
	if sha == "" {
		return nil
	}
	if err := ValidateSHA(sha); err != nil {
		return err
	}
	// Fetch the specific commit so a shallow clone can resolve it.
	if out, err := gitCmd("-C", dest, "fetch", "--depth", "1", "origin", sha).CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch %s: %w\n%s", sha, err, string(out))
	}
	if out, err := gitCmd("-C", dest, "checkout", sha).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", sha, err, string(out))
	}
	return nil
}

// HeadSHA returns the full commit hash a clone is checked out at. Used to pin
// the exact installed commit into installed_plugins.json.
func HeadSHA(repoDir string) (string, error) {
	out, err := gitCmd("-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", repoDir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// normalizeGitURL rewrites between SSH (git@github.com:...) and HTTPS
// (https://github.com/...) GitHub URLs so the caller's protocol preference
// always wins. Non-GitHub URLs pass through unchanged.
func normalizeGitURL(url string, ssh bool) string {
	if ssh {
		if strings.HasPrefix(url, "https://github.com/") {
			rest := strings.TrimPrefix(url, "https://github.com/")
			return "git@github.com:" + rest
		}
		return url
	}
	if strings.HasPrefix(url, "git@github.com:") {
		rest := strings.TrimPrefix(url, "git@github.com:")
		return "https://github.com/" + rest
	}
	return url
}

// GitHubRepoURL returns "https://github.com/<repo>.git" or, if ssh is true,
// "git@github.com:<repo>.git". Used when the only thing we know about a
// marketplace is its "<org>/<repo>" slug.
func GitHubRepoURL(repo string, ssh bool) string {
	if ssh {
		return "git@github.com:" + repo + ".git"
	}
	return "https://github.com/" + repo + ".git"
}
