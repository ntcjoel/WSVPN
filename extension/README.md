# WSVPN Chrome Extension

Smart PAC-based proxy routing extension.

## Features

- Green/gray status indicator (online/offline)
- Configurable proxy host and port
- China mainland IP bypass
- LAN IP bypass
- Custom CIDR ranges
- No data collection — all settings local

## Install (Developer Mode)

1. Open `chrome://extensions`
2. Enable "Developer mode"
3. Click "Load unpacked" → select the `extension/` directory

## Publish to Chrome Web Store

1. Register at https://chrome.google.com/webstore/devconsole ($5 one-time fee)
2. Zip the extension directory: `zip -r wsvpn-proxy.zip extension/ -x "*.md"`
3. Upload to Chrome Web Store Developer Dashboard
4. Fill in store listing using `store-listing.md`
5. Submit for review (typically 1-3 business days)

## Files

```
extension/
├── manifest.json        # Manifest V3
├── background.js        # Service worker: PAC, health check, icon
├── popup.html           # Settings UI
├── popup.js             # UI logic
├── icon16.png           # Green circle (16px)
├── icon16-off.png       # Gray circle (16px)
├── icon48.png           # Green circle (48px)
├── icon48-off.png       # Gray circle (48px)
├── icon128.png          # Green circle (128px)
├── icon128-off.png      # Gray circle (128px)
├── privacy.md           # Privacy policy
├── store-listing.md     # Store listing draft
└── README.md
```
