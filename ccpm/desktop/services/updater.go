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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/mod/semver"
	"golang.org/x/sys/unix"
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
	ctx        context.Context
	http       *http.Client
	installing atomic.Bool // guards against concurrent Install() calls
}

func NewUpdater() *Updater {
	// No Client.Timeout: it also bounds reading the body, and 60s failed the
	// 4.5MB download on any link under ~75KB/s. A server that never answers
	// is caught by ResponseHeaderTimeout; each request carries its own
	// deadline sized for what it fetches.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &Updater{http: &http.Client{
		Transport: tr,
		// Follow GitHub's asset redirects, but never onto a foreign host or
		// down to plaintext http — the redirect target is attacker-chosen if
		// the release JSON is ever tampered with.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if !trustedReleaseURL(req.URL.String()) {
				return fmt.Errorf("refusing redirect to untrusted URL %q", req.URL.Redacted())
			}
			return nil
		},
	}}
}

// releaseHosts is every host the updater will talk to or hand to the OS opener.
// GitHub serves release assets from github.com and redirects them to its
// object storage, so both must be present.
var releaseHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// trustedReleaseURL reports whether raw is an https URL on a known GitHub host.
// Everything the updater fetches, and the release page it asks the OS to open,
// is decoded from a JSON response — none of it is trusted by construction.
func trustedReleaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	return releaseHosts[u.Hostname()]
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
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
		// Skip drafts and prereleases — only stable desktop-v* builds auto-update.
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, tagPrefix) {
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
		if strings.HasSuffix(a.Name, want) && trustedReleaseURL(a.URL) {
			info.AssetURL = a.URL
			info.AssetName = a.Name
			break
		}
	}
	if info.AssetURL == "" {
		return info, fmt.Errorf("release %s has no %s asset", best.TagName, want)
	}
	if !trustedReleaseURL(info.URL) {
		info.URL = "" // never hand an untrusted scheme/host to the OS opener
	}
	info.Available = true
	return info, nil
}

// Install downloads the update for this architecture, verifies its SHA-256 against
// the release checksums, extracts it, and swaps the running .app in place before
// relaunching. It returns after spawning the detached swap helper; the app then
// quits so the helper can replace the bundle.
func (u *Updater) Install() error {
	if !u.installing.CompareAndSwap(false, true) {
		return fmt.Errorf("an update is already in progress")
	}
	// Release the guard on any failure; on success the app quits, so leave it set.
	ok := false
	defer func() {
		if !ok {
			u.installing.Store(false)
		}
	}()

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
	if err := checkInstallable(bundle); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "ccpm-update-")
	if err != nil {
		return err
	}
	// Remove the download/extract dir unless we hand it to the swap helper (which
	// deletes it after replacing the bundle) — otherwise every update leaks it.
	handedOff := false
	defer func() {
		if !handedOff {
			_ = os.RemoveAll(tmp)
		}
	}()
	u.emit("downloading", 0)

	// filepath.Base: AssetName is decoded from the release JSON, and a name of
	// "../../../x-arm64.app.zip" would pass the suffix match and escape tmp.
	zipPath := filepath.Join(tmp, filepath.Base(info.AssetName))
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
	if err := u.spawnSwap(bundle, newApp, tmp); err != nil {
		return err
	}
	handedOff = true
	ok = true
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
	// Generous: a stalled transfer still ends, a slow one gets to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	sumURL := base + "checksums.txt"
	if !trustedReleaseURL(sumURL) {
		return fmt.Errorf("checksums URL %q is not a trusted GitHub release URL", sumURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sumURL, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := u.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums.txt missing (%s) — refusing unverified update", resp.Status)
	}
	// A release manifest is a few hundred bytes; cap the read so a hostile or
	// corrupted response can't be streamed into memory without bound.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
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

// swapScript waits for the app to exit, replaces the bundle, and relaunches.
// Paths are positional args, never interpolated into the body, so shell
// metacharacters in them can't be expanded. $5 is the opener (/usr/bin/open;
// a stub under test).
//
// No set -e: the app has already quit when this runs, so every path must end
// in the reopen and the cleanup. A failed mv (a parent the user cannot write)
// leaves the old bundle where it was and reopens it.
const swapScript = `#!/bin/sh
PID="$1"; OLD="$2"; NEW="$3"; TMP="$4"; OPEN="$5"
# wait for the running app to fully exit
while kill -0 "$PID" 2>/dev/null; do sleep 0.3; done
BAK="$OLD.bak-$$"
if mv "$OLD" "$BAK"; then
  if /usr/bin/ditto "$NEW" "$OLD"; then
    xattr -dr com.apple.quarantine "$OLD" 2>/dev/null || true
    rm -rf "$BAK"
  else
    # restore on failure
    rm -rf "$OLD"
    mv "$BAK" "$OLD"
  fi
fi
rm -rf "$TMP"
"$OPEN" "$OLD"
rm -rf "$(dirname "$0")"
`

// spawnSwap writes and launches a detached helper running swapScript.
func (u *Updater) spawnSwap(oldBundle, newApp, tmp string) error {
	// MkdirTemp, not a predictable name under os.TempDir(): when TMPDIR is
	// unset Go falls back to the world-writable /tmp, where a local attacker
	// could pre-create the path as a symlink or swap the file between the
	// write and /bin/sh reading it — arbitrary code as the updating user.
	// MkdirTemp gives a fresh 0700 directory nobody else can enter.
	shDir, err := os.MkdirTemp("", "ccpm-swap-")
	if err != nil {
		return err
	}
	sh := filepath.Join(shDir, "swap.sh")
	if err := os.WriteFile(sh, []byte(swapScript), 0o700); err != nil {
		_ = os.RemoveAll(shDir)
		return err
	}
	cmd := exec.Command("/bin/sh", sh, strconv.Itoa(os.Getpid()), oldBundle, newApp, tmp, "/usr/bin/open")
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
	macos := filepath.Dir(exe)       // .../Contents/MacOS
	contents := filepath.Dir(macos)  // .../Contents
	bundle := filepath.Dir(contents) // .../CCPM.app
	if !strings.HasSuffix(bundle, ".app") {
		return "", fmt.Errorf("not running from a .app bundle (%s) — install to /Applications first", exe)
	}
	return bundle, nil
}

// checkInstallable refuses a bundle the swap helper could not replace. The
// helper runs after the app has quit, so finding out there means the app is
// gone; finding out here means a toast saying why.
//
// Gatekeeper's App Translocation runs a quarantined download from a random
// read-only mount, and replacing the bundle means renaming it inside its
// parent, which needs write access there — a root-owned /Applications refuses
// a non-admin user.
func checkInstallable(bundle string) error {
	if strings.Contains(bundle, "/AppTranslocation/") {
		return fmt.Errorf("CCPM is running from a temporary read-only location — move it to /Applications, open it from there, and update again")
	}
	parent := filepath.Dir(bundle)
	if err := unix.Access(parent, unix.W_OK); err != nil {
		return fmt.Errorf("cannot write to %s to replace CCPM (%v) — update as a user who can, or download the new version from the release page", parent, err)
	}
	return nil
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

// semverNewer reports whether a is a newer version than b, with semver's
// precedence — 1.2.0 is newer than 1.2.0-rc1. A leading "v" is optional.
// Anything that is not a valid version ("dev") is treated as oldest.
func semverNewer(a, b string) bool {
	return semver.Compare(canonicalVersion(a), canonicalVersion(b)) > 0
}

func canonicalVersion(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}
