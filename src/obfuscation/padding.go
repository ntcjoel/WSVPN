package obfuscation

import (
	"crypto/rand"
	"encoding/binary"
	mrand "math/rand"
	"sync"
	"time"
)

// Padding header: 4 bytes — [uint16_be(original_len)][uint16_be(random)]
// The 2 random bytes ensure every packet header looks different to DPI.
const paddingHeaderLen = 4

// AddPadding wraps packet with randomized header + random padding.
// Format: [uint16_be(len)][2 random bytes][original_packet][random_padding]
func AddPadding(packet []byte) []byte {
	paddingLen := 20 + mrand.Intn(181) // 20-200 bytes

	result := make([]byte, paddingHeaderLen+len(packet)+paddingLen)

	// Write original packet length as uint16 (max 65535, VPN MTU is 1500)
	binary.BigEndian.PutUint16(result, uint16(len(packet)))

	// Write 2 random bytes — changes every header
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	copy(result[2:4], randBytes)

	// Copy original packet
	copy(result[paddingHeaderLen:], packet)

	// Fill padding with random bytes
	rand.Read(result[paddingHeaderLen+len(packet):])

	return result
}

// RemovePadding extracts original packet from padded data.
func RemovePadding(data []byte) ([]byte, error) {
	if len(data) < paddingHeaderLen {
		return nil, ErrInvalidPadding
	}

	packetLen := int(binary.BigEndian.Uint16(data[:2]))

	if packetLen == 0 || packetLen > 1500 {
		return nil, ErrInvalidPacketLength
	}

	if len(data) < paddingHeaderLen+packetLen {
		return nil, ErrInvalidPadding
	}

	// Skip 2 random bytes (data[2:4]) and extract original packet
	return data[paddingHeaderLen : paddingHeaderLen+packetLen], nil
}

// GetJitterDelay returns a random delay for timing obfuscation (100ms - 2s).
func GetJitterDelay() time.Duration {
	ms := 100 + mrand.Intn(1900)
	return time.Duration(ms) * time.Millisecond
}

// GetHeartbeatInterval returns irregular heartbeat interval (30-90s).
func GetHeartbeatInterval() time.Duration {
	seconds := 30 + mrand.Intn(61)
	return time.Duration(seconds) * time.Second
}

// isPureACK checks if an IPv4 packet is a pure TCP ACK (no data, just header).
// Pure ACKs are 40 bytes: 20 IP + 20 TCP. Padding them is wasteful — real HTTPS
// connections carry similarly small TLS ACKs, so leaving them small is realistic.
func isPureACK(packet []byte) bool {
	if len(packet) < 40 || (packet[0]>>4) != 4 {
		return false
	}
	ipLen := int(packet[2])<<8 | int(packet[3])
	if ipLen != len(packet) {
		return false
	}
	if packet[9] != 6 { // TCP protocol
		return false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 {
		return false
	}
	tcpOffset := int((packet[ihl+12] >> 4) * 4)
	return ipLen == ihl+tcpOffset
}

// SimulateHTTPSPattern pads packet to match typical HTTPS traffic size distribution.
// Pure TCP ACKs are passed through with minimal padding — real TLS ACKs are small.
// Uses randomized header format: [uint16_be(len)][2 random bytes][payload][padding].
func SimulateHTTPSPattern(packet []byte) []byte {
	if isPureACK(packet) {
		// Minimal padding: just the 4-byte randomized header, no extra padding.
		// Small frames are normal in HTTPS (TLS ACKs, keepalive).
		result := make([]byte, paddingHeaderLen+len(packet))
		binary.BigEndian.PutUint16(result, uint16(len(packet)))
		randBytes := make([]byte, 2)
		rand.Read(randBytes)
		copy(result[2:4], randBytes)
		copy(result[paddingHeaderLen:], packet)
		return result
	}
	// Bias target sizes toward the packet's natural size to minimize padding waste.
	// Large data packets (>800B) should target 1480, not 64.
	sizes := []int{64, 256, 1024, 1480}
	var weights []float64
	switch {
	case len(packet) > 800:
		weights = []float64{0.05, 0.10, 0.25, 0.60}
	case len(packet) > 300:
		weights = []float64{0.10, 0.15, 0.50, 0.25}
	default:
		weights = []float64{0.20, 0.30, 0.25, 0.25}
	}

	targetSize := weightedRandomSize(sizes, weights)

	if len(packet) >= targetSize {
		return AddPadding(packet)
	}

	// Pad to target size with randomized header
	result := make([]byte, paddingHeaderLen+targetSize)

	binary.BigEndian.PutUint16(result, uint16(len(packet)))
	randBytes := make([]byte, 2)
	rand.Read(randBytes)
	copy(result[2:4], randBytes)
	copy(result[paddingHeaderLen:], packet)
	rand.Read(result[paddingHeaderLen+len(packet):])

	return result
}

// weightedRandomSize selects a size based on cumulative weights.
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

// TLS fingerprint identifiers.
const (
	TLSFingerprintChrome  = "chrome"
	TLSFingerprintFirefox = "firefox"
	TLSFingerprintIOS     = "ios"
	TLSFingerprintEdge    = "edge"
	TLSFingerprintRandom  = "random"
)

// ValidTLSFingerprints returns the list of valid TLS fingerprint names.
func ValidTLSFingerprints() []string {
	return []string{
		TLSFingerprintChrome,
		TLSFingerprintFirefox,
		TLSFingerprintIOS,
		TLSFingerprintEdge,
		TLSFingerprintRandom,
	}
}

// TrafficShape modes.
const (
	TrafficShapeOff      = "off"
	TrafficShapeJitter   = "jitter"
	TrafficShapeBrowse   = "browse"
	TrafficShapeAdaptive = "adaptive"
)

// ValidTrafficShapes returns valid traffic shape mode names.
func ValidTrafficShapes() []string {
	return []string{TrafficShapeOff, TrafficShapeJitter, TrafficShapeBrowse, TrafficShapeAdaptive}
}

// ShaperState manages traffic shaping with burst/pause cycles.
type ShaperState struct {
	mu         sync.Mutex
	mode       string
	state      string
	batchMax   int
	batchCount int
	pauseUntil time.Time
	pauseMin   time.Duration
	pauseMax   time.Duration
	jitterMin  time.Duration
	jitterMax  time.Duration
	rng        *mrand.Rand
}

// NewShaperState creates a new traffic shaper.
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
	s.batchMax = 10 + s.rng.Intn(41)
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
			s.state = "burst"
			s.newBatch()
			return 0, false
		}

		s.batchCount++

		if s.batchCount >= s.batchMax {
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

// SetMode changes the shaping mode at runtime.
func (s *ShaperState) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	s.state = "burst"
	s.newBatch()
}

// InitObfuscation initializes random seed with crypto-random value.
func InitObfuscation() {
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
