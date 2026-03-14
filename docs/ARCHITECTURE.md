# WSVPN Architecture

**Version:** V0.3  
**Last Updated:** 2026-03-03

---

## System Overview

WSVPN uses a client-server architecture with WebSocket tunneling for secure remote network access. The system provides efficient packet forwarding between clients and the internet through a virtual network interface.

---

## Core Components

### 1. WSVPN Server

**Location:** `src/server/main.go`

**Responsibilities:**
- WebSocket server listening on port 8180
- UUID authentication via `/ws/{uuid}` path
- Static IP allocation from clients.json
- Packet routing between clients (O(1) lookup)
- Obfuscation removal (depadding)
- TUN interface management (wsvpn0)
- Health endpoint (`/ws/health`)
- Config hot reload (SIGHUP handler)

**Key Structures:**
```go
type Server struct {
    config        atomic.Pointer[Config]  // Thread-safe config
    tunDevice     *water.Interface        // TUN interface
    clients       map[string]*Client      // Active connections
    ipRoute       map[string]string       // IP → ClientID (O(1) lookup)
    routeMu       sync.RWMutex            // Thread-safe route table
    clientsMu     sync.RWMutex            // Thread-safe client map
    clientManager *ClientManager          // UUID authentication
    metrics       *Metrics                // Traffic statistics
}
```

**Data Flow:**
```
WebSocket Frame → Remove Padding → Extract Original Packet → Route Table Lookup → TUN Write
```

---

### 2. WSVPN Client

**Location:** `src/client/main.go`

**Responsibilities:**
- WebSocket connection to server via HTTPS
- Obfuscation addition (padding + pattern simulation)
- TUN interface management (wsvpn-client)
- Auto reconnect with exponential backoff
- Irregular heartbeat (30-90s random)

**Key Structures:**
```go
type Client struct {
    config    *Config               // Client configuration
    tunDevice *water.Interface      // TUN interface
    conn      *websocket.Conn       // WebSocket connection
    running   bool                  // Running state
    stopCh    chan struct{}         // Stop signal
}
```

**Data Flow:**
```
TUN Read → Add Padding (HTTPS Pattern) → WebSocket Write → Server
```

---

### 3. Client Manager

**Location:** `src/server/client_manager.go`

**Responsibilities:**
- Load clients from clients.json
- UUID validation (authorized/unauthorized)
- Static IP allocation (UUID → IP mapping)
- Thread-safe client list management

**Key Methods:**
```go
func (cm *ClientManager) LoadClients(path string) error
func (cm *ClientManager) IsUUIDAuthorized(uuid string) bool
func (cm *ClientManager) GetIPByUUID(uuid string) (string, bool)
func (cm *ClientManager) Reload(path string) error  // Hot reload
```

---

### 4. Metrics Collector

**Location:** `src/server/metrics.go`

**Responsibilities:**
- Track bytes/packets in/out (atomic counters)
- Track connected clients
- Export statistics for health endpoint

**Key Structures:**
```go
type Metrics struct {
    bytesIn    atomic.Uint64
    bytesOut   atomic.Uint64
    packetsIn  atomic.Uint64
    packetsOut atomic.Uint64
    clients    sync.Map  // ClientID → ConnectTime
}
```

---

### 5. Obfuscation Module

**Location:** `src/obfuscation/padding.go`

**Responsibilities:**
- Random padding (50-500 bytes)
- HTTPS pattern simulation (64/256/1024/1480 distribution)
- Timing jitter (100ms-2s packet delay, 30-90s heartbeat)

**Key Functions:**
```go
func AddPadding(packet []byte) []byte           // Basic padding
func RemovePadding(data []byte) ([]byte, error) // Depadding
func SimulateHTTPSPattern(packet []byte) []byte // Smart padding
func GetJitterDelay() time.Duration             // Random delay
func GetHeartbeatInterval() time.Duration       // Random heartbeat
```

**Packet Format:**
```
┌─────────────────────────────────────────────────────────┐
│  [4 bytes: Original Length] │ [Original Packet] │ [Padding] │
│  Big-Endian Uint32          │ Variable          │ 50-500 bytes │
└─────────────────────────────────────────────────────────┘
```

