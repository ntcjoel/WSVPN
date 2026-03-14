#!/bin/bash
# WSVPN Log Rotation and Cleanup Script
# Retains logs for 30 days, then automatically deletes

set -e

SERVER_LOG_DIR="/var/log/wsvpn/server"
CLIENT_LOG_DIR="/var/log/wsvpn/client"
RETENTION_DAYS=30

# Create log directories if they don't exist
mkdir -p "$SERVER_LOG_DIR" "$CLIENT_LOG_DIR"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting WSVPN log cleanup..."

# Clean up server logs older than RETENTION_DAYS
if [ -d "$SERVER_LOG_DIR" ]; then
    deleted_server=$(find "$SERVER_LOG_DIR" -name "*.jsonl" -type f -mtime +$RETENTION_DAYS -delete -print 2>/dev/null | wc -l)
    echo "  - Deleted $deleted_server old server log files from $SERVER_LOG_DIR"
fi

# Clean up client logs older than RETENTION_DAYS
if [ -d "$CLIENT_LOG_DIR" ]; then
    deleted_client=$(find "$CLIENT_LOG_DIR" -name "*.jsonl" -type f -mtime +$RETENTION_DAYS -delete -print 2>/dev/null | wc -l)
    echo "  - Deleted $deleted_client old client log files from $CLIENT_LOG_DIR"
fi

# Compress logs older than 1 day (but within retention period)
if [ -d "$SERVER_LOG_DIR" ]; then
    compressed_server=$(find "$SERVER_LOG_DIR" -name "*.jsonl" -type f -mtime +1 ! -name "*.gz" -exec gzip -9 {} \; -print 2>/dev/null | wc -l)
    echo "  - Compressed $compressed_server server log files"
fi

if [ -d "$CLIENT_LOG_DIR" ]; then
    compressed_client=$(find "$CLIENT_LOG_DIR" -name "*.jsonl" -type f -mtime +1 ! -name "*.gz" -exec gzip -9 {} \; -print 2>/dev/null | wc -l)
    echo "  - Compressed $compressed_client client log files"
fi

# Report current disk usage
server_size=$(du -sh "$SERVER_LOG_DIR" 2>/dev/null | cut -f1 || echo "0")
client_size=$(du -sh "$CLIENT_LOG_DIR" 2>/dev/null | cut -f1 || echo "0")

echo "  - Server log directory size: $server_size"
echo "  - Client log directory size: $client_size"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Log cleanup completed."
