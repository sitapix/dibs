#!/usr/bin/env bash
# shellcheck shell=bash

# dibs installer — downloads the latest binary from GitHub releases.
#
# SECURITY NOTE: We recommend reviewing the script before running:
#   curl -fsSL https://raw.githubusercontent.com/sitapix/dibs/main/install.sh -o install.sh
#   less install.sh
#   bash install.sh
#
# Or install via Homebrew (preferred):
#   brew install sitapix/dibs/dibs

set -euo pipefail

REPO="sitapix/dibs"
BINARY="dibs"

# ============================================================================
# COLORS (only if stdout is a terminal)
# ============================================================================
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED='' GREEN='' YELLOW='' CYAN='' BOLD='' NC=''
fi

die() { printf '%bError: %s%b\n' "$RED" "$1" "$NC" >&2; exit 1; }
info() { printf '%b%s%b\n' "$CYAN" "$1" "$NC"; }
success() { printf '  %b✓%b %s\n' "$GREEN" "$NC" "$1"; }

# ============================================================================
# DETECT PLATFORM
# ============================================================================
detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$os" in
        darwin) os="darwin" ;;
        linux)  os="linux" ;;
        *)      die "Unsupported OS: $os" ;;
    esac

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)             die "Unsupported architecture: $arch" ;;
    esac

    printf '%s-%s' "$os" "$arch"
}

# ============================================================================
# MAIN
# ============================================================================
printf '%b%bdibs installer%b\n\n' "$CYAN" "$BOLD" "$NC"

# Check for curl
command -v curl &>/dev/null || die "curl is required but not installed."

# Detect platform
PLATFORM="$(detect_platform)"
info "Detected platform: $PLATFORM"

# Get latest release tag
info "Fetching latest release..."
LATEST=$(curl -fsSI -o /dev/null -w '%{redirect_url}' "https://github.com/$REPO/releases/latest" | grep -oE '[^/]+$')
[[ -n "$LATEST" ]] || die "Could not determine latest release"
success "Latest release: $LATEST"

# Download binary
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST/$BINARY-$PLATFORM"
TMP=$(mktemp) || die "Failed to create temp file"
trap 'rm -f -- "$TMP"' EXIT

info "Downloading $BINARY-$PLATFORM..."
curl -fsSL "$DOWNLOAD_URL" -o "$TMP" || die "Download failed. Check https://github.com/$REPO/releases for available binaries."

# Verify checksum
CHECKSUMS_URL="https://github.com/$REPO/releases/download/$LATEST/checksums.txt"
if CHECKSUMS=$(curl -fsSL "$CHECKSUMS_URL" 2>/dev/null); then
    EXPECTED=$(printf '%s' "$CHECKSUMS" | grep "$BINARY-$PLATFORM" | awk '{print $1}')
    if [[ -n "$EXPECTED" ]]; then
        if command -v sha256sum &>/dev/null; then
            ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
        elif command -v shasum &>/dev/null; then
            ACTUAL=$(shasum -a 256 "$TMP" | awk '{print $1}')
        else
            ACTUAL=""
        fi

        if [[ -n "$ACTUAL" ]]; then
            if [[ "$EXPECTED" != "$ACTUAL" ]]; then
                die "Checksum mismatch! Expected: $EXPECTED, Got: $ACTUAL"
            fi
            success "Checksum verified"
        fi
    fi
fi

# Install
INSTALL_DIR="/usr/local/bin"
if [[ ! -w "$INSTALL_DIR" ]]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

mv -- "$TMP" "$INSTALL_DIR/$BINARY" || die "Failed to install to $INSTALL_DIR"
chmod +x "$INSTALL_DIR/$BINARY"
trap - EXIT

success "Installed to $INSTALL_DIR/$BINARY"

# Check PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    printf '\n%bAdd %s to your PATH:%b\n' "$YELLOW" "$INSTALL_DIR" "$NC"
    printf '  export PATH="%s:$PATH"\n\n' "$INSTALL_DIR"
fi

printf '\n%b%bDone!%b Run %bdibs --help%b to get started.\n' "$GREEN" "$BOLD" "$NC" "$CYAN" "$NC"
