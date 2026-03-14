# WSVPN Windows Client Release Process

## Standard Release Package Structure

**Archive Name:** `wsvpn-windows-release-<version>.tar.gz`

**Package Contents:**
```
wsvpn-windows-release-v0.4.1/
├── src/
│   └── main.go                  # Complete source code
├── config/
│   └── client-windows.json      # Configuration template
├── drivers/wintun/
│   ├── amd64/wintun.dll         # 64-bit driver
│   └── arm64/wintun.dll         # ARM64 driver
├── docs/
│   ├── WINDOWS.md               # User guide
│   ├── SECURITY_AUDIT.md        # Security audit report
│   └── CHANGELOG.md             # Version history
├── scripts/
│   └── build-windows.sh         # Build script
└── wsvpn-client.exe             # Compiled binary
```

## Release Steps

### 1. Build Executable
```bash
cd /home/sq/.openclaw/workspace/wsvpn/src
GOOS=windows GOARCH=amd64 go build -o ../build/wsvpn-client-v0.4.1.exe ./client-windows
```

### 2. Create Release Package
```bash
cd /home/sq/.openclaw/workspace/wsvpn
./scripts/release-windows.sh v0.4.1
```

### 3. Verify Package
```bash
# Check archive contents
tar -tzf build/wsvpn-windows-release-v0.4.1.tar.gz

# Check file size (should be ~5-8MB)
ls -lh build/wsvpn-windows-release-v0.4.1.tar.gz
```

### 4. Send Email
```bash
himalaya message send - << 'EOF'
From: windysoaring@gmail.com
To: ntcjoel@gmail.com
Subject: WSVPN Windows Client <version> - Release Ready

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
WSVPN Windows Client <version>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📍 File Location:
  /home/sq/.openclaw/workspace/wsvpn/build/wsvpn-windows-release-<version>.tar.gz
  Size: <size>MB

📋 Package Contents:
  • src/main.go - Complete source code
  • config/client-windows.json - Configuration template
  • drivers/wintun/ - Wintun drivers (amd64 + arm64)
  • wsvpn-client.exe - Compiled binary
  • docs/ - Documentation (WINDOWS.md, SECURITY_AUDIT.md, CHANGELOG.md)
  • scripts/build-windows.sh - Build script

🔧 Key Changes (this version):
  • <list major changes/fixes>

📋 Download:
  scp sq@<host>:~/.openclaw/workspace/wsvpn/build/wsvpn-windows-release-<version>.tar.gz .

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Ellie
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
```

### 5. Update Documentation
- Update `CHANGELOG.md` with new version
- Update `SECURITY_AUDIT.md` if applicable
- Commit changes to git

## Version Naming

**Format:** `v<major>.<minor>.<patch>`

**Examples:**
- `v0.4.0` - Initial Windows release
- `v0.4.1` - Critical bug fixes (routing loop, IP assignment, deadlock)
- `v0.5.0` - New features (Chrome plugin support, etc.)
- `v1.0.0` - Production stable release

**When to increment:**
- **Major** (v1.0.0): Breaking changes, major new features
- **Minor** (v0.5.0): New features, backward compatible
- **Patch** (v0.4.1): Bug fixes, security patches

## Release Checklist

- [ ] Code changes reviewed and tested
- [ ] Security audit completed (if applicable)
- [ ] CHANGELOG.md updated
- [ ] Executable compiled successfully
- [ ] Release package created
- [ ] Package verified (contents + size)
- [ ] Email sent with download location
- [ ] Git commit pushed (if applicable)

## Archive Size Reference

| Version | Size | Notes |
|---------|------|-------|
| v0.4.0 | 7.6MB | Initial release |
| v0.4.1 | 5.8MB | Optimized package (removed duplicate files) |

**Expected size:** 5-8MB (includes wintun.dll drivers ~1MB)

## Email Delivery Notes

**Gmail Attachment Limits:**
- Max attachment size: 25MB
- .tar.gz files may be flagged for security review
- Alternative: Provide SCP download location

**Recommended Email Format:**
- Plain text (mobile-friendly)
- Clear file location path
- SCP command for download
- Brief changelog summary

---

**Last Updated:** 2026-03-06  
**Version:** v0.4.1
