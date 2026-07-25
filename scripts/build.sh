#!/bin/bash
# Cross-compile caido-mcp-server and caido-cli for release.
# Usage: ./scripts/build.sh [VERSION]
# Output: dist/

set -euo pipefail

VERSION="${1:-dev}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="${ROOT}/dist"
PLATFORMS=("darwin/amd64" "darwin/arm64" "linux/amd64" "linux/arm64" "windows/amd64" "windows/arm64")

rm -rf "$DIST"
mkdir -p "$DIST"

echo "Building $VERSION for ${#PLATFORMS[@]} targets..."

for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  suffix="${GOOS}-${GOARCH}"
  ext=""
  if [ "$GOOS" = "windows" ]; then
    ext=".exe"
  fi

  echo "  ${suffix}"

  LDFLAGS="-s -w -X github.com/c0tton-fluff/caido-mcp-server/internal/buildinfo.version=${VERSION}"

  # MCP server
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -C "$ROOT" -ldflags="$LDFLAGS" \
    -o "${DIST}/caido-mcp-server-${suffix}${ext}" ./cmd/caido-mcp-server

  # CLI
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -C "$ROOT" -ldflags="$LDFLAGS" \
    -o "${DIST}/caido-cli-${suffix}${ext}" ./cmd/caido-cli
done

echo ""
echo "Binaries in ${DIST}/:"
ls -lh "$DIST/"
