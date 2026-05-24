# Chrome Web Store Listing

## Title (max 50 chars)
WSVPN Proxy — Smart PAC Routing

## Short Description (max 132 chars)
Smart PAC routing through WSVPN SOCKS5 proxy. Bypass China mainland, LAN, and custom CIDR ranges with one click.

## Detailed Description

WSVPN Proxy is a lightweight companion extension for the WSVPN VPN service. It uses Chrome's built-in PAC (Proxy Auto-Config) scripting to intelligently route traffic through your WSVPN SOCKS5 proxy.

### Features

- **One-click toggle**: Green = online, gray = offline. Status updates every 30 seconds.
- **China mainland bypass**: Built-in IP range list routes Chinese sites through your VPN.
- **LAN bypass**: Local network addresses routed through VPN.
- **Custom CIDR**: Add your own IP ranges to route through the proxy.
- **Configurable proxy**: Set your own proxy host, port, and routing rules.
- **No data collection**: All settings stored locally. No analytics, no telemetry.

### How It Works

The extension uses Chrome's proxy API with a PAC script. When you visit a website:
1. The extension resolves the domain to an IP address
2. If the IP falls within your configured CIDR ranges (China, LAN, custom), traffic is routed through your SOCKS5 proxy
3. All other traffic goes directly — only specified ranges use the VPN

### Requirements

This extension requires a running WSVPN server with SOCKS5 proxy enabled (default port 1744). See https://github.com/ntcjoel/WSVPN for server setup.

### Privacy

This extension does not collect, store, or transmit any personal data. All configuration is stored locally on your device.

## Category
Productivity

## Language
English

## Screenshots (required)
1280x800 or 640x400 PNG files:
1. Popup showing "Online" state with green indicator
2. Popup with full settings visible
3. (optional) Chrome proxy settings showing PAC configuration

## Promo Images (optional)
Small promo: 440x280
Large promo: 920x680
Marquee promo: 1400x560
