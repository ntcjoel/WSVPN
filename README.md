# WSVPN - WebSocket VPN

**Version:** v1.0  
**Status:** Production Ready ✅  
**License:** MIT License

[![Release](https://img.shields.io/badge/release-v1.0-blue.svg)](https://github.com/ntcjoel/wsvpn/releases)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20Windows-lightgrey.svg)](https://github.com/ntcjoel/wsvpn)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org)

---

## Overview

WSVPN is a lightweight, high-performance WebSocket-based VPN designed for personal and small-team use. It provides secure remote network access through a standard WebSocket-over-TLS (WSS) tunnel with **advanced anti-DPI obfuscation** to resist deep packet inspection and censorship detection.

### Key Features

- **WebSocket Tunnel**: Full-duplex communication over standard WSS (WebSocket + TLS 1.3)
- **TLS Fingerprint Camouflage**: Mimics Chrome/Firefox/iOS browser TLS handshakes via uTLS — defeats JA3/JA4 fingerprint-based detection
- **Traffic Shaping**: Browse-mode burst/pause cycles simulate real human browsing patterns — eliminates continuous-stream VPN signatures
- **Obfuscation v2**: Double-randomized packet header format prevents fixed-pattern DPI fingerprinting
- **UUID Authentication**: Simple device-based authentication via unique identifiers
- **Multi-Client Support**: Static IP allocation per registered device
- **Config Hot Reload**: Update configuration without service restart (SIGHUP or HTTP API)
- **Auto-Reconnect**: Exponential backoff reconnection on network failure
- **HTTPS Compatible**: Works seamlessly behind standard HTTPS reverse proxies (Nginx, Caddy)
- **Lightweight**: Single binary deployment, <12MB memory footprint
- **Cross-Platform**: Linux server/client + Windows CLI client (Wintun driver)

---

## Anti-DPI Design

WSVPN v1.0 incorporates three layers of traffic obfuscation specifically designed to resist GFW/censorship deep packet inspection:

### Layer 1 — TLS Fingerprint Camouflage
Go's default `crypto/tls` produces a distinctive JA3/JA4 fingerprint that immediately identifies the client as a Go program (not a browser). WSVPN uses `refraction-networking/utls` to mimic real browser TLS ClientHello messages:

| Fingerprint | Description |
|-------------|-------------|
| `chrome` (default) | Mimics Google Chrome's TLS handshake |
| `firefox` | Mimics Mozilla Firefox's TLS handshake |
| `ios` | Mimics iOS Safari's TLS handshake |
| `edge` | Mimics Microsoft Edge's TLS handshake |
| `random` | Randomly selects from the above on each connection |

### Layer 2 — Traffic Shaping
Continuous bidirectional high-throughput is a definitive VPN signature. WSVPN implements a burst/pause state machine that mimics human browsing:

| Mode | Behavior |
|------|----------|
| `off` | No shaping (raw throughput) |
| `jitter` | Per-packet random delay (100ms–2s) |
| `browse` | Burst (10–50 packets) → 30% chance of 2–8s pause — simulates reading/thinking |
| `adaptive` | Reserved for future ML-driven pattern learning |

Paused packets are buffered and sent in goroutines — TUN reads are never blocked.

### Layer 3 — Randomized Obfuscation Header (v2)
The original v1 format `[uint32_be(len)][payload][padding]` has a detectable fixed 2-byte prefix of `0x00 0x00` (since VPN MTU ≤ 1500, always ≤ 0x05DC). The v2 format `[uint16_be(len)][uint16_be(random)][payload][padding]` replaces this with crypto-random bytes — every packet header looks different.

```
v1: [00 00 03 28][payload...][padding...]     ← detectable: bytes 0-1 always 0x00 0x00
v2: [03 28 a7 f1][payload...][padding...]     ← undetectable: bytes 2-3 are crypto-random
```

The client signals its obfuscation version via URL query parameter (`?ov=2`), so the server supports mixed v1/v2 clients simultaneously.

---

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│  Client Device                                                 │
│         ┌─────────────┐                                       │
│         │ Application │  (Browser, App, etc.)                  │
│         └──────┬──────┘                                       │
│                ↓                                               │
│         ┌─────────────┐                                       │
│         │ TUN Device  │  (wsvpn-client, 10.9.1.x/24)          │
│         └──────┬──────┘                                       │
│                ↓                                               │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ Anti-DPI Layers (client-side)                            │ │
│  │ 1. Obfuscation v2: randomized header + HTTPS-sized pad   │ │
│  │ 2. Traffic Shaping: burst/pause browsing simulation      │ │
│  └──────────────────────────────────────────────────────────┘ │
│                ↓                                               │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ TLS Fingerprint Camouflage (uTLS → Chrome/Firefox JA3)   │ │
│  └──────────────────────────────────────────────────────────┘ │
│                ↓                                               │
│         wss://your-domain.com/ws/{uuid}?ov=2                  │
└─────────────────┼─────────────────────────────────────────────┘
                  │
                  │  Internet (Encrypted — invisible to DPI)
                  ↓
┌────────────────────────────────────────────────────────────────┐
│  Nginx Reverse Proxy (your-domain.com:443)                     │
│    TLS Termination → WebSocket Upgrade → proxy to :8180        │
└─────────────────┼─────────────────────────────────────────────┘
                  │
                  ↓
┌────────────────────────────────────────────────────────────────┐
│  WSVPN Server (:8180)                                          │
│    UUID Auth → Remove Obfuscation → TUN (wsvpn0, 10.9.1.1/24) │
│    Route Table (O(1)) → Internet                               │
└────────────────────────────────────────────────────────────────┘
```

---

## Quick Start

### Prerequisites

- **Server**: Linux with TUN/TAP support, Go 1.21+, Nginx (for HTTPS)
- **Client**: Linux/Windows with TUN/TAP or Wintun driver
- **Capabilities**: `CAP_NET_ADMIN` (or root)
- **SSL Certificate**: For HTTPS termination (Let's Encrypt recommended)

### Build from Source

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn

# Build with latest dependencies (recommended)
./scripts/build.sh all --update-deps

# Build Windows client
./scripts/build-windows.sh --update-deps
```

The `--update-deps` flag automatically pulls the latest versions of uTLS, gorilla/websocket, and quic-go — keeping TLS fingerprints current with browser updates.

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
```

### Client Deployment

```bash
# Linux
scp build/wsvpn-client user@client:~/wsvpn-client/
scp config/client.json user@client:~/wsvpn-client/

# Windows — copy entire build/ directory
# Run: wsvpn-client.exe connect --config config/client.json
```

### Verify Connection

```bash
ping 10.9.1.1
```

---

## Configuration

### Server (`server.json`)

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
  "obfuscation_version_default": 1,
  "admin_token": "your-32-char-random-token-here"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `obfuscation` | bool | `true` | Enable/disable packet obfuscation |
| `obfuscation_version_default` | int | `1` | Default obfuscation version for clients not specifying `?ov=` |
| `admin_token` | string | — | Token for health/reload API endpoints |

### Client (`client.json`)

```json
{
  "name": "wsvpn-client",
  "client_ip": "10.9.1.2",
  "server_url": "wss://your-domain.com",
  "uuid": "device-phone-001",
  "reconnect": true,
  "log_level": "info",
  "obfuscation": true,
  "transport": "websocket",
  "obfuscation_version": 2,
  "tls_fingerprint": "chrome",
  "traffic_shape": "browse",
  "quic_sni": "your-domain.com"
}
```

Anti-DPI fields:

| Field | Type | Default | Values |
|-------|------|---------|--------|
| `obfuscation_version` | int | `1` | `1` = legacy header, `2` = randomized header |
| `tls_fingerprint` | string | `"chrome"` | `chrome`, `firefox`, `ios`, `edge`, `random` |
| `traffic_shape` | string | `"off"` | `off`, `jitter`, `browse`, `adaptive` |

### Clients (`clients.json`)

```json
{
  "clients": [
    {
      "uuid": "device-phone-001",
      "ip": "10.9.1.2",
      "name": "My-iPhone",
      "enabled": true
    }
  ],
  "network": "10.9.1.0/24",
  "next_dynamic_ip": 50
}
```

### Nginx Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    # WebSocket proxy — preserves UUID and ?ov= query param
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

    location /ws/reload {
        proxy_pass http://127.0.0.1:8180/ws/reload;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }
}
```

---

## Monitoring

### Health Endpoint

```bash
curl "https://your-domain.com/ws/health?token=your-admin-token"
```

```json
{
  "status": "healthy",
  "uptime": "2h30m15s",
  "clients": {"connected": 2, "configured": 2},
  "traffic": {"bytes_in": 1048576, "bytes_out": 2097152},
  "system": {"goroutines": 8, "memory_alloc_bytes": 5242880}
}
```

### Config Hot Reload

```bash
kill -SIGHUP $(pgrep wsvpn-server)
# or
curl -X POST "https://your-domain.com/ws/reload?token=your-admin-token"
```

---

## Performance

### v1.0 Test Results

| Test | Configuration | Result |
|------|---------------|--------|
| ICMP (64B) | Cross-machine, v2 obfuscation + uTLS + browse shape | 0% loss, RTT ~138–187ms |
| TCP Throughput | iperf3, v2 obfuscation + uTLS | 39.4 Mbps, 0 retransmissions |
| 50MB Download | HTTP through tunnel, browse shaping | ~28 Mbps avg |

### Resource Usage

| Metric | Value |
|--------|-------|
| Memory | <12 MB per server/client instance |
| CPU | <5% on single-core for 10 clients |
| Binary Size | ~12 MB (Linux), ~12.4 MB (Windows) |

---

## Keeping Dependencies Current

WSVPN's anti-DPI effectiveness depends on keeping uTLS fingerprints current with browser updates. Rebuild with:

```bash
# Update all dependencies and rebuild
./scripts/build.sh all --update-deps
./scripts/build-windows.sh --update-deps
```

This pulls the latest uTLS (browser TLS fingerprints), gorilla/websocket, and quic-go. Add `--update-deps` to your CI/CD pipeline or set up a cron job to rebuild periodically.

---

## Security

### Defense Layers

| Layer | Purpose | Mechanism |
|-------|---------|-----------|
| TLS 1.3 | Encryption (confidentiality) | Nginx + Let's Encrypt |
| TLS Fingerprint | JA3/JA4 evasion | uTLS Chrome/Firefox simulation |
| Traffic Shaping | Flow-pattern evasion | Burst/pause browsing simulation |
| Obfuscation v2 | Packet-pattern evasion | Randomized header + HTTPS-sized padding |
| UUID Auth | Access control | Path-based UUID in WebSocket URL |

### Best Practices

1. Use strong UUIDs (16+ characters, alphanumeric + special chars)
2. Protect `admin_token` — keep secret, rotate periodically
3. Enable obfuscation + v2 header + browse shaping in production
4. Rebuild with `--update-deps` monthly to keep TLS fingerprints current
5. Use HTTPS with valid certificate — never expose plain WebSocket

---

## Troubleshooting

### Common Issues

**1. "Failed to create TUN interface: operation not permitted"**
```bash
sudo setcap cap_net_admin+ep ./wsvpn-server
```

**2. "Address already in use"**
```bash
pkill -9 -f wsvpn-server
```

**3. "WebSocket handshake failed"**
```bash
# Verify nginx proxy and SSL certificate
sudo nginx -t && sudo nginx -s reload
openssl s_client -connect your-domain.com:443
```

**4. "Unauthorized UUID"**
```bash
# Add UUID to clients.json, then reload
kill -SIGHUP $(pgrep wsvpn-server)
```

**5. DNS resolution failure on Windows**
```bash
ipconfig /flushdns
```

---

## Version History

### v1.0 (2026-05-24) — Anti-DPI Production Release
- **TLS Fingerprint Camouflage**: uTLS integration — mimics Chrome/Firefox/iOS/Edge browser JA3/JA4 fingerprints
- **Traffic Shaping**: Burst/pause state machine simulates human browsing patterns — eliminates continuous-stream VPN signature
- **Obfuscation v2**: Double-randomized 2+2-byte header format — defeats DPI fixed-pattern detection
- **Auto-dependency updates**: `--update-deps` build flag keeps uTLS fingerprints current
- **Configurable anti-DPI**: `tls_fingerprint`, `traffic_shape`, `obfuscation_version` client config fields
- **Per-client obfuscation version**: Server supports mixed v1/v2 clients via `?ov=` URL query parameter

### v0.4.9 (2026-03-07) — Stability Release
- WebSocket mutex protection (prevents concurrent write panic)
- Error channel buffer (prevents goroutine leak)
- 24h stability test: 0% packet loss, 0 disconnects

### v0.4.6 (2026-03-06) — Dual Transport
- WebSocket + QUIC dual transport support
- Command-line flags (`-config`, `-clients`, `-version`)

### v0.4.4 (2026-03-06) — Security Fix
- Constant-time comparison for admin token (timing attack prevention)

---

## Roadmap

### v1.1
- [ ] QUIC transport with uTLS fingerprint camouflage
- [ ] Domain fronting support (CDN-based SNI hiding)
- [ ] Multi-connection traffic dispersion

### v1.2
- [ ] Mobile clients (iOS/Android)
- [ ] GUI desktop clients (Tauri)
- [ ] Chrome extension with smart routing rules

---

## License

MIT License — See [LICENSE](LICENSE) file.

---

## Acknowledgments

- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket implementation
- [refraction-networking/utls](https://github.com/refraction-networking/utls) — TLS fingerprint camouflage
- [quic-go](https://github.com/quic-go/quic-go) — QUIC implementation
- [songgao/water](https://github.com/songgao/water) — TUN/TAP interface
- [Wintun](https://www.wintun.net/) — Windows TUN driver
