## v0.5.0 (2026-05-22) - Code Quality & Test Coverage

### 🚨 Critical Fixes
- **Server WebSocket Write Mutex** - Added `writeMu sync.Mutex` to server `Client` struct; all `Conn.WriteMessage` calls now protected (prevents concurrent-write panic)
- **Server Route Data Race** - `forwardToClient` now holds `clientsMu.RLock` for the full target lookup + write sequence, preventing `forwardToTUN` defer from removing client mid-write
- **QUIC SNI Configurable** - Added `quic_sni` field to client config; falls back to hostname derived from `server_url` if not set (previously hardcoded to `app.glidesky.org`)

### 📝 Code Quality
- **Structured Logging** - Replaced all legacy `log.Info("legacy", "format %v", err)` format strings in client with proper structured log events (`event name + message + key:value map`)
- **Unit Tests Added** - 7 test cases for `obfuscation` module (padding, HTTPS pattern, jitter, heartbeat); 3 test cases for `client_manager` (load, reload, not-found)

### 📄 Documentation
- Consolidated 11 duplicate/stale docs into authoritative docs: `README.md`, `ARCHITECTURE.md`, `CHANGELOG.md`, `PROJECT_GOALS.md`, `WINDOWS.md`, `SECURITY_AUDIT.md`
- Removed: `windows-deployment-guide.md`, `PROJECT_BRIEF.md`, `COMPLETE_DOCUMENTATION.md`, `STATUS.md`, `WINDOWS_TEST_GUIDE.md`, `RELEASE_PROCESS.md`, `v0.4.2_FIXES.md`, `V0.3_RELEASE.md`, `V0.4_RELEASE.md`, `TEST_REPORT_2026-03-06.md`, `WINDOWS_TEST_REPORT_FINAL.md`, `GEMINI_V0.4.3_ANALYSIS.md`

### Affected Files
- `src/server/main.go` - Client struct, forwardToClient, forwardToTUN
- `src/client/main.go` - Config struct, getSNI(), all log statements
- `config/client.example.json` - Added `quic_sni` field
- `src/obfuscation/padding_test.go` - NEW
- `src/server/client_manager_test.go` - NEW

### Test Results
- ✅ `go test ./obfuscation/ ./server/ -v` — 10/10 tests pass

---

## v0.4.9 (2026-03-07)

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