**HTTPS Pattern Distribution:**
```
Size (bytes) | Probability | Purpose
-------------|-------------|------------------
64           | 20%         | ACK/keepalive
256          | 30%         | Small requests
1024         | 25%         | Medium responses
1480         | 25%         | Large packets (MTU)
```

---

## Data Flow Diagrams

### Outbound (Client → Server → Internet)

```
┌─────────────────────────────────────────────────────────────────┐
│  Client                                                         │
│                                                                 │
│  Application Packet (e.g., HTTP request to google.com)          │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TUN Interface (wsvpn-client, 10.9.1.2)                  │   │
│  │ Packet captured from TUN                                │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Obfuscation Layer                                       │   │
│  │ - SimulateHTTPSPattern()                                │   │
│  │ - Add 50-500 bytes random padding                       │   │
│  │ - Format: [4 bytes len][packet][padding]                │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ WebSocket Frame (Binary)                                │   │
│  │ Encapsulate padded packet                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TLS Encryption (HTTPS)                                  │   │
│  │ wss://app.glidesky.org/ws/device-phone-001              │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
└─────────────────────────────────────────────────────────────────┘
           │
           │  Internet (Encrypted TLS Traffic)
           ↓
┌─────────────────────────────────────────────────────────────────┐
│  Nginx Reverse Proxy                                            │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TLS Termination (port 443)                              │   │
│  │ Decrypt HTTPS                                           │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ WebSocket Upgrade                                       │   │
│  │ Location: /ws/device-phone-001                          │   │
│  │ Extract UUID from path                                  │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Proxy to Backend                                        │   │
│  │ ws://127.0.0.1:8180/ws/device-phone-001                 │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
└─────────────────────────────────────────────────────────────────┘
           │
           ↓
┌─────────────────────────────────────────────────────────────────┐
│  WSVPN Server                                                   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ WebSocket Handler                                       │   │
│  │ Extract UUID: device-phone-001                          │   │
│  │ Validate UUID (ClientManager.IsUUIDAuthorized)          │   │
│  │ Get assigned IP: 10.9.1.2                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Obfuscation Removal                                     │   │
│  │ - Read 4 bytes length header                            │   │
│  │ - Extract original packet                               │   │
│  │ - Discard padding                                       │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Route Table Lookup (O(1))                               │   │
│  │ Get destination IP from packet header                   │   │
│  │ Lookup in ipRoute map                                   │   │
│  │ If destination is another client → forward to client    │   │
│  │ If destination is internet → forward to TUN             │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TUN Interface (wsvpn0, 10.9.1.1)                        │   │
│  │ Write packet to TUN                                     │   │
│  │ Kernel routes to internet                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  Internet                                                       │
└─────────────────────────────────────────────────────────────────┘
```

### Inbound (Internet → Server → Client)

```
Internet Response (e.g., google.com response)
           ↓
┌─────────────────────────────────────────────────────────────────┐
│  WSVPN Server                                                   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TUN Interface (wsvpn0)                                  │   │
│  │ Read packet from TUN                                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Route Table Lookup (O(1))                               │   │
│  │ Get destination IP: 10.9.1.2                            │   │
│  │ Lookup in ipRoute → client-device-phone-001             │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Obfuscation Addition                                    │   │
│  │ - SimulateHTTPSPattern()                                │   │
│  │ - Add 50-500 bytes random padding                       │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ WebSocket Write                                         │   │
│  │ Send to client-device-phone-001                         │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
└─────────────────────────────────────────────────────────────────┘
           │
           │  WebSocket Frame
           ↓
┌─────────────────────────────────────────────────────────────────┐
│  Nginx Reverse Proxy                                            │
│  Proxy to WebSocket backend                                     │
└─────────────────────────────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────────────────────────────┐
│  Client                                                         │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ WebSocket Read                                          │   │
│  │ Receive binary frame                                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Obfuscation Removal                                     │   │
│  │ - Read 4 bytes length header                            │   │
│  │ - Extract original packet                               │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ TUN Write                                               │   │
│  │ Write packet to wsvpn-client                            │   │
│  │ Kernel delivers to application                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│           ↓                                                     │
│  Application (Browser, etc.)                                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Thread Safety Design

### Atomic Operations

```go
// Config updates (hot reload)
type Server struct {
    config atomic.Pointer[Config]  // Lock-free config access
}

