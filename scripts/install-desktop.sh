#!/bin/sh
# CCPM desktop app installer (macOS) — https://github.com/nitin-1926/claude-code-profile-manager
# Usage: curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install-desktop.sh | sh
#
# Why this exists: the app is ad-hoc signed, not notarized (that needs a paid
# Apple Developer ID). A browser download carries the com.apple.quarantine
# flag, and Gatekeeper refuses a quarantined ad-hoc app outright — on macOS 15+
# it reports it as "damaged" with no Open Anyway button. curl does not set that
# flag, so a build fetched here opens like one you built yourself. The same is
# true of the in-app updater, which is why updates never hit the prompt.

set -e

REPO="nitin-1926/claude-code-profile-manager"
APP="CCPM.app"
INSTALL_DIR="${CCPM_APP_DIR:-/Applications}"

sha256_of() {
    shasum -a 256 "$1" | awk '{print $1}'
}

main() {
    if [ "$(uname -s)" != "Darwin" ]; then
        echo "Error: the CCPM desktop app is macOS only. The CLI works everywhere: see the README." >&2
        exit 1
    fi
    case "$(uname -m)" in
        arm64)  ARCH="arm64" ;;
        x86_64) ARCH="amd64" ;;
        *) echo "Error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac

    # Not /releases/latest: CLI (v*) and desktop (desktop-v*) releases share
    # one list, and "latest" is whichever was published last. The list comes
    # back newest first, so the first desktop-vX.Y.Z tag not flagged as a
    # prerelease is the newest desktop build (see install.sh for the parsing).
    TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=50" |
        tr ',' '\n' |
        awk -F'"' '/"tag_name":/ { tag = $4 }
            /"prerelease": *false/ { if (tag ~ /^desktop-v[0-9][0-9.]*$/) { print tag; exit } }')
    if [ -z "$TAG" ]; then
        echo "Error: could not find a desktop release. See https://github.com/${REPO}/releases?q=desktop-v" >&2
        exit 1
    fi
    VERSION="${TAG#desktop-v}"
    ZIP="CCPM-${VERSION}-${ARCH}.app.zip"
    BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

    echo "Installing CCPM desktop v${VERSION} (${ARCH}) to ${INSTALL_DIR}/${APP}"

    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    curl -fsSL "${BASE_URL}/${ZIP}" -o "${TMP_DIR}/${ZIP}"
    curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP_DIR}/checksums.txt"

    EXPECTED=$(grep "  ${ZIP}$" "${TMP_DIR}/checksums.txt" | awk '{print $1}')
    ACTUAL=$(sha256_of "${TMP_DIR}/${ZIP}")
    if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
        echo "Error: checksum mismatch for ${ZIP} (expected '${EXPECTED}', got '${ACTUAL}'). Refusing to install." >&2
        exit 1
    fi
    echo "  sha256 ok"

    # ditto, not unzip: it restores the bundle's code signature intact.
    /usr/bin/ditto -x -k "${TMP_DIR}/${ZIP}" "$TMP_DIR"
    /usr/bin/codesign --verify --deep --strict "${TMP_DIR}/${APP}"

    if pgrep -x CCPM >/dev/null 2>&1; then
        echo "  quitting the running CCPM"
        osascript -e 'tell application "CCPM" to quit' >/dev/null 2>&1 || true
        sleep 1
    fi

    SUDO=""
    mkdir -p "$INSTALL_DIR" 2>/dev/null || true
    if [ ! -w "$INSTALL_DIR" ]; then
        SUDO="sudo"
    fi
    # Copy in beside the old app, then swap: a failed copy (disk full, a
    # cancelled sudo prompt) must not leave the user with no app at all.
    STAGED="${INSTALL_DIR}/.${APP}.installing"
    $SUDO rm -rf "$STAGED"
    $SUDO /usr/bin/ditto "${TMP_DIR}/${APP}" "$STAGED"
    $SUDO rm -rf "${INSTALL_DIR:?}/${APP}"
    $SUDO mv "$STAGED" "${INSTALL_DIR}/${APP}"

    echo ""
    echo "Done. Open CCPM from ${INSTALL_DIR} or Spotlight; later updates install from inside the app."
    if ! command -v ccpm >/dev/null 2>&1; then
        echo "The app uses the ccpm CLI for write actions. Install it too:"
        echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | sh"
    fi
}

main
