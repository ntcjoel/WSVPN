# WSVPN - WebSocket VPN

**Version:** v0.4.9  
**Status:** Production Ready ✅  
**License:** MIT License

[![Release](https://img.shields.io/badge/release-v0.4.9-blue.svg)](https://github.com/ntcjoel/wsvpn/releases)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20Windows-lightgrey.svg)](https://github.com/ntcjoel/wsvpn)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org)

---

## 🚀 Overview

WSVPN is a lightweight, high-performance WebSocket-based VPN designed for personal and small-team use. It provides secure remote network access through a simple WebSocket tunnel with minimal overhead and advanced traffic obfuscation.

### ✨ Key Features

- **WebSocket Tunnel**: Full-duplex communication over standard WebSocket protocol (WSS)
- **UUID Authentication**: Simple device-based authentication via unique identifiers
- **Multi-Client Support**: Static IP allocation per registered device
- **Traffic Obfuscation**: HTTPS pattern simulation, adaptive padding, timing jitter
- **Health Monitoring**: Real-time metrics and status endpoint
- **Config Hot Reload**: Update configuration without service restart (SIGHUP or HTTP API)
- **HTTPS Compatible**: Works seamlessly with standard HTTPS reverse proxies (Nginx, Caddy)
- **Lightweight**: Single binary deployment, <10MB memory footprint
- **Cross-Platform**: Linux server/client + Windows CLI client (Wintun driver)
- **Auto-Reconnect**: Exponential backoff reconnection on network failure

### 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Client Device                                                  │
│    ┌─────────────┐                                             │
│    │ Application │  (Browser, App, etc.)                       │
│    └──────┬──────┘                                             │
│           ↓                                                     │
│    ┌─────────────┐                                             │
│    │ TUN Device  │  (wsvpn-client, 10.9.1.x/24)                │
│    └──────┬──────┘                                             │
│           ↓                                                     │
│    ┌─────────────────────────────────────────┐                 │
│    │ Obfuscation Layer                       │                 │
│    │ - Random Padding (50-500 bytes)         │                 │
│    │ - HTTPS Pattern Simulation              │                 │
│    │ - Timing Jitter (100ms-2s)              │                 │
│    └─────────────────────────────────────────┘                 │
│           ↓                                                     │
│    ┌─────────────┐                                             │
│    │ TLS (HTTPS) │  wss://your-domain.com/ws/{uuid}           │
│    └──────┬──────┘                                             │
└───────────┼─────────────────────────────────────────────────────┘
            │
            │  Internet (Encrypted TLS)
            ↓
┌─────────────────────────────────────────────────────────────────┐
│  Nginx Reverse Proxy (your-domain.com:443)                      │
│    ┌─────────────────────────────────────────┐                 │
│    │ TLS Termination                         │                 │
│    └─────────────────────────────────────────┘                 │
│           ↓                                                     │
│    ┌─────────────────────────────────────────┐                 │
│    │ WebSocket Proxy /ws/{uuid} → :8180      │                 │
│    └─────────────────────────────────────────┘                 │
└───────────┼─────────────────────────────────────────────────────┘
            │
            ↓
┌─────────────────────────────────────────────────────────────────┐
│  WSVPN Server (:8180)                                           │
│    ┌─────────────────────────────────────────┐                 │
│    │ UUID Authentication                     │                 │
│    └─────────────────────────────────────────┘                 │
│           ↓                                                     │
│    ┌─────────────────────────────────────────┐                 │
│    │ Remove Obfuscation                      │                 │
│    └─────────────────────────────────────────┘                 │
│           ↓                                                     │
│    ┌─────────────┐                                             │
│    │ TUN Device  │  (wsvpn0, 10.9.1.1/24)                      │
│    └──────┬──────┘                                             │
│           ↓                                                     │
│    ┌─────────────┐                                             │
│    │ Internet    │  (Route to destination)                     │
│    └─────────────┘                                             │
└─────────────────────────────────────────────────────────────────┘
```

---

## 📦 Pre-built Binaries

Download pre-compiled binaries from the [Releases page](https://github.com/ntcjoel/wsvpn/releases):

| Platform | Binary | Size |
|----------|--------|------|
| **Linux Server** | `wsvpn-server` | ~9.5 MB |
| **Linux Client** | `wsvpn-client` | ~10.5 MB |
| **Windows Client** | `wsvpn-client.exe` | ~10.4 MB |

### Verify Checksums

```bash
# SHA256 checksums are provided in each release
sha256sum wsvpn-server
sha256sum wsvpn-client
```

---

## 🚀 Quick Start

### Prerequisites

- **Server**: Linux with TUN/TAP support, Go 1.21+ (for building), Nginx (for HTTPS)
- **Client**: Linux/Windows with TUN/TAP or Wintun support
- **Capabilities**: `CAP_NET_ADMIN` (or root access)
- **SSL Certificate**: For HTTPS termination (Let's Encrypt recommended)

### Build from Source

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn
./scripts/build.sh all
```

### Server Deployment

```bash
# 1. Copy binary and config to server
scp build/wsvpn-server user@your-server:~/wsvpn/
scp config/server.json user@your-server:~/wsvpn/
scp config/clients.json user@your-server:~/wsvpn/

# 2. Set capabilities (no root needed for runtime)
ssh user@your-server "sudo setcap cap_net_admin+ep ~/wsvpn/wsvpn-server"

# 3. Start server
ssh user@your-server "cd ~/wsvpn && nohup ./wsvpn-server > server.log 2>&1 &"

# 4. (Optional) Install systemd service
sudo cp systemd/wsvpn-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable wsvpn-server
sudo systemctl start wsvpn-server
```

### Client Deployment

```bash
# 1. Copy binary and config to client
scp build/wsvpn-client user@client-machine:~/wsvpn-client/
scp config/client.json user@client-machine:~/wsvpn-client/

# 2. Set capabilities
ssh user@client-machine "sudo setcap cap_net_admin+ep ~/wsvpn-client/wsvpn-client"

# 3. Start client
ssh user@client-machine "cd ~/wsvpn-client && nohup ./wsvpn-client > client.log 2>&1 &"
```

### Verify Connection

```bash
# From client machine
ping 10.9.1.1
```

---

## 🔧 Configuration

### Server Configuration (`server.json`)

```json
{
  "name": "wsvpn0",
  "network": "10.9.1.0/24",
  "server_ip": "10.9.1.1",
  "listen_addr": ":8180",
  "websocket_path": "/ws/",
  "clients_file": "clients.json",
  "log_level": "info",
  "obfuscation": true,
  "admin_token": "your-32-char-random-token-here"
}
```

### Clients Configuration (`clients.json`)

```json
{
  "clients": [
    {
      "uuid": "device-phone-001",
      "ip": "10.9.1.2",
      "name": "My-iPhone",
      "enabled": true
    },
    {
      "uuid": "device-laptop-002",
      "ip": "10.9.1.3",
      "name": "My-MacBook",
      "enabled": true
    }
  ],
  "network": "10.9.1.0/24",
  "next_dynamic_ip": 50
}
```

### Client Configuration (`client.json`)

```json
{
  "name": "wsvpn-client",
  "client_ip": "10.9.1.2",
  "server_url": "wss://your-domain.com",
  "uuid": "device-phone-001",
  "reconnect": true,
  "log_level": "info",
  "obfuscation": true
}
```

### Nginx Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    location ~ "^/ws/([a-zA-Z0-9_-]{8,64})$" {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_pass http://127.0.0.1:8180;
        proxy_read_timeout 3600s;
    }

    location /ws/health {
        proxy_pass http://127.0.0.1:8180/ws/health;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }
}
```

---

## 📊 Monitoring & Operations

### Health Endpoint

```bash
curl "https://your-domain.com/ws/health?token=your-admin-token"
```

**Response:**
```json
{
  "status": "healthy",
  "uptime": "2h30m15s",
  "start_time": "2026-03-03T10:00:00Z",
  "clients": {
    "connected": 2,
    "configured": 2,
    "client_details": [
      {"id": "client-device-phone-001", "ip": "10.9.1.2"}
    ]
  },
  "traffic": {
    "bytes_in": 1048576,
    "bytes_out": 2097152,
    "packets_in": 1000,
    "packets_out": 2000
  },
  "system": {
    "goroutines": 15,
    "memory_alloc_bytes": 5242880,
    "cpus": 4,
    "go_version": "go1.24.4"
  }
}
```

### Config Hot Reload

**Via SIGHUP:**
```bash
kill -SIGHUP $(pgrep wsvpn-server)
```

**Via HTTP API:**
```bash
curl -X POST "https://your-domain.com/ws/reload?token=your-admin-token"
```

### Check Version

```bash
./wsvpn-server -version
./wsvpn-client -version
```

---

## 📈 Performance

### Test Results (v0.4.9)

| Test Type | Configuration | Result |
|-----------|---------------|--------|
| **ICMP (64 bytes)** | Cross-machine | 0% loss, RTT ~143ms |
| **ICMP (1472 bytes)** | Cross-machine | 0% loss, RTT ~143ms |
| **Stability Test** | 24 hours | 0% loss, 0 disconnects ✅ |
| **Throughput** | TCP iperf3 | ~50-85 Mbps |

### Resource Usage

| Metric | Value |
|--------|-------|
| **Memory** | <10 MB per server instance |
| **CPU** | <5% on single-core for 10 clients |
| **Goroutines** | ~6 base + 2 per client |
| **Binary Size** | ~10 MB (Linux), ~10.4 MB (Windows) |

---

## 🔒 Security

### Authentication

- **UUID-based**: Each device has a unique UUID (min 8 chars) in WebSocket path
- **Static IP Mapping**: Pre-defined UUID → IP assignment in `clients.json`
- **Unauthorized Rejection**: Unknown UUIDs receive HTTP 401

### Encryption

- **TLS 1.2/1.3**: Industry-standard encryption via Nginx/Let's Encrypt
- **Certificate Validation**: Client validates server certificate
- **No Inner Encryption**: VPN payload encrypted by TLS (no TLS-in-TLS overhead)

### Traffic Obfuscation

- **Adaptive Packet Sizing**: Dynamic packet size adjustment (64/256/1024/1480 bytes)
- **Random Padding**: 50-500 bytes per packet
- **Timing Jitter**: 100ms-2s random delays
- **HTTPS Pattern Simulation**: Mimics normal HTTPS traffic patterns

### Security Best Practices

1. **Use strong UUIDs**: Minimum 16 characters, alphanumeric + special chars
2. **Protect admin_token**: Keep secret, rotate periodically, use 32+ chars
3. **Enable obfuscation**: Always use in production (unless debugging)
4. **Monitor logs**: Watch for unauthorized connection attempts
5. **Limit exposure**: Firewall restrict access to server port
6. **Use HTTPS**: Never expose WebSocket without TLS termination

---

## 🛠️ Troubleshooting

### Common Issues

**1. "Failed to create TUN interface: ioctl: operation not permitted"**
```bash
# Solution: Set capabilities
sudo setcap cap_net_admin+ep ./wsvpn-server
# Or run as root (not recommended for production)
sudo ./wsvpn-server
```

**2. "Address already in use"**
```bash
# Solution: Kill existing process
pkill -9 -f wsvpn-server
# Or change port in server.json
```

**3. "WebSocket handshake failed"**
```bash
# Check Nginx configuration
sudo nginx -t
sudo nginx -s reload

# Verify SSL certificate
openssl s_client -connect your-domain.com:443
```

**4. "Unauthorized UUID"**
```bash
# Add UUID to clients.json
# Restart or hot-reload server
```

**5. "100% packet loss"**
```bash
# Check TUN interface
ip addr show wsvpn0

# Check routing
ip route show

# Check server logs
tail -f ~/wsvpn/server.log
```

### Clean Old Interfaces

```bash
# Server side
sudo ip link delete wsvpn0 2>/dev/null

# Client side (Linux)
sudo ip link delete wsvpn-client 2>/dev/null

# Client side (Windows)
# Reboot or use: pnputil /remove-device <device-id>
```

---

## 📝 Version History

### v0.4.9 (2026-03-07) - Comprehensive Stability Release
- ✅ WebSocket mutex protection (prevents concurrent write panic)
- ✅ Error channel buffer (prevents goroutine leak)
- ✅ Unified version constants across all components
- ✅ 24h stability test: 0% packet loss, 0 disconnects

### v0.4.6 (2026-03-06) - Dual Transport + CLI Flags
- ✅ WebSocket + QUIC dual transport support
- ✅ Command-line flags (`-config`, `-clients`, `-version`)
- ✅ Enhanced error recovery

### v0.4.5 (2026-03-06) - Route & Cleanup Fixes
- ✅ Temporary routes (no permanent route pollution)
- ✅ Route cleanup on disconnect

### v0.4.4 (2026-03-06) - Security Fix
- ✅ Constant-time comparison for admin token (timing attack prevention)

### v0.4.2 (2026-03-06) - Critical Stability Fixes
- ✅ WebSocket mutex protection
- ✅ Routing loop prevention
- ✅ Goroutine leak fix
- ✅ Auto-reconnect mechanism

### v0.3.0 (2026-03-03) - Production Ready
- ✅ Multi-client support with UUID authentication
- ✅ Health monitoring endpoint
- ✅ Config hot reload
- ✅ Traffic metrics collection

---

## 🎯 Roadmap

### v1.0 (Target)
- [x] 24h stability test (0% packet loss, 0 crashes) ✅
- [ ] Multi-client concurrent test (5+ devices)
- [ ] Mobile clients (iOS/Android)
- [ ] GUI clients (Tauri desktop, Flutter mobile)

### Future Considerations
- [ ] Dynamic IP allocation (DHCP-like pool)
- [ ] UUID rotation mechanism
- [ ] Rate limiting (token bucket)
- [ ] Prometheus metrics export
- [ ] Grafana dashboard
- [ ] Chrome plugin (smart routing rules)

---

## 📄 License

MIT License - See [LICENSE](LICENSE) file for details.

---

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📧 Contact

- **Repository**: https://github.com/ntcjoel/wsvpn
- **Issues**: https://github.com/ntcjoel/wsvpn/issues

---

## 🙏 Acknowledgments

- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [songgao/water](https://github.com/songgao/water) - TUN/TAP interface
- [quic-go](https://github.com/quic-go/quic-go) - QUIC implementation
- [Wintun](https://www.wintun.net/) - Windows TUN driver
