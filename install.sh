#!/bin/sh
# revoco installer script
# Usage: curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/install.sh | bash
#
# Environment variables:
#   REVOCO_INSTALL_DIR  - Installation directory (default: ~/.local/bin or /usr/local/bin)
#   REVOCO_VERSION      - Specific version to install (default: latest)

set -e

REPO="fulgidus/revoco"
BINARY_NAME="revoco"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        CYGWIN*|MINGW*|MSYS*) echo "windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64";;
        arm64|aarch64)  echo "arm64";;
        *)              error "Unsupported architecture: $(uname -m)";;
    esac
}

# Get latest version from GitHub API
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Download file
download() {
    url="$1"
    dest="$2"
    
    info "Downloading from ${url}"
    
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$dest"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url" -O "$dest"
    else
        error "Neither curl nor wget found"
    fi
}

# Verify checksum
verify_checksum() {
    file="$1"
    expected="$2"
    
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | cut -d' ' -f1)
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | cut -d' ' -f1)
    else
        warn "sha256sum/shasum not found, skipping checksum verification"
        return 0
    fi
    
    if [ "$actual" != "$expected" ]; then
        error "Checksum verification failed!\nExpected: $expected\nActual:   $actual"
    fi
    
    success "Checksum verified"
}

# Main installation
main() {
    info "revoco installer"
    echo ""
    
    # Detect platform
    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "Detected platform: ${OS}/${ARCH}"
    
    # Determine version
    VERSION="${REVOCO_VERSION:-}"
    if [ -z "$VERSION" ]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
    fi
    
    if [ -z "$VERSION" ]; then
        error "Could not determine version to install"
    fi
    
    info "Installing version: ${VERSION}"
    
    # Determine archive format and binary name
    if [ "$OS" = "windows" ]; then
        ARCHIVE_EXT="zip"
        BINARY="${BINARY_NAME}.exe"
    else
        ARCHIVE_EXT="tar.gz"
        BINARY="${BINARY_NAME}"
    fi
    
    # Strip 'v' prefix from version for archive name
    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.${ARCHIVE_EXT}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
    
    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    # Download archive and checksums
    ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"
    CHECKSUMS_PATH="${TMP_DIR}/checksums.txt"
    
    download "$DOWNLOAD_URL" "$ARCHIVE_PATH"
    download "$CHECKSUMS_URL" "$CHECKSUMS_PATH"
    
    # Verify checksum
    EXPECTED_CHECKSUM=$(grep "$ARCHIVE_NAME" "$CHECKSUMS_PATH" | cut -d' ' -f1)
    if [ -n "$EXPECTED_CHECKSUM" ]; then
        verify_checksum "$ARCHIVE_PATH" "$EXPECTED_CHECKSUM"
    else
        warn "Checksum not found for ${ARCHIVE_NAME}, skipping verification"
    fi
    
    # Extract archive
    info "Extracting..."
    if [ "$ARCHIVE_EXT" = "zip" ]; then
        unzip -q "$ARCHIVE_PATH" -d "$TMP_DIR"
    else
        tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
    fi
    
    # Determine installation directory
    INSTALL_DIR="${REVOCO_INSTALL_DIR:-}"
    if [ -z "$INSTALL_DIR" ]; then
        # Try ~/.local/bin first (user-writable)
        if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
            INSTALL_DIR="$HOME/.local/bin"
        elif [ -w "/usr/local/bin" ]; then
            INSTALL_DIR="/usr/local/bin"
        else
            INSTALL_DIR="$HOME/.local/bin"
            mkdir -p "$INSTALL_DIR"
        fi
    fi
    
    # Install binary
    info "Installing to ${INSTALL_DIR}/${BINARY}"
    
    # Find the binary in extracted files
    EXTRACTED_BINARY=$(find "$TMP_DIR" -name "$BINARY" -type f | head -1)
    if [ -z "$EXTRACTED_BINARY" ]; then
        # Try without extension for cross-platform archives
        EXTRACTED_BINARY=$(find "$TMP_DIR" -name "${BINARY_NAME}" -type f | head -1)
    fi
    
    if [ -z "$EXTRACTED_BINARY" ]; then
        error "Could not find ${BINARY} in extracted archive"
    fi
    
    mv "$EXTRACTED_BINARY" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
    
    success "Installed ${BINARY} to ${INSTALL_DIR}"
    
    # Check if install dir is in PATH
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            echo ""
            warn "${INSTALL_DIR} is not in your PATH"
            echo ""
            echo "Add it to your shell profile:"
            echo ""
            echo "  # For bash (~/.bashrc or ~/.bash_profile):"
            echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
            echo ""
            echo "  # For zsh (~/.zshrc):"
            echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
            echo ""
            echo "  # For fish (~/.config/fish/config.fish):"
            echo "  fish_add_path ${INSTALL_DIR}"
            echo ""
            ;;
    esac
    
    # Verify installation
    echo ""
    if command -v "${INSTALL_DIR}/${BINARY}" >/dev/null 2>&1; then
        INSTALLED_VERSION=$("${INSTALL_DIR}/${BINARY}" --version 2>&1 || echo "unknown")
        success "Installation complete! Version: ${INSTALLED_VERSION}"
    else
        success "Installation complete!"
    fi
    
    echo ""
    info "Run 'revoco --help' to get started"
}

main "$@"
