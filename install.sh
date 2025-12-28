#!/bin/bash
# Gorelay installer script
# Usage: curl -sSL https://raw.githubusercontent.com/yejune/gorelay/main/install.sh | bash

set -e

REPO="yejune/gorelay"
BINARY="gorelay"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Get latest version
echo "🔍 Checking latest version..."
LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
    echo "❌ Failed to get latest version"
    exit 1
fi

echo "📦 Downloading $BINARY $LATEST for $OS/$ARCH..."

# Download
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST/$BINARY-$OS-$ARCH"
echo "DOWNLOAD_URL: $DOWNLOAD_URL"
TMP_FILE=$(mktemp)

if ! curl -sL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
    echo "❌ Download failed"
    rm -f "$TMP_FILE"
    exit 1
fi

# Make executable
chmod +x "$TMP_FILE"

# Install
echo "📋 Installing to $INSTALL_DIR..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_FILE" "$INSTALL_DIR/$BINARY"
else
    sudo mv "$TMP_FILE" "$INSTALL_DIR/$BINARY"
    sudo chmod +x "$INSTALL_DIR/$BINARY"
fi

echo ""
echo "✅ $BINARY $LATEST installed successfully!"
echo ""
echo "Get started:"
echo "  gorelay init     # Create Gorelayfile.yaml"
echo "  gorelay help     # Show help"
