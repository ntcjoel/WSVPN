# WSVPN Windows Client Testing Guide

**Test Machine:** tcw.vps (10.1.0.252:2201)  
**Date:** 2026-03-07  
**Version:** v0.4.7  

---

## 1. Environment Overview

| Component | Value |
|-----------|-------|
| **Test Machine** | tcw.vps (Windows 10 Pro 64-bit) |
| **SSH Access** | `ssh ntcjo@10.1.0.252 -p 2201` |
| **Server** | ts.vps (10.1.0.252:8180) |
| **VPN Network** | 10.9.1.0/24 |
| **Client UUID** | `device-windows-tcw-003` |
| **Assigned IP** | 10.9.1.4 |

---

## 2. Pre-Test Checklist

### 2.1 Verify Server Status

```bash
# SSH to server
ssh sq@10.1.0.252

# Check server is running
ps aux | grep wsvpn-server

# Check health endpoint
curl -ks "http://localhost:8180/ws/health?token=wsvpn_admin_k7m9p2x4q8r1t5v3w6y0z"
```

Expected output: `{"status":"healthy",...}`

### 2.2 Verify Client Configuration on Server

```bash
# Check clients.json on server
ssh sq@10.1.0.252 "cat ~/wsvpn-test/clients.json"
```

Must include:
```json
{
  "uuid": "device-windows-tcw-003",
  "ip": "10.9.1.4",
  "name": "tcw.vps-TestClient",
  "enabled": true
}
```

---

## 3. Windows Client Setup

### 3.1 Create Working Directory

```powershell
# SSH to Windows machine
ssh ntcjo@10.1.0.252 -p 2201

# Create directory
powershell -Command "New-Item -ItemType Directory -Force -Path C:\wsvpn-test"
```

### 3.2 Deploy Files

From Linux host:

```bash
# Copy client executable
scp -P 2201 /home/sq/.openclaw/workspace/wsvpn/build/wsvpn-client-v0.4.7.exe ntcjo@10.1.0.252:C:/wsvpn-test/wsvpn-client.exe

# Copy Wintun driver (CRITICAL - must be in root of test directory)
scp -P 2201 /home/sq/.openclaw/workspace/wsvpn/drivers/wintun/amd64/wintun.dll ntcjo@10.1.0.252:C:/wsvpn-test/wintun.dll

# Copy client config
scp -P 2201 /home/sq/.openclaw/workspace/wsvpn/config/client-windows-tcw.json ntcjo@10.1.0.252:C:/wsvpn-test/client-windows.json
```

### 3.3 Verify File Deployment

```powershell
# SSH to Windows machine
ssh ntcjo@10.1.0.252 -p 2201

# Check files exist
powershell -Command "Get-ChildItem C:\wsvpn-test | Select-Object Name,Length"
```

Expected files:
- `wsvpn-client.exe` (~10.4 MB)
- `wintun.dll` (~427 KB)
- `client-windows.json` (~250 bytes)

---

## 4. Configuration File

**File:** `C:\wsvpn-test\client-windows.json`

```json
{
  "name": "WSVPN",
  "client_ip": "",
  "server_url": "ws://10.1.0.252:8180",
  "uuid": "device-windows-tcw-003",
  "reconnect": true,
  "log_level": "info",
  "obfuscation": false,
  "transport": "websocket",
  "dns": "8.8.8.8",
  "quic_sni": ""
}
```

**Key Fields:**
- `server_url`: Use `ws://` for inner-net, `wss://` for TLS
- `uuid`: Must match entry in server's `clients.json`
- `obfuscation`: Set to `false` for testing (known bug in v0.4.6)

---

## 5. Connection Test

### 5.1 Run Client

```powershell
# SSH to Windows machine
ssh ntcjo@10.1.0.252 -p 2201

# Start VPN connection
powershell -Command "cd C:\wsvpn-test; .\wsvpn-client.exe connect --config client-windows.json"
```

### 5.2 Expected Success Output

```
2026/03/07 XX:XX:XX Wintun driver loaded
2026/03/07 XX:XX:XX Config file: client-windows.json (size: 251 bytes)
2026/03/07 XX:XX:XX Server IP resolved: 10.1.0.252
2026/03/07 XX:XX:XX Connection established
2026/03/07 XX:XX:XX Server assigned IP: 10.9.1.4
2026/03/07 XX:XX:XX TUN interface WSVPN created
2026/03/07 XX:XX:XX Route added: 0.0.0.0/0 via 10.9.1.1
2026/03/07 XX:XX:XX VPN connection ready
```

### 5.3 Verify Connection

**On Windows (new PowerShell window):**

```powershell
# Check network adapter
powershell -Command "Get-NetAdapter | Where-Object { $_.Name -like '*WSVPN*' }"

# Check IP configuration
powershell -Command "ipconfig /all | findstr -A 5 WSVPN"

# Test connectivity to server
ping 10.9.1.1
```

**On Server:**

```bash
# Check connected clients
ssh sq@10.1.0.252 "curl -ks 'http://localhost:8180/ws/health?token=wsvpn_admin_k7m9p2x4q8r1t5v3w6y0z'"
```

Should show `"connected": 1` (or higher if other clients connected)

---

## 6. Known Issues & Troubleshooting

### Issue 1: Wintun Driver Not Found

**Error:**
```
Failed to load wintun driver: wintun.dll not found (arch: amd64)
```