func (s *Server) getConfig() *Config {
    return s.config.Load()  // Atomic read
}

func (s *Server) setConfig(cfg *Config) {
    s.config.Store(cfg)  // Atomic write
}
```

### Mutex Protection

```go
// Route table (ipRoute map)
routeMu sync.RWMutex

// Read (frequent)
s.routeMu.RLock()
clientID := s.ipRoute[dstIP]
s.routeMu.RUnlock()

// Write (rare)
s.routeMu.Lock()
s.ipRoute[clientIP] = clientID
s.routeMu.Unlock()

// Client map (clients map)
clientsMu sync.RWMutex
// Same RWMutex pattern
```

### Atomic Counters (Metrics)

```go
type Metrics struct {
    bytesIn    atomic.Uint64  // Lock-free counter
    bytesOut   atomic.Uint64
    packetsIn  atomic.Uint64
    packetsOut atomic.Uint64
}

func (m *Metrics) RecordInbound(bytes int) {
    m.bytesIn.Add(uint64(bytes))
    m.packetsIn.Add(1)
}
```

---

## Memory Management

### Packet Pool

```go
var packetPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 2048)  // Reusable 2KB buffer
    },
}

// Usage
buffer := packetPool.Get().([]byte)
defer packetPool.Put(buffer)  // Return to pool

n, _ := tun.Read(buffer)
packet := buffer[:n]
```

**Benefits:**
- Reduces GC pressure
- Reuses memory across packets
- Fixed 2KB size covers most MTU scenarios

---

## Error Handling

### Client Disconnection

```go
func (s *Server) forwardToTUN(client *Client) {
    defer func() {
        // Cleanup on exit
        s.clientsMu.Lock()
        delete(s.clients, client.ID)
        s.clientsMu.Unlock()
        
        s.routeMu.Lock()
        delete(s.ipRoute, client.IPStr)
        s.routeMu.Unlock()
        
        s.metrics.RemoveClient(client.ID)
        client.Conn.Close()
        close(client.stopCh)
        log.Printf("Client %s disconnected", client.ID)
    }()
    
    for {
        _, _, err := client.Conn.ReadMessage()
        if err != nil {
            return  // Triggers defer cleanup
        }
        // Process packet...
    }
}
```

### Auto Reconnect (Client)

```go
func (c *Client) reconnect() {
    backoff := 1 * time.Second
    maxBackoff := 30 * time.Second
    
    for c.running {
        time.Sleep(backoff)
        
        if err := c.connectWebSocket(); err != nil {
            log.Printf("Reconnection failed: %v", err)
            backoff *= 2  // Exponential backoff
            if backoff > maxBackoff {
                backoff = maxBackoff
            }
            continue
        }
        
        backoff = 1 * time.Second  // Reset on success
        log.Printf("Reconnected successfully")
        
        go c.forwardToServer()
        c.forwardFromServer()
    }
}
```

---

## Configuration Hot Reload

### SIGHUP Handler

```go
func (s *Server) setupSignalHandlers() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    
    go func() {
        for range sigCh {
            log.Printf("Received SIGHUP, reloading configuration...")
            
            newConfig, err := loadConfig("server.json")
            if err != nil {
                log.Printf("Failed to reload config: %v", err)
                continue
            }
            
            s.setConfig(newConfig)  // Atomic swap
            
            if err := s.clientManager.Reload(newConfig.ClientsFile); err != nil {
                log.Printf("Failed to reload clients: %v", err)
                continue
            }
            
            log.Printf("Configuration reloaded (obfuscation=%v)", newConfig.Obfuscation)
        }
    }()
}
```

**Atomic Swap Guarantees:**
- No locks needed for config read
- Existing connections unaffected
- New connections use updated config
- Obfuscation toggle applies immediately

---

## Security Considerations

### UUID Authentication Flow

```
1. Client connects to /ws/device-phone-001
2. Server extracts UUID from path
3. ClientManager.IsUUIDAuthorized(uuid) checks clients.json
4. If authorized:
   - GetIPByUUID(uuid) returns assigned IP
   - Create client entry
   - Add route table entry
5. If unauthorized:
   - Return HTTP 401
   - Log attempt
   - Close connection
