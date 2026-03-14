#!/bin/bash
# Build Windows CLI client for WSVPN
# Usage: ./build-windows.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
SRC_DIR="$PROJECT_ROOT/src/client-windows"

echo "=== WSVPN Windows Client Build ==="
echo "Project Root: $PROJECT_ROOT"
echo "Source: $SRC_DIR"
echo "Output: $BUILD_DIR"

# Create build directory
mkdir -p "$BUILD_DIR"

# Change to src directory for proper module resolution
cd "$PROJECT_ROOT/src"

# Check if cross-compiling or native
if [[ "$(go env GOOS)" == "windows" ]]; then
    echo "Building natively for Windows..."
    go build -o "$BUILD_DIR/wsvpn-client.exe" -trimpath ./client-windows
else
    echo "Cross-compiling for Windows (amd64)..."
    GOOS=windows GOARCH=amd64 go build -o "$BUILD_DIR/wsvpn-client.exe" -trimpath ./client-windows
fi

# Copy drivers directory
echo "Copying Wintun drivers..."
mkdir -p "$BUILD_DIR/drivers/wintun/amd64"
mkdir -p "$BUILD_DIR/drivers/wintun/arm64"
cp "$PROJECT_ROOT/drivers/wintun/amd64/wintun.dll" "$BUILD_DIR/drivers/wintun/amd64/"
cp "$PROJECT_ROOT/drivers/wintun/arm64/wintun.dll" "$BUILD_DIR/drivers/wintun/arm64/"

# Copy config
echo "Copying configuration..."
mkdir -p "$BUILD_DIR/config"
cp "$PROJECT_ROOT/config/client-windows.json" "$BUILD_DIR/config/"

echo ""
echo "=== Build Complete ==="
echo "Binary: $BUILD_DIR/wsvpn-client.exe"
echo "Drivers: $BUILD_DIR/drivers/wintun/"
echo "Config: $BUILD_DIR/config/client-windows.json"
echo ""
echo "To test on Windows:"
echo "  1. Copy entire build/ directory to Windows machine"
echo "  2. Run: wsvpn-client.exe connect --config config/client-windows.json"
echo ""
