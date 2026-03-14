package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"time"
)

// HealthResponse represents the health endpoint response
type HealthResponse struct {
	Status      string        `json:"status"`
	Uptime      string        `json:"uptime"`
	StartTime   time.Time     `json:"start_time"`
	Clients     ClientStats   `json:"clients"`
	Traffic     TrafficStats  `json:"traffic"`
	System      SystemStats   `json:"system"`
}

// ClientStats represents client statistics
type ClientStats struct {
	Connected     int               `json:"connected"`
	Configured    int               `json:"configured"`
	ClientDetails []ClientDetail    `json:"client_details,omitempty"`
}

// ClientDetail represents details about a connected client
type ClientDetail struct {
	ID          string    `json:"id"`
	IP          string    `json:"ip"`
	ConnectedAt time.Time `json:"connected_at"`
}

// TrafficStats represents traffic statistics
type TrafficStats struct {
	BytesIn    uint64 `json:"bytes_in"`
	BytesOut   uint64 `json:"bytes_out"`
	PacketsIn  uint64 `json:"packets_in"`
	PacketsOut uint64 `json:"packets_out"`
}

// SystemStats represents system metrics
type SystemStats struct {
	Goroutines  int     `json:"goroutines"`
	MemoryAlloc uint64  `json:"memory_alloc_bytes"`
	MemorySys   uint64  `json:"memory_sys_bytes"`
	CPUs        int     `json:"cpus"`
	GoVersion   string  `json:"go_version"`
}

// HandleHealth handles the health endpoint
// GET /ws/health?token=<admin_token>
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Validate admin token
	config := s.getConfig()
	token := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(config.AdminToken)) != 1 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get metrics
	stats := s.metrics.GetStats()

	// Build response
	response := HealthResponse{
		Status:    "healthy",
		Uptime:    stats.Uptime.String(),
		StartTime: stats.StartTime,
		Clients: ClientStats{
			Connected:  stats.ClientCount,
			Configured: s.clientManager.GetClientCount(),
		},
		Traffic: TrafficStats{
			BytesIn:    stats.BytesIn,
			BytesOut:   stats.BytesOut,
			PacketsIn:  stats.PacketsIn,
			PacketsOut: stats.PacketsOut,
		},
		System: SystemStats{
			Goroutines: runtime.NumGoroutine(),
			MemoryAlloc: 0,
			MemorySys:   0,
			CPUs:        runtime.NumCPU(),
			GoVersion:   runtime.Version(),
		},
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	response.System.MemoryAlloc = memStats.Alloc
	response.System.MemorySys = memStats.Sys

	// Get connected client details
	s.clientsMu.RLock()
	for id, client := range s.clients {
		response.Clients.ClientDetails = append(response.Clients.ClientDetails, ClientDetail{
			ID: id,
			IP: client.IPStr,
		})
	}
	s.clientsMu.RUnlock()

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-WSVPN-Version", "0.3")

	// Encode response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode health response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleConfigReload handles manual config reload endpoint
// POST /ws/reload?token=<admin_token>
func (s *Server) HandleConfigReload(w http.ResponseWriter, r *http.Request) {
	// Validate admin token
	config := s.getConfig()
	token := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(config.AdminToken)) != 1 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Reload client configuration
	if err := s.clientManager.Reload(config.ClientsFile); err != nil {
		log.Printf("Config reload failed: %v", err)
		http.Error(w, "Config reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Configuration reloaded successfully via HTTP endpoint")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "reloaded",
		"message": "Configuration reloaded successfully",
	})
}
