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
    echo "  Install:  ${INSTALL_DIR}/${BINARY}"

    # Determine archive format
    EXT="tar.gz"
    if [ "$(echo "$PLATFORM" | cut -d_ -f1)" = "windows" ]; then
        EXT="zip"
    fi

    ARCHIVE_NAME="${BINARY}_${PLATFORM}.${EXT}"
    BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"
    ARCHIVE_URL="${BASE_URL}/${ARCHIVE_NAME}"
    CHECKSUMS_URL="${BASE_URL}/checksums.txt"

    echo "  URL:      ${ARCHIVE_URL}"

    # Download archive + checksum manifest. Fail closed if either is missing;
    # we won't silently install an unverified binary.
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    echo ""
    echo "Downloading..."
    if ! curl -fsSL "$ARCHIVE_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"; then
        echo "Error: failed to download ${ARCHIVE_URL}" >&2
        exit 1
    fi
    if ! curl -fsSL "$CHECKSUMS_URL" -o "${TMP_DIR}/checksums.txt"; then
        echo "Error: failed to download ${CHECKSUMS_URL}" >&2
        echo "  Refusing to install without a checksum to verify against." >&2
        exit 1
    fi

    echo "Verifying SHA-256..."
    EXPECTED=$(grep "  ${ARCHIVE_NAME}$" "${TMP_DIR}/checksums.txt" | awk '{print $1}')
    if [ -z "$EXPECTED" ]; then
        echo "Error: ${ARCHIVE_NAME} not listed in checksums.txt — possible tampering or missing release asset." >&2
        exit 1
    fi
    ACTUAL=$(sha256_of "${TMP_DIR}/${ARCHIVE_NAME}")
    if [ "$EXPECTED" != "$ACTUAL" ]; then
        echo "Error: checksum mismatch for ${ARCHIVE_NAME}." >&2
        echo "  expected: $EXPECTED" >&2
        echo "  actual:   $ACTUAL" >&2
        echo "  Refusing to install a tampered binary." >&2
        exit 1
    fi
    echo "  sha256 ok (${EXPECTED})"

    echo "Extracting..."
    if [ "$EXT" = "zip" ]; then
        unzip -q "${TMP_DIR}/${ARCHIVE_NAME}" -d "$TMP_DIR"
    else
        tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"
    fi

    # Install binary
    echo "Installing to ${INSTALL_DIR}..."
    if [ -w "$INSTALL_DIR" ]; then
        cp "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        chmod +x "${INSTALL_DIR}/${BINARY}"
    else
        sudo cp "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY}"
    fi

    echo ""
    echo "Done! ccpm v${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
    echo ""
    echo "Next steps:"
    echo "  1. Add shell integration:  echo 'eval \"\$(ccpm shell-init)\"' >> ~/.zshrc"
    echo "  2. Reload shell:           source ~/.zshrc"
    echo "  3. Create your first profile: ccpm add personal"
    echo ""
}

main
