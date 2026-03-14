#!/bin/bash
# WSVPN Build and Deploy Script
# Usage: ./build.sh [server|client|all|deploy-server|deploy-client|deploy-all]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SRC_DIR="$PROJECT_DIR/src"
BUILD_DIR="$PROJECT_DIR/build"
CONFIG_DIR="$PROJECT_DIR/config"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

build_server() {
    log_info "Building WSVPN server..."
    cd "$SRC_DIR"
    
    # Download dependencies
    go mod download
    
    # Build server
    CGO_ENABLED=1 go build -o "$BUILD_DIR/wsvpn-server" -v ./server
    
    log_info "Server built: $BUILD_DIR/wsvpn-server"
}

build_client() {
    log_info "Building WSVPN client..."
    cd "$SRC_DIR"
    
    # Download dependencies
    go mod download
    
    # Build client
    CGO_ENABLED=1 go build -o "$BUILD_DIR/wsvpn-client" -v ./client
    
    log_info "Client built: $BUILD_DIR/wsvpn-client"
}

deploy_server() {
    log_info "Deploying server to ts.vps (10.1.0.252)..."
    
    # Copy binary
    scp -i ~/.ssh/id_ed25519 "$BUILD_DIR/wsvpn-server" sq@10.1.0.252:~/wsvpn/
    
    # Copy config files
    scp -i ~/.ssh/id_ed25519 "$CONFIG_DIR/server.json" sq@10.1.0.252:~/wsvpn/
    scp -i ~/.ssh/id_ed25519 "$CONFIG_DIR/clients.json" sq@10.1.0.252:~/wsvpn/config/
    
    log_info "Server deployed to ~/wsvpn/"
    log_info "To reload config: kill -SIGHUP \$(pgrep wsvpn-server)"
    log_info "Health check: curl 'http://10.1.0.252:8080/ws/health?token=<admin_token>'"
}

deploy_client() {
    log_info "Deploying client to tc.vps (10.1.0.252:2200)..."
    
    # Copy binary
    scp -i ~/.ssh/id_ed25519 -P 2200 "$BUILD_DIR/wsvpn-client" sq@10.1.0.252:~/wsvpn-client/
    
    # Copy config
    scp -i ~/.ssh/id_ed25519 -P 2200 "$CONFIG_DIR/client.json" sq@10.1.0.252:~/wsvpn-client/
    
    log_info "Client deployed to ~/wsvpn-client/"
}

# Main
mkdir -p "$BUILD_DIR"
mkdir -p "$CONFIG_DIR"

case "${1:-all}" in
    server)
        build_server
        ;;
    client)
        build_client
        ;;
    deploy-server)
        deploy_server
        ;;
    deploy-client)
        deploy_client
        ;;
    all)
        build_server
        build_client
        log_info "Build complete!"
        ;;
    deploy-all)
        deploy_server
        deploy_client
        log_info "Deployment complete!"
        ;;
    *)
        echo "Usage: $0 [server|client|deploy-server|deploy-client|all|deploy-all]"
        exit 1
        ;;
esac
