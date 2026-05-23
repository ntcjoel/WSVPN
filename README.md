# WSVPN — WebSocket VPN

**Version:** v1.0
**Status:** Production Ready
**License:** MIT License

---

## Overview

WSVPN is a lightweight WebSocket-based VPN for personal and small-team use. It tunnels IP traffic through a standard WebSocket-over-TLS (WSS) connection — indistinguishable from normal HTTPS traffic. Advanced obfuscation resists deep packet inspection and traffic analysis.

### Key Features

- **TLS Fingerprint Camouflage** — Mimics Chrome, Firefox, iOS, or Edge browser TLS handshakes (uTLS). The TLS ClientHello is indistinguishable from a real browser.
- **Traffic Shaping** — Burst/pause state machine simulates human browsing patterns. No continuous-stream VPN signature.
- **Randomized Packet Obfuscation** — Every packet header contains crypto-random bytes. No fixed pattern detectable by DPI.
- **WebSocket over TLS** — Standard WSS on port 443. Works behind any HTTPS reverse proxy (Nginx, Caddy).
- **UUID Authentication** — Simple device-based access control with static IP allocation.
- **Config Hot Reload** — Update configuration without restart (SIGHUP or HTTP API).
- **Auto-Reconnect** — Exponential backoff on network failure.
- **Docker Support** — Single-command deployment with Docker Compose.
- **Cross-Platform** — Linux server/client + Windows CLI client (Wintun driver).
- **Lightweight** — Single binary, <12 MB memory per instance.

---

## Anti-DPI Design

WSVPN uses three layers of obfuscation:

### Layer 1 — TLS Fingerprint Camouflage

Go's default `crypto/tls` produces a distinctive JA3/JA4 fingerprint that marks the client as a Go program. WSVPN uses `refraction-networking/utls` to mimic real browser TLS ClientHello messages:

| `tls_fingerprint` | Mimics |
|-------------------|--------|
| `chrome` (default) | Google Chrome |
| `firefox` | Mozilla Firefox |
| `ios` | iOS Safari |
| `edge` | Microsoft Edge |
| `random` | Picks randomly per connection |

### Layer 2 — Traffic Shaping

Continuous bidirectional throughput is a VPN signature. WSVPN uses a burst/pause state machine:

| `traffic_shape` | Behavior |
|-----------------|----------|
| `off` | No shaping |
| `jitter` | Per-packet random delay (100ms–2s) |
| `browse` | Burst (10–50 packets) → 30% chance of 2–8s pause |
| `adaptive` | Reserved for future ML-driven patterns |

### Layer 3 — Randomized Packet Obfuscation

Packets are padded to HTTPS-typical sizes (64/256/1024/1480 bytes, weighted distribution). The header format uses a 4-byte prefix where 2 bytes encode the original length and 2 bytes are crypto-random — every packet header looks different.

```
[03 28 a7 f1][original payload...][random padding...]
 ───┬─── ──┬──
   len    random (changes per packet)
```

---

## Quick Start

### Docker Compose (recommended)

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn

# Prepare config and SSL certificate
mkdir -p nginx/ssl nginx/html
cp config/server.example.json config/server.json
cp config/clients.example.json config/clients.json
# Edit server.json and clients.json with your settings
# Place your SSL cert at nginx/ssl/cert.pem and nginx/ssl/key.pem

# Start
docker compose up -d

# Check health
curl https://your-domain.com/ws/health?token=your-admin-token
```

### Docker (standalone server)

```bash
# Build
docker build -t wsvpn-server .

# Run (requires NET_ADMIN for TUN interface)
docker run -d \
    --cap-add=NET_ADMIN \
    --name wsvpn \
    -p 8180:8180 \
    -v $(pwd)/config:/config:ro \
    wsvpn-server
```

### Build from Source

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn

# Build with latest dependencies
./scripts/build.sh all --update-deps

# Build Windows client
./scripts/build-windows.sh --update-deps
```

The `--update-deps` flag pulls the latest uTLS (browser fingerprints), gorilla/websocket, and quic-go.

### Manual Server Setup

```bash
# Copy binary and config
scp build/wsvpn-server user@server:~/wsvpn/
scp config/server.json user@server:~/wsvpn/
scp config/clients.json user@server:~/wsvpn/

# Set capability (avoids running as root)
ssh user@server "sudo setcap cap_net_admin+ep ~/wsvpn/wsvpn-server"

# Start
ssh user@server "cd ~/wsvpn && nohup ./wsvpn-server > server.log 2>&1 &"
```

