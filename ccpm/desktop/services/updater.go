//go:build darwin

package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// CurrentVersion is the running app's version, injected at build time via
//
//	-ldflags "-X github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services.CurrentVersion=0.1.0"
//
// It stays "dev" for local/untagged builds, where the updater is a no-op.
var CurrentVersion = "dev"

const (
	releasesURL = "https://api.github.com/repos/nitin-1926/claude-code-profile-manager/releases?per_page=50"
	tagPrefix   = "desktop-v"
	userAgent   = "CCPM-Desktop-Updater"
)

// Updater checks GitHub Releases for a newer desktop build and, on request,
// downloads it and swaps the running .app in place. Self-downloaded builds are
// not Gatekeeper-quarantined, so the update installs without a re-prompt.
type Updater struct {
	ctx  context.Context
	http *http.Client
}

func NewUpdater() *Updater {
	return &Updater{http: &http.Client{Timeout: 60 * time.Second}}
}

// SetContext is called once from the App's startup with the Wails runtime ctx.
func (u *Updater) SetContext(ctx context.Context) { u.ctx = ctx }

// UpdateInfo is the result of a check, returned to the frontend.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`       // release web page
	AssetURL  string `json:"assetUrl"`  // this-arch .app.zip download URL
	AssetName string `json:"assetName"` // this-arch .app.zip file name
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	HTMLURL    string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

// Check queries GitHub for the newest desktop-v* release and reports whether it
// is newer than the running build. A "dev" build never reports an update.
func (u *Updater) Check() (UpdateInfo, error) {
	info := UpdateInfo{Available: false, Current: CurrentVersion}
	if CurrentVersion == "dev" {
		return info, nil // untagged local build — nothing to update to
	}

	req, _ := http.NewRequest(http.MethodGet, releasesURL, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.http.Do(req)
	if err != nil {
		return info, fmt.Errorf("contact GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return info, fmt.Errorf("decode releases: %w", err)
	}

	var best *ghRelease
	var bestVer string
	for i := range releases {
		r := &releases[i]
		if r.Draft || !strings.HasPrefix(r.TagName, tagPrefix) {
			continue
		}
		ver := strings.TrimPrefix(r.TagName, tagPrefix)
		if best == nil || semverNewer(ver, bestVer) {
			best, bestVer = r, ver
		}
	}
	if best == nil {
		return info, nil // no desktop releases yet
	}

	info.Latest = bestVer
	info.Notes = best.Body
	info.URL = best.HTMLURL
	if !semverNewer(bestVer, CurrentVersion) {
		return info, nil // up to date
	}

	// Match the .app.zip asset for this architecture (arm64 / amd64).
	want := "-" + runtime.GOARCH + ".app.zip"
	for _, a := range best.Assets {
		if strings.HasSuffix(a.Name, want) {
			info.AssetURL = a.URL
			info.AssetName = a.Name
			break
		}
	}
	if info.AssetURL == "" {
		return info, fmt.Errorf("release %s has no %s asset", best.TagName, want)
	}
	info.Available = true
	return info, nil
}

// Install downloads the update for this architecture, verifies its SHA-256 against
// the release checksums, extracts it, and swaps the running .app in place before
// relaunching. It returns after spawning the detached swap helper; the app then
// quits so the helper can replace the bundle.
func (u *Updater) Install() error {
	info, err := u.Check()
	if err != nil {
		return err
	}
	if !info.Available {
		return fmt.Errorf("no update available")
	}

	bundle, err := appBundlePath()
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "ccpm-update-")
	if err != nil {
		return err
	}
	u.emit("downloading", 0)

	zipPath := filepath.Join(tmp, info.AssetName)
	sum, err := u.download(info.AssetURL, zipPath)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}

	// Integrity: verify against checksums.txt from the same release.
	if err := u.verifyChecksum(info, sum); err != nil {
		return err
	}
	u.emit("extracting", 90)

	// ditto -x -k preserves the ad-hoc code signature; the zip has the .app at its root.
	extractDir := filepath.Join(tmp, "extracted")
	if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, extractDir).CombinedOutput(); err != nil {
		return fmt.Errorf("extract: %v: %s", err, out)
	}
	newApp, err := findDotApp(extractDir)
	if err != nil {
		return err
	}

	u.emit("installing", 98)
	if err := u.spawnSwap(bundle, newApp); err != nil {
		return err
	}
	u.emit("relaunching", 100)

	// Give the helper a beat to start waiting on our PID, then quit so it can swap.
	go func() {
		time.Sleep(400 * time.Millisecond)
		if u.ctx != nil {
			wr.Quit(u.ctx)
		} else {
			os.Exit(0)
		}
	}()
	return nil
}

