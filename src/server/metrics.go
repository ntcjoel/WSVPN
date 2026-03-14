package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks server statistics
type Metrics struct {
	bytesIn     atomic.Uint64
	bytesOut    atomic.Uint64
	packetsIn   atomic.Uint64
	packetsOut  atomic.Uint64
	clientCount atomic.Int64
	startTime   time.Time
	mu          sync.RWMutex
	connectedIDs map[string]time.Time // client ID -> connection time
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		startTime:    time.Now(),
		connectedIDs: make(map[string]time.Time),
	}
}

// RecordInbound records an inbound packet
func (m *Metrics) RecordInbound(bytes int) {
	m.bytesIn.Add(uint64(bytes))
	m.packetsIn.Add(1)
}

// RecordOutbound records an outbound packet
func (m *Metrics) RecordOutbound(bytes int) {
	m.bytesOut.Add(uint64(bytes))
	m.packetsOut.Add(1)
}

// AddClient records a new client connection
func (m *Metrics) AddClient(id string) {
	m.mu.Lock()
	m.connectedIDs[id] = time.Now()
	m.mu.Unlock()
	m.clientCount.Add(1)
}

// RemoveClient records a client disconnection
func (m *Metrics) RemoveClient(id string) {
	m.mu.Lock()
	delete(m.connectedIDs, id)
	m.mu.Unlock()
	m.clientCount.Add(-1)
}

// GetStats returns current statistics
func (m *Metrics) GetStats() Stats {
	m.mu.RLock()
	connectedIDs := make(map[string]time.Time)
	for k, v := range m.connectedIDs {
		connectedIDs[k] = v
	}
	m.mu.RUnlock()

	return Stats{
		BytesIn:     m.bytesIn.Load(),
		BytesOut:    m.bytesOut.Load(),
		PacketsIn:   m.packetsIn.Load(),
		PacketsOut:  m.packetsOut.Load(),
		ClientCount: int(m.clientCount.Load()),
		Uptime:      time.Since(m.startTime),
		StartTime:   m.startTime,
		Clients:     connectedIDs,
	}
}

// Stats represents a snapshot of server statistics
type Stats struct {
	BytesIn     uint64
	BytesOut    uint64
	PacketsIn   uint64
	PacketsOut  uint64
	ClientCount int
	Uptime      time.Duration
	StartTime   time.Time
	Clients     map[string]time.Time
}
