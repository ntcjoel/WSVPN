package obfuscation

import (
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand"
	"sync"
	"time"
)

// Padding header: 4 bytes for original packet length
const paddingHeaderLen = 4

// Obfuscation version constants
const (
	ObfuscationVersion1 = 1 // [4-byte big-endian length][payload][padding]
	ObfuscationVersion2 = 2 // [2-byte big-endian length][2-byte random][payload][padding]
)

const paddingHeaderLenV2 = 4 // Also 4 bytes: 2 length + 2 random

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

// TLS fingerprint identifiers for browser fingerprint camouflage
const (
	TLSFingerprintChrome  = "chrome"
	TLSFingerprintFirefox = "firefox"
	TLSFingerprintIOS     = "ios"
	TLSFingerprintEdge    = "edge"
	TLSFingerprintRandom  = "random"
)

// ValidTLSFingerprints returns the list of valid TLS fingerprint names
func ValidTLSFingerprints() []string {
	return []string{
		TLSFingerprintChrome,
		TLSFingerprintFirefox,
		TLSFingerprintIOS,
		TLSFingerprintEdge,
		TLSFingerprintRandom,
	}
}

// TrafficShape modes
const (
	TrafficShapeOff      = "off"
	TrafficShapeJitter   = "jitter"
	TrafficShapeBrowse   = "browse"
	TrafficShapeAdaptive = "adaptive"
)

// ValidTrafficShapes returns valid traffic shape mode names
func ValidTrafficShapes() []string {
	return []string{TrafficShapeOff, TrafficShapeJitter, TrafficShapeBrowse, TrafficShapeAdaptive}
}

// ShaperState manages traffic shaping with burst/pause cycles to mimic real browsing
type ShaperState struct {
	mu         sync.Mutex
	mode       string
	state      string        // "burst" or "pause"
	batchMax   int           // packets in current burst batch
	batchCount int           // packets sent so far in this batch
	pauseUntil time.Time     // when the current pause ends
	pauseMin   time.Duration
	pauseMax   time.Duration
	jitterMin  time.Duration
	jitterMax  time.Duration
	rng        *mrand.Rand // local RNG for thread safety
}

// NewShaperState creates a new traffic shaper
func NewShaperState(mode string) *ShaperState {
	s := &ShaperState{
		mode:      mode,
		state:     "burst",
		pauseMin:  2 * time.Second,
		pauseMax:  8 * time.Second,
		jitterMin: 0,
		jitterMax: 50 * time.Millisecond,
		rng:       mrand.New(mrand.NewSource(time.Now().UnixNano())),
	}
	s.newBatch()
	return s
}

func (s *ShaperState) newBatch() {
	s.batchMax = 10 + s.rng.Intn(41) // 10-50 packets
	s.batchCount = 0
}

// NextDelay returns the delay to apply before sending the next packet,
// and whether the packet should be buffered (held during a pause).
func (s *ShaperState) NextDelay() (delay time.Duration, shouldBuffer bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.mode {
	case TrafficShapeOff:
		return 0, false
	case TrafficShapeJitter:
		return GetJitterDelay(), false
	case TrafficShapeBrowse, TrafficShapeAdaptive:
		now := time.Now()
		if s.state == "pause" {
			if now.Before(s.pauseUntil) {
				return time.Until(s.pauseUntil), true
			}
			// Pause ended — transition back to burst
			s.state = "burst"
			s.newBatch()
			return 0, false
		}

		// In burst state
		s.batchCount++

		if s.batchCount >= s.batchMax {
			// 30% chance to pause after a batch
			if s.rng.Float64() < 0.30 {
				s.state = "pause"
				pauseDuration := s.pauseMin + time.Duration(s.rng.Int63n(int64(s.pauseMax-s.pauseMin)))
				s.pauseUntil = now.Add(pauseDuration)
				return pauseDuration, true
			}
			s.newBatch()
		}

		jitter := time.Duration(s.rng.Int63n(int64(s.jitterMax - s.jitterMin + 1)))
		return jitter, false
	default:
		return 0, false
	}
}

// SetMode changes the shaping mode at runtime
func (s *ShaperState) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	s.state = "burst"
	s.newBatch()
}

// InitObfuscation initializes random seed with crypto-random value
func InitObfuscation() {
	// Use crypto/rand to seed math/rand securely
	b := make([]byte, 8)
	rand.Read(b)
	seed := int64(binary.BigEndian.Uint64(b))
	mrand.Seed(seed ^ time.Now().UnixNano())
}

