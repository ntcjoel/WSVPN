# Privacy Policy for WSVPN Proxy

**Last updated: 2026-05-24**

## Data Collection

WSVPN Proxy does **not** collect, store, or transmit any personal data. Specifically:

- **No analytics**: No usage tracking, no crash reporting.
- **No telemetry**: No data is sent to any external server.
- **No cookies**: No cookies are set or read.
- **No browsing data**: The extension does not access, log, or monitor your browsing history.

## Data Stored Locally

All configuration data (proxy host, port, routing preferences, CIDR lists) is stored **exclusively on your device** using Chrome's `storage.local` API. This data never leaves your browser.

## Network Access

The extension interacts with your configured SOCKS5 proxy server. The proxy server address and port are set by you. The extension does not connect to any third-party servers.

## Permissions

- `proxy`: Required to configure Chrome's PAC-based proxy routing.
- `storage`: Required to save your settings locally.
- `alarms`: Required for periodic proxy health checks (every 30 seconds).
- `host_permissions`: Required so the PAC script can route all URLs.

## Contact

For questions about this privacy policy, open an issue at:
https://github.com/ntcjoel/WSVPN/issues