### Client Setup

```bash
# Linux
scp build/wsvpn-client config/client.json user@client:~/wsvpn-client/
ssh user@client "sudo setcap cap_net_admin+ep ~/wsvpn-client/wsvpn-client"
ssh user@client "cd ~/wsvpn-client && nohup ./wsvpn-client > client.log 2>&1 &"

# Windows — copy build/ directory, then:
wsvpn-client.exe connect --config client.json
```

### Verify

```bash
ping 10.9.1.1
```

---

## Configuration

### Server (`config/server.json`)

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

### Client (`config/client.json`)

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
  "tls_fingerprint": "chrome",
  "traffic_shape": "browse",
  "quic_sni": "your-domain.com"
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `obfuscation` | `true` | Enable packet obfuscation |
| `tls_fingerprint` | `"chrome"` | Browser TLS fingerprint to mimic |
| `traffic_shape` | `"off"` | Traffic shaping mode |

### Clients (`config/clients.json`)

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

### Nginx (`nginx/nginx.conf`)

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    location ~ "^/ws/([a-zA-Z0-9_-]{8,64})$" {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_pass http://wsvpn:8180;
        proxy_read_timeout 3600s;
    }

    location /ws/health {
        proxy_pass http://wsvpn:8180/ws/health;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }

    location /ws/reload {
        proxy_pass http://wsvpn:8180/ws/reload;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
    }
}
```

---

## Docker Compose Quick Reference

```
services:
  wsvpn          — VPN server (port 8180, internal only)
  nginx          — TLS termination + reverse proxy (port 443)
```

Directory layout for Docker Compose:

```
wsvpn/
├── docker-compose.yml
├── Dockerfile
├── config/
│   ├── server.json          # Server configuration
│   └── clients.json         # Client UUID/IP list
├── nginx/
│   ├── nginx.conf           # Nginx reverse proxy config
│   ├── ssl/
│   │   ├── cert.pem         # SSL certificate
│   │   └── key.pem          # SSL private key
│   └── html/
│       └── index.html       # Optional: landing page
```

---

## Monitoring

```bash
# Health check
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

### Hot Reload

```bash
docker compose exec wsvpn kill -SIGHUP 1
# or
curl -X POST "https://your-domain.com/ws/reload?token=your-admin-token"
```

---

## Resource Usage

| Metric | Value |
|--------|-------|
| Memory | <12 MB per instance |
| CPU | <5% (single core, 10 clients) |
| Binary Size | ~12 MB (Linux), ~12.4 MB (Windows) |

---

## Keeping Dependencies Current

uTLS fingerprints must stay current with browser updates. Rebuild with:

```bash
./scripts/build.sh all --update-deps
./scripts/build-windows.sh --update-deps
```

Add `--update-deps` to CI/CD or a monthly cron job.

---

## Troubleshooting

**"Failed to create TUN interface: operation not permitted"**
```bash
sudo setcap cap_net_admin+ep ./wsvpn-server
# Docker: ensure --cap-add=NET_ADMIN
```

**"Address already in use"**
```bash
pkill -9 -f wsvpn-server
# Docker: docker compose down && docker compose up -d
```

**"WebSocket handshake failed"**
```bash
nginx -t && nginx -s reload
openssl s_client -connect your-domain.com:443
```

**"Unauthorized UUID"** — Add UUID to `clients.json`, send SIGHUP or call `/ws/reload`.

**DNS failure on Windows** — `ipconfig /flushdns`

---

## Roadmap

- [ ] QUIC transport with uTLS fingerprint camouflage
- [ ] Domain fronting (CDN-based SNI hiding)
- [ ] Multi-connection traffic dispersion
- [ ] Mobile clients (iOS/Android)
- [ ] GUI desktop clients

---

## License

MIT License — See [LICENSE](LICENSE) file.

---

## Acknowledgments

- [refraction-networking/utls](https://github.com/refraction-networking/utls) — TLS fingerprint camouflage
- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket
- [quic-go/quic-go](https://github.com/quic-go/quic-go) — QUIC
- [songgao/water](https://github.com/songgao/water) — TUN/TAP
- [Wintun](https://www.wintun.net/) — Windows TUN driver
