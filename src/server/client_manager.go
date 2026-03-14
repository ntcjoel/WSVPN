package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ClientConfig represents a single client configuration
type ClientConfig struct {
	UUID    string `json:"uuid"`
	IP      string `json:"ip"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ClientsConfig represents the full clients configuration
type ClientsConfig struct {
	Clients       []ClientConfig `json:"clients"`
	Network       string         `json:"network"`
	NextDynamicIP int            `json:"next_dynamic_ip"`
}

// ClientManager handles UUID to IP mapping and client authentication
type ClientManager struct {
	clients map[string]*ClientConfig // UUID -> ClientConfig
	ipRoute map[string]string        // IP -> UUID
	mu      sync.RWMutex
	config  *ClientsConfig
}

// NewClientManager creates a new ClientManager
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*ClientConfig),
		ipRoute: make(map[string]string),
	}
}

// LoadClients loads client configurations from JSON file
func (cm *ClientManager) LoadClients(path string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read clients file: %w", err)
	}

	var config ClientsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse clients config: %w", err)
	}

	cm.config = &config
	cm.clients = make(map[string]*ClientConfig)
	cm.ipRoute = make(map[string]string)

	// Index clients by UUID and IP
	for i := range config.Clients {
		client := &config.Clients[i]
		if client.Enabled {
			cm.clients[client.UUID] = client
			cm.ipRoute[client.IP] = client.UUID
		}
	}

	return nil
}

// GetClientByUUID retrieves a client configuration by UUID
func (cm *ClientManager) GetClientByUUID(uuid string) (*ClientConfig, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[uuid]
	if !exists {
		return nil, false
	}

	return client, true
}

// GetIPByUUID returns the IP address for a given UUID
func (cm *ClientManager) GetIPByUUID(uuid string) (string, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[uuid]
	if !exists {
		return "", false
	}

	return client.IP, true
}

// GetUUIDByIP returns the UUID for a given IP
func (cm *ClientManager) GetUUIDByIP(ip string) (string, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	uuid, exists := cm.ipRoute[ip]
	return uuid, exists
}

// IsUUIDAuthorized checks if a UUID is authorized to connect
func (cm *ClientManager) IsUUIDAuthorized(uuid string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[uuid]
	if !exists {
		return false
	}

	return client.Enabled
}

// GetClientCount returns the number of configured clients
func (cm *ClientManager) GetClientCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.clients)
}

// GetNetwork returns the configured network CIDR
func (cm *ClientManager) GetNetwork() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.config == nil {
		return ""
	}
	return cm.config.Network
}

// Reload reloads the client configuration from disk
func (cm *ClientManager) Reload(path string) error {
	return cm.LoadClients(path)
}
