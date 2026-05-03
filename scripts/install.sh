#!/bin/sh
# ccpm installer — https://github.com/nitin-1926/claude-code-profile-manager
# Usage: curl -fsSL https://raw.githubusercontent.com/nitin-1926/claude-code-profile-manager/main/scripts/install.sh | sh

set -e

REPO="nitin-1926/claude-code-profile-manager"
BINARY="ccpm"
INSTALL_DIR="${CCPM_INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *) echo "Error: Unsupported OS: $OS" >&2; exit 1 ;;
    esac

    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) echo "Error: Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac

    echo "${OS}_${ARCH}"
}

# Get latest release version
get_latest_version() {
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
        grep '"tag_name"' |
        sed -E 's/.*"v([^"]+)".*/\1/'
}

# Pick a SHA-256 verifier available on the host. Errors out if none found —
# we refuse to install without verifying the archive hash, because a
# tampered download would ship a trojaned binary that runs with the user's
# privileges on first invocation.
sha256_of() {
    _file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$_file" | awk '{print $1}'
        return
    fi
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$_file" | awk '{print $1}'
        return
    fi
    if command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$_file" | awk '{print $NF}'
        return
    fi
    echo "Error: no SHA-256 utility found (sha256sum / shasum / openssl)." >&2
    echo "  Install one and re-run; ccpm refuses to install without verifying the archive hash." >&2
    exit 1
}

main() {
    echo "Installing ccpm..."

    PLATFORM=$(detect_platform)
    VERSION=$(get_latest_version)

    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest version." >&2
        echo "Check https://github.com/${REPO}/releases" >&2
        exit 1
    fi

    echo "  Version:  v${VERSION}"
    echo "  Platform: ${PLATFORM}"
