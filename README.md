# WSVPN — WebSocket VPN

**Version:** v1.2
**Status:** Production Ready
**License:** MIT

---

## Overview

WSVPN is a lightweight WebSocket-based VPN for personal and small-team use. It tunnels IP traffic through standard WebSocket-over-TLS (WSS) connections — indistinguishable from normal HTTPS traffic. Advanced obfuscation resists deep packet inspection and traffic analysis.

### Key Features

- **TLS Fingerprint Camouflage** — Mimics Chrome, Firefox, iOS, or Edge browser TLS handshakes (uTLS)
- **Traffic Shaping** — Burst/pause state machine simulates human browsing patterns
- **Randomized Packet Obfuscation** — Every packet header contains crypto-random bytes
- **Domain Fronting** — CDN-based SNI hiding: TLS connects to a front domain, real destination hidden in encrypted Host header
- **Multi-Connection Dispersion** — Traffic distributed across multiple server domains to simulate multi-site browsing
- **Built-in SOCKS5 Proxy** — Server exposes a SOCKS5 proxy on the VPN IP (default: port 1744)
- **Chrome Extension** — Smart PAC-based routing with China mainland / LAN / custom CIDR bypass rules
- **UUID Authentication** — Device-based access control with static IP allocation
- **Config Hot Reload** — Update configuration without restart
- **Docker Support** — Single-command deployment with Docker Compose
- **Cross-Platform** — Linux server/client + Windows CLI client (Wintun driver)

---

## Anti-DPI Design

Three layers of obfuscation:

### Layer 1 — TLS Fingerprint Camouflage

Go's default `crypto/tls` produces a distinctive JA3/JA4 fingerprint. WSVPN uses `refraction-networking/utls` to mimic real browsers:

| `tls_fingerprint` | Mimics |
|-------------------|--------|
| `chrome` (default) | Google Chrome |
| `firefox` | Mozilla Firefox |
| `ios` | iOS Safari |
| `edge` | Microsoft Edge |
| `random` | Picks randomly per connection |

### Layer 2 — Traffic Shaping

Burst/pause state machine breaks the continuous-stream VPN signature:

| `traffic_shape` | Behavior |
|-----------------|----------|
| `off` | No shaping |
| `jitter` | Per-packet random delay (100ms–2s) |
| `browse` | Burst (10–50 packets) → 30% chance of 2–8s pause |
| `adaptive` | Reserved for future ML-driven patterns |

### Layer 3 — Randomized Packet Obfuscation

Packets are padded to HTTPS-typical sizes (64/256/1024/1480 bytes, weighted distribution). Each header contains 2 bytes of crypto-random data — no two packets have the same header pattern.

---

## Architecture

```
Client Device
  Application → TUN (10.9.1.x)
    → Obfuscation + Traffic Shaping
      → uTLS (Chrome/Firefox fingerprint)
        → wss://domain/ws/{uuid}  ──→  Internet (TLS 1.3)
                                         → Nginx → WSVPN Server (:8180)
                                           → TUN (10.9.1.1)
                                             → Internet / SOCKS5 (:1744)
```

---

## Quick Start

### Docker Compose

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn
mkdir -p nginx/ssl nginx/html
cp config/server.example.json config/server.json
cp config/clients.example.json config/clients.json
# Edit config files, place SSL cert at nginx/ssl/

docker compose up -d
curl "https://your-domain.com/ws/health?token=your-admin-token"
```

### Docker (standalone)

```bash
docker build -t wsvpn-server .
docker run -d --cap-add=NET_ADMIN --name wsvpn \
    -p 8180:8180 -v $(pwd)/config:/config:ro wsvpn-server
