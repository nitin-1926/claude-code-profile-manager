#!/usr/bin/env bash
#
# Build the CCPM desktop release artifacts for both macOS architectures:
#   - CCPM-<ver>-<arch>.dmg      drag-to-Applications download for humans (lzma)
#   - CCPM-<ver>-<arch>.app.zip  payload the in-app self-updater downloads (ditto)
#   - checksums.txt              SHA-256 the updater verifies against
#
# The version is baked into the binary via -ldflags so the updater knows what it
# is running. Usage:
#
#   scripts/build-desktop-dmg.sh <version>       # e.g. 0.1.0  or  desktop-v0.1.0
#
# Requires macOS with the Wails CLI, Go, and Node available.
set -euo pipefail

RAW="${1:?usage: build-desktop-dmg.sh <version|tag>}"
VERSION="${RAW#desktop-v}"; VERSION="${VERSION#v}"   # accept 0.1.0, v0.1.0, desktop-v0.1.0

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DESKTOP="$REPO_ROOT/ccpm/desktop"
DIST="$DESKTOP/dist"
VERSION_SYM="github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services.CurrentVersion"

command -v wails >/dev/null || { echo "error: wails CLI not found (go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0)" >&2; exit 1; }

rm -rf "$DIST"; mkdir -p "$DIST"
cd "$DESKTOP"

for arch in arm64 amd64; do
  echo "==> building darwin/$arch (v$VERSION)"
  wails build -platform "darwin/$arch" -clean -trimpath -ldflags "-X $VERSION_SYM=$VERSION"

  app="build/bin/CCPM.app"

  # Updater payload: ditto keeps the .app at the archive root and preserves the
  # ad-hoc code signature (a plain `zip` would not).
  /usr/bin/ditto -c -k --keepParent "$app" "$DIST/CCPM-$VERSION-$arch.app.zip"

  # Human download: lzma-compressed drag-to-Applications dmg.
  staging="$(mktemp -d)"
  cp -R "$app" "$staging/"
  ln -s /Applications "$staging/Applications"
  hdiutil create -volname "CCPM $VERSION" -srcfolder "$staging" -ov -format ULMO \
    "$DIST/CCPM-$VERSION-$arch.dmg" >/dev/null
  rm -rf "$staging"
done

cd "$DIST"
shasum -a 256 -- *.dmg *.app.zip > checksums.txt

echo "==> release artifacts (v$VERSION):"
ls -lh "$DIST"
