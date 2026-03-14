#!/bin/bash
# WSVPN Windows Client Release Script
# Usage: ./scripts/release-windows.sh <version>
# Example: ./scripts/release-windows.sh v0.4.1

set -e

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    echo "Error: Version required"
    echo "Usage: $0 <version>"
    echo "Example: $0 v0.4.1"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
SRC_DIR="$PROJECT_ROOT/src"
RELEASE_DIR="/tmp/wsvpn-windows-release-${VERSION}"
OUTPUT_FILE="${BUILD_DIR}/wsvpn-windows-release-${VERSION}.tar.gz"

echo "=== WSVPN Windows Client Release ${VERSION} ==="
echo "Project Root: $PROJECT_ROOT"
echo "Output: $OUTPUT_FILE"

# Clean previous release
rm -rf "$RELEASE_DIR"

# Create release directory structure
mkdir -p "$RELEASE_DIR/src"
mkdir -p "$RELEASE_DIR/config"
mkdir -p "$RELEASE_DIR/drivers/wintun/amd64"
mkdir -p "$RELEASE_DIR/drivers/wintun/arm64"
mkdir -p "$RELEASE_DIR/docs"
mkdir -p "$RELEASE_DIR/scripts"

# Copy source code
echo "Copying source code..."
cp "$SRC_DIR/client-windows/main.go" "$RELEASE_DIR/src/"

# Copy configuration
echo "Copying configuration..."
cp "$PROJECT_ROOT/config/client-windows.json" "$RELEASE_DIR/config/"

# Copy Wintun drivers
echo "Copying Wintun drivers..."
cp "$PROJECT_ROOT/drivers/wintun/amd64/wintun.dll" "$RELEASE_DIR/drivers/wintun/amd64/"
cp "$PROJECT_ROOT/drivers/wintun/arm64/wintun.dll" "$RELEASE_DIR/drivers/wintun/arm64/"

# Copy executable
echo "Copying executable..."
cp "$BUILD_DIR/wsvpn-client-${VERSION}.exe" "$RELEASE_DIR/wsvpn-client.exe"

# Copy documentation
echo "Copying documentation..."
cp "$PROJECT_ROOT/docs/WINDOWS.md" "$RELEASE_DIR/docs/"
cp "$PROJECT_ROOT/docs/SECURITY_AUDIT.md" "$RELEASE_DIR/docs/"
cp "$PROJECT_ROOT/docs/CHANGELOG.md" "$RELEASE_DIR/docs/"

# Copy scripts
echo "Copying scripts..."
cp "$SCRIPT_DIR/build-windows.sh" "$RELEASE_DIR/scripts/"

# Create release archive
echo "Creating release archive..."
cd /tmp
tar -czvf "$OUTPUT_FILE" "wsvpn-windows-release-${VERSION}/"

# Verify
echo ""
echo "=== Release Complete ==="
echo "Archive: $OUTPUT_FILE"
ls -lh "$OUTPUT_FILE"
echo ""
echo "Contents:"
tar -tzf "$OUTPUT_FILE" | head -20
echo ""
echo "To send via email:"
echo "  scp sq@<host>:$OUTPUT_FILE ."
echo ""

# Cleanup
rm -rf "$RELEASE_DIR"
