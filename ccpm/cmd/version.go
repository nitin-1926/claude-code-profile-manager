package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var versionCheckLatest bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the ccpm version (optionally check for a newer release)",
	RunE:  runVersion,
}

func init() {
	versionCmd.Flags().BoolVar(&versionCheckLatest, "check-latest", false, "query GitHub for the latest release (cached for 24h)")
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	fmt.Printf("ccpm %s\n", version)
	if !versionCheckLatest {
		return nil
	}

	latest, err := latestReleaseTag()
	if err != nil {
		// Network problems must not fail the command — version info printed.
		fmt.Fprintf(os.Stderr, "Warning: could not check latest release: %v\n", err)
		return nil
	}
	switch semver.Compare(normalizeSemver(latest), normalizeSemver(version)) {
	case 1:
		color.New(color.FgYellow, color.Bold).Printf("Update available: %s → %s\n", version, latest)
		fmt.Println("  npm:  npm i -g @ngcodes/ccpm")
		fmt.Println("  curl: re-run the install script from the README")
	default:
		color.New(color.FgGreen).Printf("Up to date (latest release: %s)\n", latest)
	}
	return nil
}

// latestVersionCache is the on-disk shape of the 24h release-check cache.
type latestVersionCache struct {
	Tag       string    `json:"tag"`
	CheckedAt time.Time `json:"checked_at"`
}

func versionCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ccpm", "latest-version.json"), nil
}

// latestReleaseTag returns the newest release tag, using a 24h disk cache so
// repeated checks don't hammer the GitHub API (and work offline).
func latestReleaseTag() (string, error) {
	cachePath, cacheErr := versionCachePath()
	if cacheErr == nil {
		if data, err := os.ReadFile(cachePath); err == nil {
			if tag, ok := freshCachedTag(data, time.Now()); ok {
				return tag, nil
			}
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/nitin-1926/claude-code-profile-manager/releases?per_page=50", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	tag, err := newestCLITag(body)
	if err != nil {
		return "", err
	}

	if cacheErr == nil {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err == nil {
			if data, err := json.Marshal(latestVersionCache{Tag: tag, CheckedAt: time.Now()}); err == nil {
				_ = os.WriteFile(cachePath, data, 0o600)
			}
		}
	}
	return tag, nil
}

// freshCachedTag returns the cached tag if it is under 24h old AND is a stable
// CLI release tag. Binaries before 0.7.0 read /releases/latest and could cache
// "desktop-v0.1.1"; trusting that made a freshly upgraded CLI report "Up to
// date (latest release: desktop-v0.1.1)" until the entry expired.
func freshCachedTag(data []byte, now time.Time) (string, bool) {
	var c latestVersionCache
	if json.Unmarshal(data, &c) != nil || now.Sub(c.CheckedAt) >= 24*time.Hour {
		return "", false
	}
	if !semver.IsValid(c.Tag) || semver.Prerelease(c.Tag) != "" {
		return "", false
	}
	return c.Tag, true
}

// newestCLITag picks the newest stable CLI release from a /releases listing.
//
// Not /releases/latest: that is whichever release was published last, and the
// desktop app's desktop-v* releases share the list. When one of those was
// newest, the check compared against "desktop-v0.1.1", which is not semver, and
// reported every install as up to date. The listing is newest first.
func newestCLITag(body []byte) (string, error) {
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", err
	}
	for _, r := range releases {
		if semver.IsValid(r.TagName) && semver.Prerelease(r.TagName) == "" && !r.Draft && !r.Prerelease {
			return r.TagName, nil
		}
	}
	return "", fmt.Errorf("no CLI release in the releases listing")
}
