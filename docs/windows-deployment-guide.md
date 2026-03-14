# WSVPN Windows Client Deployment Guide

## Overview
This guide explains how to deploy the WSVPN Windows client with proper Wintun driver setup.

## Prerequisites
- Windows 7 SP1 or later (64-bit recommended)
- Administrative privileges to install network adapters
- Go 1.19+ (for building from source)

## Installation Steps

### 1. Download the Client
Download the latest `wsvpn-client.exe` binary from releases.

### 2. Wintun Driver Setup (CRITICAL)
The client requires the Wintun driver DLL. Place the `wintun.dll` file in one of these locations:

#### Option A: Same Directory as Executable (Recommended)
```
C:\Program Files\WSVPN\
├── wsvpn-client.exe
└── wintun.dll
```

#### Option B: Drivers Subdirectory Structure
```
C:\Program Files\WSVPN\
├── wsvpn-client.exe
└── drivers\
    └── wintun\
        ├── amd64\
        │   └── wintun.dll
        └── arm64\
            └── wintun.dll
```

### 3. Configuration File
Create a `client-windows.json` configuration file:

```json
{
  "name": "WSVPN",
  "server_url": "wss://your-server.com",
  "uuid": "your-unique-identifier",
  "reconnect": true,
  "transport": "websocket",
  "obfuscation": true
}
```

### 4. Running the Client
Open an elevated command prompt and run:

```cmd
wsvpn-client.exe connect --config client-windows.json
```

## Building from Source

To build the Windows client:

```bash
cd /path/to/wsvpn/src/client-windows
GOOS=windows GOARCH=amd64 go build -o wsvpn-client.exe .
```

Make sure to place the appropriate `wintun.dll` (x64 version for amd64 builds) in the same directory as the executable or in the drivers structure described above.

## Troubleshooting

### Common Issues

1. **"Failed to find the tap device in registry"** - This error indicates the old TAP driver was being referenced. The fixed version uses pure Wintun and should not show this error.

2. **"wintun.dll not found"** - Ensure the DLL is placed in the correct location as described above.

3. **"Access denied"** - Run the client with administrative privileges.

### Verification

After successful connection, you should see:
- A "WSVPN" network adapter in Windows Network Connections
- The assigned IP address configured on that adapter
- Log messages indicating successful connection

## Security Notes

- The client creates a virtual network adapter that requires administrator privileges
- All traffic is encrypted through the WebSocket/QUIC connection
- The Wintun driver is digitally signed by WireGuard/Wintun project