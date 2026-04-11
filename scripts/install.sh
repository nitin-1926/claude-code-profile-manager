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

    URL="https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY}_${PLATFORM}.${EXT}"
    echo "  URL:      ${URL}"

    # Download and extract
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    echo ""
    echo "Downloading..."
    curl -fsSL "$URL" -o "${TMP_DIR}/archive.${EXT}"

    echo "Extracting..."
    if [ "$EXT" = "zip" ]; then
        unzip -q "${TMP_DIR}/archive.${EXT}" -d "$TMP_DIR"
    else
        tar -xzf "${TMP_DIR}/archive.${EXT}" -C "$TMP_DIR"
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
