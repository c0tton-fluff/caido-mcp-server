#!/bin/bash
# Caido MCP Server Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/c0tton-fluff/caido-mcp-server/main/install.sh | bash

set -e

REPO="c0tton-fluff/caido-mcp-server"
VERSION="v1.0.0"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
    darwin) OS="darwin" ;;
    linux) OS="linux" ;;
    mingw*|msys*|cygwin*) OS="windows" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="caido-mcp-server-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
    BINARY="${BINARY}.exe"
fi

URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}"

echo "Installing caido-mcp-server ${VERSION}..."
echo "  OS: ${OS}, Arch: ${ARCH}"
echo "  URL: ${URL}"

# Create install directory
mkdir -p "$INSTALL_DIR"

# Download binary
if command -v curl &> /dev/null; then
    curl -fsSL "$URL" -o "${INSTALL_DIR}/caido-mcp-server"
elif command -v wget &> /dev/null; then
    wget -q "$URL" -O "${INSTALL_DIR}/caido-mcp-server"
else
    echo "Error: curl or wget required"
    exit 1
fi

chmod +x "${INSTALL_DIR}/caido-mcp-server"

echo ""
echo "Installed to: ${INSTALL_DIR}/caido-mcp-server"
echo ""
echo "Next steps:"
echo "  1. Add ${INSTALL_DIR} to your PATH (if not already)"
echo "  2. Run: CAIDO_URL=http://localhost:8080 caido-mcp-server login"
echo "  3. Add to your MCP config (see README)"
