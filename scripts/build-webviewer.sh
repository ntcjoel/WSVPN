#!/bin/bash
# Build WSVPN Web Log Viewer

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SRC_DIR="$PROJECT_DIR/src/webviewer"
BUILD_DIR="$PROJECT_DIR/build"

echo "Building WSVPN Web Log Viewer..."

# Create build directory
mkdir -p "$BUILD_DIR"

# Build
cd "$SRC_DIR"
go build -o "$BUILD_DIR/wsvpn-webviewer" .

echo "✅ Build complete: $BUILD_DIR/wsvpn-webviewer"
echo ""
echo "Usage:"
echo "  ./build/wsvpn-webviewer -log-dir /var/log/wsvpn/server -port 8181"
echo ""
echo "Or with custom log directory:"
echo "  ./build/wsvpn-webviewer -log-dir ~/wsvpn-test -port 8181"
