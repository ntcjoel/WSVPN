# WSVPN — WebSocket VPN

**Version:** v1.2 &nbsp;|&nbsp; **License:** MIT &nbsp;|&nbsp; [Download](https://github.com/ntcjoel/WSVPN/releases)

---

## Overview

WSVPN tunnels IP traffic through standard WebSocket-over-TLS (WSS) — indistinguishable from normal HTTPS. Advanced obfuscation resists deep packet inspection and traffic analysis.

---

## Download

Pre-built binaries are available on the [Releases page](https://github.com/ntcjoel/WSVPN/releases):

| File | Platform | Description |
|------|----------|-------------|
| `wsvpn-server` | Linux amd64 | Server binary |
| `wsvpn-client` | Linux amd64 | CLI client |
| `wsvpn-client.exe` | Windows amd64 | CLI client (terminal) |
| `wsvpn-client-gui.exe` | Windows amd64 | GUI client (systray icon) |

---

## Build from Source

### Prerequisites

- Go 1.21+
- Linux: `gcc` (CGO required for TUN)
- Windows: none (pure Go cross-compile)

### Linux (server + client)

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn/src

# Server
CGO_ENABLED=1 go build -o wsvpn-server ./server

# Client
CGO_ENABLED=1 go build -o wsvpn-client ./client
```

### Windows (cross-compile from Linux)

```bash
cd wsvpn/src

# CLI version (smaller, no GUI)
GOOS=windows GOARCH=amd64 go build -tags cli -ldflags="-s -w" -o wsvpn-client.exe ./client-windows

# GUI version (systray icon)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o wsvpn-client-gui.exe ./client-windows
```

Or use the build scripts:

```bash
./scripts/build.sh all --update-deps     # Linux server + client
./scripts/build-windows.sh --update-deps # Windows client
```

The `--update-deps` flag pulls latest uTLS/gorilla/quic-go to keep browser fingerprints current.

---

## Server Setup

### Option A: Docker Compose (recommended)

```bash
git clone https://github.com/ntcjoel/wsvpn.git
cd wsvpn

# Create config files
cp config/server.example.json config/server.json
cp config/clients.example.json config/clients.json

# Edit server.json:
#   - Set a strong admin_token (used for admin panel + health API)
#   - Keep obfuscation: true
# Edit clients.json:
#   - Add your client UUIDs and IPs

# Build and start
docker compose up -d

# Verify
curl "http://127.0.0.1:8180/ws/health?token=your-admin-token"
```

The Docker container:
- Uses `network_mode: host` — binds directly to the host's network
- Requires `NET_ADMIN` capability for TUN interface
- Reads config from `./config/` mounted read-only
- Restarts automatically (`unless-stopped`)

### Option B: Native Binary

```bash
# Copy binary and config
scp wsvpn-server user@server:~/wsvpn/
scp config/server.json config/clients.json user@server:~/wsvpn/

# Set capability (avoids running as root)
ssh user@server "sudo setcap cap_net_admin+ep ~/wsvpn/wsvpn-server"

# Start
ssh user@server "cd ~/wsvpn && nohup ./wsvpn-server > server.log 2>&1 &"
```

### Nginx Reverse Proxy

The server listens on plain HTTP (`:8180`). Put nginx in front for TLS:

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # WebSocket tunnel
    location ~ "^/ws/([a-zA-Z0-9_-]{8,64})$" {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_pass http://127.0.0.1:8180;
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }

    # Health + admin
    location /ws/health   { proxy_pass http://127.0.0.1:8180/ws/health; proxy_http_version 1.1; proxy_set_header Host $host; }
    location /ws/reload   { proxy_pass http://127.0.0.1:8180/ws/reload; proxy_http_version 1.1; proxy_set_header Host $host; }
    location /ws/admin    { proxy_pass http://127.0.0.1:8180/ws/admin;  proxy_http_version 1.1; proxy_set_header Host $host; }
    location /ws/admin/api/ { proxy_pass http://127.0.0.1:8180/ws/admin/api/; proxy_http_version 1.1; proxy_set_header Host $host; }
}
```

### Server Configuration

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
| `obfuscation` | `true` | Enable packet obfuscation |
| `socks5_enabled` | `true` | Built-in SOCKS5 proxy on VPN IP |
| `socks5_port` | `1744` | SOCKS5 proxy port |
| `admin_token` | — | Auth for `/ws/health`, `/ws/admin`, `/ws/reload` |

### Clients Configuration (`clients.json`)

```json
{
  "clients": [
    {"uuid": "my-phone-001", "ip": "10.9.1.2", "name": "iPhone", "enabled": true},
    {"uuid": "my-laptop-002", "ip": "10.9.1.3", "name": "Laptop", "enabled": true}
  ],
  "network": "10.9.1.0/24",
  "next_dynamic_ip": 50
}
```

Each client gets a unique UUID and a fixed IP in the VPN subnet.

---

## Client Setup

### Linux

```bash
# Copy binary and config
scp wsvpn-client user@client:~/wsvpn-client/
scp config/client.json user@client:~/wsvpn-client/

# Set capability
ssh user@client "sudo setcap cap_net_admin+ep ~/wsvpn-client/wsvpn-client"

# Start
ssh user@client "cd ~/wsvpn-client && nohup ./wsvpn-client > client.log 2>&1 &"

# Verify
ping 10.9.1.1
```

### Windows CLI

```cmd
# Start VPN (foreground, terminal output)
wsvpn-client.exe

# Custom config file
wsvpn-client.exe -config my-config.json

# Show version
wsvpn-client.exe -version
```

### Windows GUI

Double-click `wsvpn-client-gui.exe` — a tray icon appears in the taskbar.

Right-click the icon:
- **Connect** / **Disconnect** — Start or stop VPN
- **Settings...** — Opens `client.json` in Notepad for editing
- **Reload Config** — Reconnects with updated settings
- **Exit** — Quit

Mouse over the icon to see connection status.

### Client Configuration

```json
{
  "name": "wsvpn-client",
  "client_ip": "10.9.1.2",
  "server_url": "wss://your-domain.com",
  "uuid": "my-phone-001",
  "reconnect": true,
  "log_level": "info",
  "obfuscation": true,
  "transport": "websocket",
  "tls_fingerprint": "chrome",
  "traffic_shape": "off",
  "front_domain": "",
  "dispersion_peers": [],
  "traffic_induction": false,
  "induction_domains": []
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `server_url` | — | Server address (`wss://domain` or `ws://ip:port`) |
| `uuid` | — | Must match an entry in server's `clients.json` |
| `client_ip` | — | Must match the IP assigned in `clients.json` |
| `obfuscation` | `true` | Enable packet obfuscation |
| `tls_fingerprint` | `chrome` | `chrome` / `firefox` / `ios` / `edge` / `random` |
| `traffic_shape` | `off` | `off` / `jitter` / `browse` / `adaptive` |
| `front_domain` | `""` | CDN domain for domain fronting |
| `dispersion_peers` | `[]` | Extra server URLs for traffic dispersion |
| `traffic_induction` | `false` | Background HTTP noise during idle |

---

## Admin Panel

The server includes a web-based management UI at `/ws/admin`:

```
https://your-domain.com/ws/admin?token=your-admin-token
```

Features:
- Real-time server status (uptime, connections, traffic)
- Connected clients list (status, IP, UUID)
- Add / edit / delete client configurations
- Enable / disable clients
- Changes take effect immediately (auto hot-reload)

---

## SOCKS5 Proxy

The server exposes a SOCKS5 proxy on the VPN IP. Connected clients can use it to route traffic through the server:

```bash
curl --socks5 10.9.1.1:1744 https://example.com
```

---

## Monitoring

```bash
# Health check
curl "https://your-domain.com/ws/health?token=your-admin-token"
# → {"status":"healthy","clients":{"connected":2},"traffic":{...}}

# Hot reload (after editing clients.json)
curl -X POST "https://your-domain.com/ws/reload?token=your-admin-token"
```

---

## Anti-DPI Design

| Layer | Mechanism | DPI defeats |
|-------|-----------|-------------|
| **TLS Fingerprint** | uTLS mimics Chrome/Firefox/iOS/Edge JA3/JA4 | Protocol identification |
| **Traffic Shaping** | Burst/pause browsing simulation | Continuous-flow detection |
| **Packet Obfuscation** | Randomized header + HTTPS-sized padding | Fixed-pattern detection |
| **Domain Fronting** | CDN SNI + real Host header | Destination identification |
| **Multi-Connection** | Round-robin across multiple servers | Single-domain anomaly |
| **Traffic Induction** | Background HTTP noise during idle | Idle tunnel detection |

---

## Directory Layout

```
wsvpn/
├── docker-compose.yml      # Docker Compose config
├── Dockerfile              # Docker image build
├── src/                    # Go source code
│   ├── server/             # Server (main.go, handlers.go, socks5.go, admin.go, ...)
│   ├── client/             # Linux client
│   ├── client-windows/     # Windows client (CLI + GUI)
│   └── obfuscation/        # Shared obfuscation library
├── config/                 # Example config files
│   ├── server.example.json
│   ├── clients.example.json
│   └── client.example.json
├── scripts/                # Build scripts
├── extension/              # Chrome extension
└── nginx/                  # Nginx config for Docker
```

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `operation not permitted` | `sudo setcap cap_net_admin+ep` or Docker `--cap-add=NET_ADMIN` |
| `Address already in use` | Kill existing process or change port in config |
| `WebSocket handshake failed` | Check nginx config: `nginx -t && nginx -s reload` |
| `Unauthorized UUID` | Add UUID to `clients.json`, reload via admin panel or SIGHUP |
| DNS failure (Windows) | `ipconfig /flushdns` |
| GUI won't start | Windows GUI needs `wintun.dll` in the same directory |

---

## Dependencies

```bash
./scripts/build.sh all --update-deps  # Pulls latest versions
```

Rebuild monthly to keep uTLS browser fingerprints current.

---

## License

MIT — See [LICENSE](LICENSE).

## Acknowledgments

- [refraction-networking/utls](https://github.com/refraction-networking/utls) — TLS fingerprint camouflage
- [gorilla/websocket](https://github.com/gorilla/websocket) — WebSocket
- [songgao/water](https://github.com/songgao/water) — TUN/TAP
- [Wintun](https://www.wintun.net/) — Windows TUN driver
