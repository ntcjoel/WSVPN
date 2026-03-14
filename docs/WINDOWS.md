# WSVPN Windows Client Guide

## Overview

The Windows client provides native VPN connectivity using the Wintun driver, with full compatibility with the Linux/macOS versions.

## Prerequisites

- Windows 10/11 (amd64 or arm64)
- Administrator privileges (for TUN interface creation)
- .NET Framework 4.7+ (for installer)

## Installation

### Option A: Installer (Recommended)

1. Download `wsvpn-client-setup.exe`
2. Run installer (requires admin rights)
3. Installation directory: `C:\Program Files\WSVPN\`
4. Start menu shortcut created automatically

### Option B: Portable

1. Extract `build/` directory to any location
2. Run `wsvpn-client.exe` from command prompt (as Administrator)

## Configuration

Edit `config/client-windows.json`:

```json
{
  "name": "WSVPN",           // TUN interface name
  "client_ip": "10.9.1.2",   // Assigned VPN IP
  "server_url": "wss://your-server.com",
  "uuid": "your-uuid-here",  // Authentication UUID
  "reconnect": true,         // Auto-reconnect on disconnect
  "log_level": "info",       // debug, info, warn, error
  "obfuscation": true,       // Traffic obfuscation
  "transport": "websocket"   // websocket or quic
}
```

## Usage

### Command Line

```cmd
# Start connection
wsvpn-client.exe connect --config config/client-windows.json

# Stop connection
wsvpn-client.exe disconnect

# Check status
wsvpn-client.exe status

# Show version
wsvpn-client.exe version
```

### Run as Windows Service (Optional)

Use NSSM (Non-Sucking Service Manager):

```cmd
# Download NSSM
nssm install WSVPN "C:\Program Files\WSVPN\wsvpn-client.exe"
nssm set WSVPN ObjectName LocalSystem
nssm set WSVPN Start SERVICE_AUTO_START
nssm start WSVPN
```

## Troubleshooting

### "Failed to load wintun.dll"

**Cause:** Driver file missing or wrong architecture.

**Solution:**
1. Ensure `drivers/wintun/amd64/wintun.dll` exists (for 64-bit Windows)
2. Ensure `drivers/wintun/arm64/wintun.dll` exists (for ARM Windows)
3. Run as Administrator

### "Permission denied" when creating TUN

**Cause:** Insufficient privileges.

**Solution:** Run command prompt as Administrator.

### "Connection refused"

**Cause:** Server unreachable or wrong URL.

**Solution:**
1. Check `server_url` in config
2. Verify network connectivity
3. Check firewall settings

### DNS not working

**Cause:** DNS not configured.

**Solution:** The client sets DNS to 8.8.8.8 by default. To customize:

```cmd
netsh interface ipv4 set dns name="WSVPN" static 1.1.1.1 primary
```

## Wintun Driver

The Wintun driver is included in the `drivers/wintun/` directory:

- `amd64/wintun.dll` - 64-bit Intel/AMD processors
- `arm64/wintun.dll` - ARM processors (Surface Pro X, etc.)

**License:** GPL-3.0 (https://www.wintun.net)

**Download:** https://www.wintun.net/builds/wintun-0.14.1.zip

## Building from Source

```bash
# From project root
./scripts/build-windows.sh

# Or manually
GOOS=windows GOARCH=amd64 go build -o build/wsvpn-client.exe ./src/client-windows
```

## Security Notes

- Keep `uuid` confidential (it's your authentication credential)
- Use TLS (wss://) for production
- Don't modify `wintun.dll` (signature verification may fail)

## Support

For issues, check:
- Server logs
- Client logs (stdout/stderr)
- Windows Event Viewer (if running as service)