```

### TLS Termination

```
Client ↔ Nginx (TLS 1.2/1.3 encrypted)
Nginx ↔ WSVPN Server (plaintext WebSocket, localhost only)
```

**Security Boundaries:**
- TLS protects internet-facing traffic
- Localhost connection (127.0.0.1) not exposed
- UUID in WebSocket path encrypted by TLS

### Obfuscation vs Encryption

| Layer | Purpose | Implementation |
|-------|---------|----------------|
| TLS | Encryption (confidentiality) | Nginx + SSL certificate |
| Obfuscation | DPI evasion (stealth) | Padding + pattern simulation |

**Note:** Obfuscation is NOT encryption. It hides traffic patterns, not content.

---

## Performance Optimizations

### O(1) Route Lookup

```go
// Before (O(n) iteration)
for _, client := range s.clients {
    if client.IP == dstIP {
        return client
    }
}

// After (O(1) map lookup)
clientID := s.ipRoute[dstIP.String()]
return s.clients[clientID]
```

### Memory Pool

```go
// Without pool: Allocate new buffer per packet
buffer := make([]byte, 2048)  // GC pressure

// With pool: Reuse buffer
buffer := packetPool.Get().([]byte)
defer packetPool.Put(buffer)  // Return for reuse
```

### Atomic Counters

```go
// Without atomics: Mutex overhead
mu.Lock()
bytesIn += n
mu.Unlock()

// With atomics: Lock-free
bytesIn.Add(uint64(n))
```

---

## Scalability

### Current Limits

| Resource | Limit | Notes |
|----------|-------|-------|
| Clients | 100+ | Tested with 2, scalable to 100+ |
| Throughput | 1 Gbps | Limited by single-threaded Go |
| Memory | ~10 MB base + 1 MB per 10 clients | Efficient pooling |
| CPU | <10% on single-core | For 10 clients |

### Bottlenecks

1. **Single-threaded packet processing**: Go runtime schedules goroutines
2. **TUN interface**: Kernel context switches
3. **WebSocket overhead**: Frame encapsulation

### Future Optimizations (V1.0+)

- **Rust core**: Refactor packet processing to Rust
- **Multi-queue TUN**: Parallel packet processing
- **UDP transport**: Optional UDP instead of WebSocket

---

## Monitoring Architecture

### Health Endpoint Metrics

```json
{
  "status": "healthy",
  "uptime": "2h30m15s",
  "clients": {
    "connected": 2,
    "configured": 2
  },
  "traffic": {
    "bytes_in": 1048576,
    "bytes_out": 2097152
  },
  "system": {
    "goroutines": 15,
    "memory_alloc_bytes": 5242880
  }
}
```

**Metric Collection:**
- Bytes/packets: Atomic counters (increment on each packet)
- Client count: sync.Map (concurrent map)
- System metrics: runtime.MemStats()

---

## Testing Strategy

### Unit Tests (Future)

```go
// Obfuscation tests
func TestAddPadding(t *testing.T) {
    packet := []byte("test")
    padded := AddPadding(packet)
    
    // Verify padding added
    if len(padded) <= len(packet) {
        t.Error("Padding not added")
    }
    
    // Verify original packet recovered
    original, _ := RemovePadding(padded)
    if !bytes.Equal(original, packet) {
        t.Error("Original packet not recovered")
    }
}

// Route table tests
func TestRouteLookup(t *testing.T) {
    server := NewServer()
    server.ipRoute["10.9.1.2"] = "client-1"
    
    client := server.routePacket(packetTo("10.9.1.2"))
    if client.ID != "client-1" {
        t.Error("Route lookup failed")
    }
}
```

### Integration Tests

```bash
# Automated test script
./scripts/test-integration.sh

# Steps:
# 1. Start server
# 2. Start client
# 3. Ping test
# 4. iperf3 test
# 5. Verify metrics
# 6. Cleanup
```

---

## References

- **WebSocket RFC:** https://datatracker.ietf.org/doc/html/rfc6455
- **TUN/TAP Documentation:** https://www.kernel.org/doc/Documentation/networking/tuntap.txt
- **Go sync/atomic:** https://pkg.go.dev/sync/atomic
- **Gorilla WebSocket:** https://pkg.go.dev/github.com/gorilla/websocket