**Solution:**
Ensure `wintun.dll` is in `C:\wsvpn-test\` (same directory as executable):

```powershell
powershell -Command "Copy-Item C:\wsvpn-test\drivers\wintun\amd64\wintun.dll C:\wsvpn-test\wintun.dll -Force"
```

---

### Issue 2: TUN Adapter Creation Fails

**Error:**
```
init TUN: create TUN: Failed to find the tap device in registry with specified ComponentId 'tap0901', TAP driver may be not installed
```

**Root Cause:**
Windows system lacks Wintun kernel driver registration, or conflicting TAP drivers exist.

**Solutions (Try in Order):**

#### Option A: Install Official Wintun Driver

1. Download from: https://www.wintun.net/builds/wintun-0.14.1.zip
2. Extract and run installer as Administrator
3. Reboot Windows
4. Retry connection test

#### Option B: Check for TAP Driver Conflicts

```powershell
# List network adapters
powershell -Command "Get-PnpDevice -Class Net | Select-Object FriendlyName,Status"

# Look for OpenVPN TAP, WireGuard, or other VPN adapters
```

If found, temporarily disable conflicting adapters:

```powershell
powershell -Command "Disable-PnpDevice -InstanceId '<INSTANCE_ID>' -Confirm:$false"
```

#### Option C: Use Alternative Interface Name

Edit `client-windows.json`:

```json
{
  "name": "wsvpn0",  // Try different name
  ...
}
```

---

### Issue 3: Cannot Reach Server

**Error:**
```
dial WebSocket failed: dial tcp <IP>:8180: connectex: A socket operation was attempted to an unreachable network
```

**Diagnosis:**

```powershell
# Test network connectivity
ping 10.1.0.252

# Test port accessibility
powershell -Command "Test-NetConnection -ComputerName 10.1.0.252 -Port 8180"
```

**Solution:**
- Ensure firewall allows outbound connections to port 8180
- Verify server is listening: `ssh sq@10.1.0.252 "netstat -tlnp | grep 8180"`

---

### Issue 4: DNS Resolution Fails

**Error:**
```
resolve server IP: DNS lookup failed
```

**Solution:**
Use IP address directly in `server_url`:

```json
{
  "server_url": "ws://10.1.0.252:8180"
}
```

---

## 7. Disconnect & Cleanup

### Stop Client

```powershell
# In the PowerShell window running the client, press Ctrl+C

# Or kill process
powershell -Command "Get-Process wsvpn-client | Stop-Process -Force"
```

### Remove TUN Adapter

```powershell
# Delete WSVPN adapter
powershell -Command "Remove-NetAdapter -Name 'WSVPN' -Confirm:$false"

# Or via netsh
powershell -Command "netsh interface delete interface 'WSVPN'"
```

### Verify Cleanup

```powershell
# Check no残留 routes
powershell -Command "route print | findstr 10.9.1"

# Check no残留 adapters
powershell -Command "Get-NetAdapter | Where-Object { $_.Name -like '*WSVPN*' }"
```

---

## 8. Test Scenarios

### Test 1: Basic Connectivity

```powershell
# Connect
powershell -Command "cd C:\wsvpn-test; .\wsvpn-client.exe connect --config client-windows.json"

# In another window, test ping
ping 10.9.1.1 -n 10
```

**Expected:** 0% packet loss, RTT < 200ms

---

### Test 2: Large Packet

```powershell
ping 10.9.1.1 -l 1472 -n 20
```

**Expected:** 0% packet loss

---

### Test 3: Stability Test (30 seconds)

```powershell
ping 10.9.1.1 -i 0.5 -n 60
```

**Expected:** 0% packet loss, stable RTT

---

### Test 4: Throughput Test

**On Server:**
```bash
ssh sq@10.1.0.252 "iperf3 -s -B 10.9.1.1 -D"
```

**On Windows:**
```powershell
# Download iperf3 for Windows first
iperf3 -c 10.9.1.1 -t 10
```

**Expected:** > 40 Mbps throughput

---

## 9. Log Collection

### Client Logs

Client outputs to stdout/stderr. Capture with:

```powershell
powershell -Command "cd C:\wsvpn-test; .\wsvpn-client.exe connect --config client-windows.json 2>&1 | Out-File -FilePath C:\wsvpn-test\client.log"
```

### Server Logs

```bash
ssh sq@10.1.0.252 "tail -50 ~/wsvpn-test/server.log"
```

---

## 10. Quick Reference Commands

| Task | Command |
|------|---------|
| **SSH to Windows** | `ssh ntcjo@10.1.0.252 -p 2201` |
| **SSH to Server** | `ssh sq@10.1.0.252` |
| **Start Client** | `cd C:\wsvpn-test; .\wsvpn-client.exe connect --config client-windows.json` |
| **Check Server Health** | `curl -ks "http://localhost:8180/ws/health?token=..."` |
| **Deploy Files** | `scp -P 2201 <file> ntcjo@10.1.0.252:C:/wsvpn-test/` |
| **Kill Client** | `powershell -Command "Get-Process wsvpn-client \| Stop-Process -Force"` |

---

## 11. Current Status (2026-03-07 11:17)

**Completed:**
- ✅ Server deployment (ts.vps)
- ✅ Client executable deployment (tcw.vps)
- ✅ Wintun driver deployment
- ✅ Configuration setup
- ✅ Network connectivity verification (10.1.0.252:8180 accessible)

**Blocked:**
- ❌ TUN adapter creation fails (`ComponentId 'tap0901'` not found)

**Next Steps:**
1. Install official Wintun driver on Windows (Option A above)
2. Or check for TAP driver conflicts (Option B)
3. Re-test connection

---

**Contact:** Ellie (Assistant)  
**Generated:** 2026-03-07 11:17 GMT+8