// download streams url to dst, emitting progress (0..85%), and returns the file's
// lowercase hex SHA-256.
func (u *Updater) download(url, dst string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := u.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}

	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return "", werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if total > 0 {
				u.emit("downloading", int(done*85/total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyChecksum downloads checksums.txt from the release and confirms the
// downloaded asset's SHA-256 matches its listed value.
func (u *Updater) verifyChecksum(info UpdateInfo, gotSum string) error {
	// checksums.txt lives at the same base URL as the asset.
	base := info.AssetURL[:strings.LastIndex(info.AssetURL, "/")+1]
	req, _ := http.NewRequest(http.MethodGet, base+"checksums.txt", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := u.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums.txt missing (%s) — refusing unverified update", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.HasSuffix(f[1], info.AssetName) {
			if !strings.EqualFold(f[0], gotSum) {
				return fmt.Errorf("checksum mismatch for %s — update rejected", info.AssetName)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum for %s — refusing unverified update", info.AssetName)
}

// spawnSwap writes and launches a detached helper that waits for this process to
// exit, replaces the bundle with the new one, and relaunches it.
func (u *Updater) spawnSwap(oldBundle, newApp string) error {
	script := fmt.Sprintf(`#!/bin/sh
set -e
PID=%d
OLD=%q
NEW=%q
# wait for the running app to fully exit
while kill -0 "$PID" 2>/dev/null; do sleep 0.3; done
BAK="$OLD.bak-$$"
mv "$OLD" "$BAK"
if /usr/bin/ditto "$NEW" "$OLD"; then
  xattr -dr com.apple.quarantine "$OLD" 2>/dev/null || true
  rm -rf "$BAK"
else
  # restore on failure
  rm -rf "$OLD"
  mv "$BAK" "$OLD"
fi
/usr/bin/open "$OLD"
`, os.Getpid(), oldBundle, newApp)

	sh := filepath.Join(os.TempDir(), fmt.Sprintf("ccpm-swap-%d.sh", os.Getpid()))
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", sh)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // survive our exit
	return cmd.Start()
}

func (u *Updater) emit(phase string, pct int) {
	if u.ctx != nil {
		wr.EventsEmit(u.ctx, "updater:progress", map[string]any{"phase": phase, "percent": pct})
	}
}

// appBundlePath resolves the running .app bundle from the executable path,
// e.g. /Applications/CCPM.app/Contents/MacOS/CCPM -> /Applications/CCPM.app.
func appBundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	// .../CCPM.app/Contents/MacOS/CCPM
	macos := filepath.Dir(exe)         // .../Contents/MacOS
	contents := filepath.Dir(macos)    // .../Contents
	bundle := filepath.Dir(contents)   // .../CCPM.app
	if !strings.HasSuffix(bundle, ".app") {
		return "", fmt.Errorf("not running from a .app bundle (%s) — install to /Applications first", exe)
	}
	return bundle, nil
}

// findDotApp returns the single *.app directory directly inside dir.
func findDotApp(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .app found in downloaded update")
}

// semverNewer reports whether a is a newer X.Y.Z than b. Non-numeric or "dev"
// versions are treated as oldest.
func semverNewer(a, b string) bool {
	return cmpSemver(a, b) > 0
}

func cmpSemver(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // drop pre-release / build metadata
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}
