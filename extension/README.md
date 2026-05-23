# WSVPN Chrome Extension

Smart routing extension that directs traffic through the WSVPN SOCKS5 proxy.

## Features

- **China mainland IP bypass**: Routes traffic to Chinese IPs through the VPN
- **LAN bypass**: Routes LAN addresses (10.x, 192.168.x, 172.16.x) through the VPN
- **Custom CIDR**: User-defined IP ranges to route through the VPN
- **One-click toggle**: Enable/disable from the popup
- **PAC-based routing**: Uses Chrome Proxy API for efficient, declarative routing

## Installation

1. Open Chrome and go to `chrome://extensions`
2. Enable "Developer mode" (top right)
3. Click "Load unpacked" and select the `extension/` directory
4. The WSVPN icon appears in the toolbar
5. Click the icon to configure routing rules

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| Proxy host | 10.9.1.1 | WSVPN server VPN IP |
| Proxy port | 1744 | SOCKS5 proxy port |
| China bypass | On | Route China IPs through VPN |
| LAN bypass | On | Route LAN IPs through VPN |
| Custom CIDR | — | User-defined IP ranges |

## How It Works

The extension uses Chrome's PAC (Proxy Auto-Config) scripting to:
1. Check if the destination IP falls within configured CIDR ranges
2. If yes → route through SOCKS5 proxy (WSVPN tunnel)
3. If no → direct connection

All other traffic goes directly — only specified ranges use the VPN.

## Updating China IP List

The China IP list is embedded in `background.js` (CHINA_CIDR array).
To update with the latest IP allocations:

```bash
curl http://www.ipdeny.com/ipblocks/data/countries/cn.zone | while read cidr; do
  echo "'$cidr',"
done
```

Paste the output into the CHINA_CIDR array in `background.js`.
