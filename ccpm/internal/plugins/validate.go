package plugins

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Marketplace manifests are remote-controlled input: every name, ref, sha,
// url, and path they declare ends up in a filepath.Join or a git argv. These
// validators are the single trust boundary between manifest JSON and the
// filesystem / git. Callers must validate before joining or executing.

var (
	nameRegex    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	versionRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
	shaRegex     = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
	repoRegex    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	sshURLRegex  = regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9._/-]+$`)
)

// ValidateName checks a marketplace or plugin name that will become a path
// segment. Rejects separators, traversal, leading dot/dash, and control chars.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("invalid name %q: must start alphanumeric and contain only letters, digits, '.', '_', '-' (max 128 chars)", name)
	}
	return nil
}

// ValidateVersion checks a version string that will become a path segment
// (plugin cache dirs are keyed by version read from plugin.json).
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	if !versionRegex.MatchString(version) {
		return fmt.Errorf("invalid version %q: must start alphanumeric and contain only letters, digits, '.', '_', '+', '-' (max 64 chars)", version)
	}
	return nil
}

// ValidateRef checks a git branch/tag name passed to `git clone --branch`.
// Empty is allowed (default branch). Rejects flag injection and traversal.
func ValidateRef(ref string) error {
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid git ref %q: must not start with '-'", ref)
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("invalid git ref %q: must not contain '..'", ref)
	}
	if len(ref) > 128 {
		return fmt.Errorf("invalid git ref: too long (%d chars)", len(ref))
	}
	for _, r := range ref {
		if r < 0x20 || r == 0x7f || r == ' ' || r == '~' || r == '^' || r == ':' || r == '\\' {
			return fmt.Errorf("invalid git ref %q: contains forbidden character %q", ref, r)
		}
	}
	return nil
}

// ValidateSHA checks a git commit hash. Empty is allowed (no pin).
func ValidateSHA(sha string) error {
	if sha == "" {
		return nil
	}
	if !shaRegex.MatchString(sha) {
		return fmt.Errorf("invalid git sha %q: expected 7-40 hex characters", sha)
	}
	return nil
}

// ValidateRepo checks a GitHub "<org>/<repo>" slug.
func ValidateRepo(repo string) error {
	if repo == "" {
		return fmt.Errorf("repo cannot be empty")
	}
	if !repoRegex.MatchString(repo) {
		return fmt.Errorf("invalid repo %q: expected <org>/<repo>", repo)
	}
	return nil
}

// ValidateGitURL allowlists clone URL schemes. file://, ext::, bare paths and
// anything starting with '-' are rejected — a hostile marketplace.json must
// not be able to make git read the local filesystem or run helpers.
func ValidateGitURL(url string) error {
	if url == "" {
		return fmt.Errorf("git url cannot be empty")
	}
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "ssh://") {
		return nil
	}
	if sshURLRegex.MatchString(url) {
		return nil
	}
	return fmt.Errorf("invalid git url %q: only https://, ssh://, or git@host:path URLs are allowed", url)
}

// ValidateLocalPath checks a path from a manifest that will be joined under a
// clone root. Must stay relative and inside the root.
func ValidateLocalPath(p string) error {
	if p == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !filepath.IsLocal(p) {
		return fmt.Errorf("invalid path %q: must be relative and stay inside the repository", p)
	}
	return nil
}
