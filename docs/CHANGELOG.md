## v0.4.9 (2026-03-07) - Comprehensive Stability Release

### 🚨 Critical Fixes (Consolidated)
- **WebSocket Mutex Protection** - Prevents concurrent write panic (CRITICAL FIX #1)
- **Error Channel Buffer** - Buffer size=3 prevents goroutine leak (CRITICAL FIX #3)
- **Non-blocking Error Send** - `sendError()` uses select with default case

### 📝 Code Quality
- Unified version constants across all components (client/server/windows)
- Added `-version` flag to all binaries
- Version logging on startup

### Affected Files
- `src/client/main.go` - Version constant, -version flag, mutex protection
- `src/server/main.go` - Version constant, -version flag
- `src/client-windows/main.go` - Consolidated fixes

### Test Results
- ✅ tc.vps stability test: 60/60 packets, 0% loss, RTT ~140ms
- ✅ Cross-platform consistency (Linux/Windows)

---

## v0.4.8 (2026-03-07) - Goroutine Leak Fix

### 🚨 Critical Fixes
- **Error Channel Buffer Size** - Changed from unbuffered to buffer=3
- **Prevents Goroutine Leak** - Non-blocking error sends in 3 goroutines

### Affected Files
- `src/client-windows/main.go` - errCh buffer, sendError() implementation

---

## v0.4.7 (2026-03-06) - WebSocket Concurrency Fix

### 🚨 Critical Fixes
- **WebSocket Mutex** - Added `wsWriteMu sync.Mutex` to protect concurrent writes
- **Prevents Panic** - "concurrent write to websocket connection" eliminated

### Affected Files
- `src/client-windows/main.go` - Client struct, writePacket() function

---

## v0.4.6 (2026-03-06) - Dual Transport + CLI Flags

### ✨ New Features
- **WebSocket + QUIC Dual Transport** - Configurable via `transport` field
- **Command-line Flags** - `-config` and `-clients` flags for server/client
- **Version Reporting** - `-version` flag shows build info

### 📝 Configuration
- `transport` field: `websocket`, `quic`, or `both`
- `quic_sni` field for QUIC SNI override (client)
- `dns` field for custom DNS (client)

### Affected Files
- `src/client/main.go` - CLI flags, QUIC support
- `src/server/main.go` - CLI flags, QUIC listener
- `src/client-windows/main.go` - Windows QUIC implementation

### Test Results
- ✅ v0.4.6 full test report: 0% packet loss, 48.9 Mbps throughput
- ✅ Windows TUN connectivity verified

---

## v0.4.5 (2026-03-06) - Route & Cleanup Fixes

### 🚨 Critical Fixes
- Temporary routes (removed `-p` flag, prevents permanent route pollution)
- Route cleanup on disconnect (restores network state)

### 📝 Code Quality
- Removed dead code (`runtime.GOOS` check in Windows-only build)

### Affected Files
- `src/client-windows/main.go`

---

## v0.4.4 (2026-03-06) - Security Fix

### 🔒 Security Fixes
- Constant-time comparison for admin token (prevents timing attack)
- Using `crypto/subtle.ConstantTimeCompare` instead of `!=` in handlers

### Affected Files
- `src/server/handlers.go`