// AddPaddingV2 adds padding with non-fixed header format.
// Format: [uint16_be(original_len)][uint16_be(random)][original_packet][padding to target]
// The 2 random bytes ensure every header looks different to DPI.
func AddPaddingV2(packet []byte) []byte {
	paddingLen := 50 + mrand.Intn(451)

	result := make([]byte, paddingHeaderLenV2+len(packet)+paddingLen)

	// Write original packet length as uint16 (max 65535, VPN MTU is 1500)
	binary.BigEndian.PutUint16(result, uint16(len(packet)))

	// Write 2 random bytes — changes every header, defeating DPI pattern detection
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	copy(result[2:4], randBytes)

	// Copy original packet
	copy(result[paddingHeaderLenV2:], packet)

	// Fill padding with random bytes
	rand.Read(result[paddingHeaderLenV2+len(packet):])

	return result
}

// RemovePaddingV2 extracts original packet from v2 padded data
func RemovePaddingV2(data []byte) ([]byte, error) {
	if len(data) < paddingHeaderLenV2 {
		return nil, ErrInvalidPadding
	}

	// Read original packet length
	packetLen := int(binary.BigEndian.Uint16(data[:2]))

	// Validate length
	if packetLen == 0 || packetLen > 1500 {
		return nil, ErrInvalidPacketLength
	}

	if len(data) < paddingHeaderLenV2+packetLen {
		return nil, ErrInvalidPadding
	}

	// Skip 2 random bytes (data[2:4]) and extract original packet
	return data[paddingHeaderLenV2 : paddingHeaderLenV2+packetLen], nil
}

// AddPaddingVersion selects the appropriate padding function based on version
func AddPaddingVersion(packet []byte, version int) []byte {
	switch version {
	case ObfuscationVersion2:
		return AddPaddingV2(packet)
	case ObfuscationVersion1:
		fallthrough
	default:
		return AddPadding(packet)
	}
}

// RemovePaddingVersion selects the appropriate removal function based on version
func RemovePaddingVersion(data []byte, version int) ([]byte, error) {
	switch version {
	case ObfuscationVersion2:
		return RemovePaddingV2(data)
	case ObfuscationVersion1:
		fallthrough
	default:
		return RemovePadding(data)
	}
}

// SimulateHTTPSPatternVersion pads packet to HTTPS size distribution with the given version
func SimulateHTTPSPatternVersion(packet []byte, version int) []byte {
	sizes := []int{64, 256, 1024, 1480}
	weights := []float64{0.20, 0.30, 0.25, 0.25}

	targetSize := weightedRandomSize(sizes, weights)

	if len(packet) >= targetSize {
		return AddPaddingVersion(packet, version)
	}

	// Pad to target size
	headerSize := paddingHeaderLen
	if version == ObfuscationVersion2 {
		headerSize = paddingHeaderLenV2
	}

	result := make([]byte, headerSize+targetSize)

	if version == ObfuscationVersion2 {
		binary.BigEndian.PutUint16(result, uint16(len(packet)))
		randBytes := make([]byte, 2)
		rand.Read(randBytes)
		copy(result[2:4], randBytes)
		copy(result[4:], packet)
	} else {
		binary.BigEndian.PutUint32(result, uint32(len(packet)))
		copy(result[headerSize:], packet)
	}

	rand.Read(result[headerSize+len(packet):])

	return result
}

// RemovePaddingAuto detects the obfuscation version from the data and removes padding.
// Tries v1 first (backward compatible), then falls back to v2.
func RemovePaddingAuto(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, ErrInvalidPadding
	}

	// Try V1 first (backward compatible — most common case)
	v1Len := int(binary.BigEndian.Uint32(data[:4]))
	if v1Len > 0 && v1Len <= 1500 && len(data) >= 4+v1Len {
		return data[4 : 4+v1Len], nil
	}

	// Fall back to V2
	v2Len := int(binary.BigEndian.Uint16(data[:2]))
	if v2Len > 0 && v2Len <= 1500 && len(data) >= 4+v2Len {
		return data[4 : 4+v2Len], nil
	}

	return nil, ErrInvalidPadding
}

// Errors
type paddingError string

func (e paddingError) Error() string { return string(e) }

const (
	ErrInvalidPadding      = paddingError("invalid padding data")
	ErrInvalidPacketLength = paddingError("invalid packet length")
)