```

### Build from Source

```bash
./scripts/build.sh all --update-deps     # Linux
./scripts/build-windows.sh --update-deps # Windows
```

### Manual Deploy

```bash
scp build/wsvpn-server config/*.json user@server:~/wsvpn/
ssh user@server "sudo setcap cap_net_admin+ep ~/wsvpn/wsvpn-server"
ssh user@server "cd ~/wsvpn && nohup ./wsvpn-server > server.log 2>&1 &"
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
  "socks5_enabled": true,
  "socks5_port": 1744,
  "admin_token": "your-32-char-random-token-here"
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `socks5_enabled` | `true` | Built-in SOCKS5 proxy |
| `socks5_port` | `1744` | SOCKS5 proxy listen port |

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
  "front_domain": "",
  "dispersion_peers": [],
  "quic_sni": "your-domain.com"
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `tls_fingerprint` | `"chrome"` | Browser TLS fingerprint |
| `traffic_shape` | `"off"` | Traffic shaping mode |
| `front_domain` | `""` | CDN front domain for domain fronting |
| `dispersion_peers` | `[]` | Additional server URLs for traffic dispersion |

### Domain Fronting

Route VPN traffic through a CDN to hide the real server:

```json
{
  "server_url": "wss://real-server.com",
  "front_domain": "https://d123.cloudfront.net"
}
```

- TLS connects to `d123.cloudfront.net` (visible to DPI)
- HTTP Host header is `real-server.com` (hidden inside TLS)
- CDN must be configured to forward to `real-server.com` as origin

### Traffic Dispersion

Distribute VPN packets across multiple servers:

```json
{
  "server_url": "wss://primary.com",
  "dispersion_peers": [
    "wss://secondary.com",
    "wss://tertiary.com"
  ]
}
```

Packets are round-robin distributed across all connections. A DPI observer sees connections to multiple different domains — resembling normal multi-site browsing.

### SOCKS5 Proxy

The server automatically exposes a SOCKS5 proxy (RFC 1928) on `10.9.1.1:1744`. VPN clients can use this proxy directly:

```bash
curl --socks5 10.9.1.1:1744 https://example.com
```

### Nginx

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
        proxy_pass http://127.0.0.1:8180;
        proxy_read_timeout 3600s;
    }

    location /ws/health { proxy_pass http://127.0.0.1:8180/ws/health; proxy_http_version 1.1; proxy_set_header Host $host; }
    location /ws/reload { proxy_pass http://127.0.0.1:8180/ws/reload; proxy_http_version 1.1; proxy_set_header Host $host; }
}
```

---

## Chrome Extension

`extension/` contains a Chrome extension that routes traffic through the VPN SOCKS5 proxy using PAC (Proxy Auto-Config) scripting.

### Features

- Route China mainland IP ranges through the VPN
- Route LAN addresses through the VPN
- Custom CIDR ranges
- One-click enable/disable from toolbar popup

### Install

1. Chrome → `chrome://extensions` → Developer mode ON
2. Load unpacked → select `extension/` directory
3. Click toolbar icon to configure

---

## Docker Compose Layout

```
wsvpn/
├── docker-compose.yml
├── Dockerfile
├── config/
│   ├── server.json
│   └── clients.json
├── nginx/
│   ├── nginx.conf
│   └── ssl/
│       ├── cert.pem
│       └── key.pem
└── extension/          # Chrome extension (optional)
```

---

## Monitoring

```bash
curl "https://your-domain.com/ws/health?token=your-admin-token"
# → {"status":"healthy","clients":{"connected":2},"traffic":{...}}
```

Hot reload: `kill -SIGHUP $(pgrep wsvpn-server)` or `POST /ws/reload`

---

## Resource Usage

| Metric | Value |
|--------|-------|
| Memory | <12 MB per instance |
| CPU | <5% (single core, 10 clients) |
| Binary Size | ~12 MB (Linux), ~12.4 MB (Windows) |

---

## Keeping Dependencies Current

```bash
./scripts/build.sh all --update-deps     # Pulls latest uTLS, gorilla, quic-go
```

Recommended: rebuild monthly to keep TLS fingerprints aligned with browser updates.

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| TUN permission denied | `sudo setcap cap_net_admin+ep ./wsvpn-server` (Docker: `--cap-add=NET_ADMIN`) |
| Address in use | `pkill -9 -f wsvpn-server` |
| WebSocket handshake failed | `nginx -t && nginx -s reload` |
| Unauthorized UUID | Add UUID to `clients.json`, reload via SIGHUP |
| DNS failure (Windows) | `ipconfig /flushdns` |

---

## Roadmap

- [ ] QUIC transport with uTLS fingerprint
- [ ] Mobile clients (iOS/Android)
- [ ] GUI desktop clients

---

## License

MIT — See [LICENSE](LICENSE).

## Acknowledgments

- [refraction-networking/utls](https://github.com/refraction-networking/utls) — TLS fingerprint camouflage
- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket
- [quic-go/quic-go](https://github.com/quic-go/quic-go) — QUIC
- [songgao/water](https://github.com/songgao/water) — TUN/TAP
- [Wintun](https://www.wintun.net/) — Windows TUN driver
