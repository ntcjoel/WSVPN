package main

import (
	"os"
	"testing"
)

func TestClientManagerLoadClients(t *testing.T) {
	// Create a temporary clients.json
	content := `{
		"clients": [
			{
				"uuid": "device-001",
				"ip": "10.9.1.2",
				"name": "iPhone",
				"enabled": true
			},
			{
				"uuid": "device-002",
				"ip": "10.9.1.3",
				"name": "MacBook",
				"enabled": false
			},
			{
				"uuid": "device-003",
				"ip": "10.9.1.4",
				"name": "Android",
				"enabled": true
			}
		],
		"network": "10.9.1.0/24",
		"next_dynamic_ip": 50
	}`

	tmpFile, err := os.CreateTemp("", "clients-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cm := NewClientManager()
	if err := cm.LoadClients(tmpFile.Name()); err != nil {
		t.Fatalf("LoadClients() error = %v", err)
	}

	// Test GetClientCount — only enabled clients (2)
	count := cm.GetClientCount()
	if count != 2 {
		t.Errorf("GetClientCount() = %d, want 2 (device-001 and device-003, device-002 disabled)", count)
	}

	// Test IsUUIDAuthorized
	if !cm.IsUUIDAuthorized("device-001") {
		t.Errorf("IsUUIDAuthorized(device-001) = false, want true")
	}
	if cm.IsUUIDAuthorized("device-002") {
		t.Errorf("IsUUIDAuthorized(device-002) = true, want false (disabled)")
	}
	if !cm.IsUUIDAuthorized("device-003") {
		t.Errorf("IsUUIDAuthorized(device-003) = false, want true")
	}
	if cm.IsUUIDAuthorized("unknown") {
		t.Errorf("IsUUIDAuthorized(unknown) = true, want false")
	}

	// Test GetIPByUUID
	ip, ok := cm.GetIPByUUID("device-001")
	if !ok || ip != "10.9.1.2" {
		t.Errorf("GetIPByUUID(device-001) = %q, ok=%v, want 10.9.1.2, true", ip, ok)
	}
	_, ok = cm.GetIPByUUID("device-002")
	if ok {
		t.Errorf("GetIPByUUID(device-002) = _, ok=true, want false (disabled)")
	}
	_, ok = cm.GetIPByUUID("unknown")
	if ok {
		t.Errorf("GetIPByUUID(unknown) = _, ok=true, want false")
	}

	// Test GetUUIDByIP
	uuid, ok := cm.GetUUIDByIP("10.9.1.2")
	if !ok || uuid != "device-001" {
		t.Errorf("GetUUIDByIP(10.9.1.2) = %q, ok=%v, want device-001, true", uuid, ok)
	}
	_, ok = cm.GetUUIDByIP("10.9.1.3")
	if ok {
		t.Errorf("GetUUIDByIP(10.9.1.3) = _, ok=true, want false (device-002 disabled)")
	}

	// Test GetNetwork
	network := cm.GetNetwork()
	if network != "10.9.1.0/24" {
		t.Errorf("GetNetwork() = %q, want 10.9.1.0/24", network)
	}
}

func TestClientManagerReload(t *testing.T) {
	content1 := `{
		"clients": [
			{"uuid": "device-001", "ip": "10.9.1.2", "name": "iPhone", "enabled": true}
		],
		"network": "10.9.1.0/24",
		"next_dynamic_ip": 50
	}`
	content2 := `{
		"clients": [
			{"uuid": "device-001", "ip": "10.9.1.2", "name": "iPhone", "enabled": true},
			{"uuid": "device-002", "ip": "10.9.1.3", "name": "NewDevice", "enabled": true}
		],
		"network": "10.9.1.0/24",
		"next_dynamic_ip": 50
	}`

	tmpFile1, err := os.CreateTemp("", "clients1-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile1.Name())

	tmpFile2, err := os.CreateTemp("", "clients2-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile2.Name())

	if _, err := tmpFile1.WriteString(content1); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile1.Close()

	if _, err := tmpFile2.WriteString(content2); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile2.Close()

	cm := NewClientManager()
	if err := cm.LoadClients(tmpFile1.Name()); err != nil {
		t.Fatalf("LoadClients() error = %v", err)
	}

	if cm.GetClientCount() != 1 {
		t.Errorf("Initial GetClientCount() = %d, want 1", cm.GetClientCount())
	}

	// Reload with new file
	if err := cm.Reload(tmpFile2.Name()); err != nil {
		t.Errorf("Reload() error = %v", err)
	}

	if cm.GetClientCount() != 2 {
		t.Errorf("After reload GetClientCount() = %d, want 2", cm.GetClientCount())
	}

	ip, ok := cm.GetIPByUUID("device-002")
	if !ok || ip != "10.9.1.3" {
		t.Errorf("After reload GetIPByUUID(device-002) = %q, ok=%v, want 10.9.1.3, true", ip, ok)
	}
}

func TestClientManagerNotFound(t *testing.T) {
	cm := NewClientManager()
	if err := cm.LoadClients("/nonexistent/path/clients.json"); err == nil {
		t.Errorf("LoadClients() with nonexistent path should error")
	}
}
