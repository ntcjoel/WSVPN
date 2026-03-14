package obfuscation

import (
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand"
	"time"
)

// Padding header: 4 bytes for original packet length
const paddingHeaderLen = 4

// AddPadding adds random padding to obfuscate packet size
// Format: [4 bytes original_len][original_packet][random_padding]
func AddPadding(packet []byte) []byte {
	// Random padding between 50-500 bytes
	paddingLen := 50 + mrand.Intn(451)
	
	// Create result buffer: header + packet + padding
	result := make([]byte, paddingHeaderLen+len(packet)+paddingLen)
	
	// Write original packet length
	binary.BigEndian.PutUint32(result, uint32(len(packet)))
	
	// Copy original packet
	copy(result[paddingHeaderLen:], packet)
	
	// Fill padding with random bytes
	rand.Read(result[paddingHeaderLen+len(packet):])
	
	return result
}

// RemovePadding extracts original packet from padded data
func RemovePadding(data []byte) ([]byte, error) {
	if len(data) < paddingHeaderLen {
		return nil, ErrInvalidPadding
	}
	
	// Read original packet length
	packetLen := binary.BigEndian.Uint32(data[:paddingHeaderLen])
	
	// Validate length
	if packetLen == 0 || packetLen > 1500 {
		return nil, ErrInvalidPacketLength
	}
	
	if uint32(len(data)) < paddingHeaderLen+packetLen {
		return nil, ErrInvalidPadding
	}
	
	// Extract original packet
	return data[paddingHeaderLen : paddingHeaderLen+int(packetLen)], nil
}

// GetJitterDelay returns a random delay for timing obfuscation
// Simulates irregular human browsing patterns (100ms - 2s)
func GetJitterDelay() time.Duration {
	ms := 100 + mrand.Intn(1900)
	return time.Duration(ms) * time.Millisecond
}

// GetHeartbeatInterval returns irregular heartbeat interval (30-90s)
func GetHeartbeatInterval() time.Duration {
	seconds := 30 + mrand.Intn(61)
	return time.Duration(seconds) * time.Second
}

// SimulateHTTPSPattern pads packet to match typical HTTPS traffic patterns
// Web traffic has varied packet sizes, not fixed VPN patterns
func SimulateHTTPSPattern(packet []byte) []byte {
	// HTTPS typical packet size distribution
	// 64: ACK/keepalive (20%)
	// 256: Small requests (30%)
	// 1024: Medium responses (25%)
	// 1480: Large packets (25%)
	
	sizes := []int{64, 256, 1024, 1480}
	weights := []float64{0.20, 0.30, 0.25, 0.25}
	
	targetSize := weightedRandomSize(sizes, weights)
	
	// Don't shrink packets, only pad if smaller
	if len(packet) >= targetSize {
		return AddPadding(packet)
	}
	
	// Pad to target size
	result := make([]byte, paddingHeaderLen+targetSize)
	
	binary.BigEndian.PutUint32(result, uint32(len(packet)))
	copy(result[paddingHeaderLen:], packet)
	rand.Read(result[paddingHeaderLen+len(packet):])
	
	return result
}

// weightedRandomSize selects a size based on weights
func weightedRandomSize(sizes []int, weights []float64) int {
	r := mrand.Float64()
	cumulative := 0.0
	
	for i, weight := range weights {
		cumulative += weight
		if r < cumulative {
			return sizes[i]
		}
	}
	
	return sizes[len(sizes)-1]
}

// InitObfuscation initializes random seed with crypto-random value
func InitObfuscation() {
	// Use crypto/rand to seed math/rand securely
	b := make([]byte, 8)
	rand.Read(b)
	seed := int64(binary.BigEndian.Uint64(b))
	mrand.Seed(seed ^ time.Now().UnixNano())
}

// Errors
type paddingError string

func (e paddingError) Error() string { return string(e) }

const (
	ErrInvalidPadding      = paddingError("invalid padding data")
	ErrInvalidPacketLength = paddingError("invalid packet length")
)
