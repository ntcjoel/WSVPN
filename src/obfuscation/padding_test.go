package obfuscation

import (
	"testing"
)

// InitObfuscation is exported — tests in same package can access it directly

func TestAddPadding(t *testing.T) {
	InitObfuscation()

	tests := []struct {
		name      string
		packet    []byte
		wantLen   int // minimum expected length: header + original + 50 padding
		wantError bool
	}{
		{
			name:      "empty packet",
			packet:    []byte{},
			wantLen:   paddingHeaderLen + 0 + 50, // header + 0 + min padding
			wantError: true, // packetLen==0 is rejected by RemovePadding
		},
		{
			name:      "small packet",
			packet:    []byte("hello"),
			wantLen:   paddingHeaderLen + 5 + 50,
			wantError: false,
		},
		{
			name:      "typical IP packet (512 bytes)",
			packet:    make([]byte, 512),
			wantLen:   paddingHeaderLen + 512 + 50,
			wantError: false,
		},
		{
			name:      "maximum typical packet (1500 bytes)",
			packet:    make([]byte, 1500),
			wantLen:   paddingHeaderLen + 1500 + 50,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddPadding(tt.packet)

			// Must be at least header + original + min padding
			if len(result) < tt.wantLen {
				t.Errorf("AddPadding() len = %d, want >= %d", len(result), tt.wantLen)
			}

			// Result must start with correct length header
			if len(result) >= paddingHeaderLen {
				// RemovePadding should recover original
				recovered, err := RemovePadding(result)
				if tt.wantError {
					// Expected to fail — skip recovery check
					return
				}
				if (err != nil) != tt.wantError {
					t.Errorf("RemovePadding() error = %v, wantError %v", err, tt.wantError)
					return
				}
				if string(recovered) != string(tt.packet) {
					t.Errorf("RemovePadding() recovered packet mismatch")
				}
			}
		})
	}
}

func TestRemovePadding(t *testing.T) {
	InitObfuscation()

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "too short for header",
			data:    []byte{0x00},
			wantErr: true,
		},
		{
			name:    "valid padded data",
			data:    AddPadding([]byte("test data")),
			wantErr: false,
		},
		{
			name:    "zero length packet (invalid - rejected by server)",
			data:    AddPadding([]byte{}),
			wantErr: true, // packetLen==0 is rejected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RemovePadding(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemovePadding() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSimulateHTTPSPattern(t *testing.T) {
	InitObfuscation()

	packet := []byte("test packet content")
	result := SimulateHTTPSPattern(packet)

	// Remove padding and verify original recovered
	recovered, err := RemovePadding(result)
	if err != nil {
		t.Errorf("SimulateHTTPSPattern() produced invalid padding: %v", err)
	}
	if string(recovered) != string(packet) {
		t.Errorf("SimulateHTTPSPattern() recovered != original")
	}

	// Result should be larger than input (padding added)
	if len(result) <= len(packet)+paddingHeaderLen {
		t.Errorf("SimulateHTTPSPattern() did not add padding: len=%d, input=%d", len(result), len(packet))
	}
}

func TestWeightedRandomSize(t *testing.T) {
	InitObfuscation()

	sizes := []int{64, 256, 1024, 1480}
	weights := []float64{0.20, 0.30, 0.25, 0.25}

	// Run many times to verify distribution roughly matches weights
	counts := make(map[int]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		size := weightedRandomSize(sizes, weights)
		counts[size]++
	}

	tolerance := 0.1 // 10% tolerance

	for i, size := range sizes {
		expectedRatio := weights[i]
		actualRatio := float64(counts[size]) / float64(iterations)
		diff := actualRatio - expectedRatio
		if diff < -tolerance || diff > tolerance {
			t.Errorf("weightedRandomSize() distribution for size %d: got %.2f%%, want ~%.0f%% (+-%.0f%%)",
				size, actualRatio*100, expectedRatio*100, tolerance*100)
		}
	}
}

func TestGetJitterDelay(t *testing.T) {
	InitObfuscation()

	for i := 0; i < 100; i++ {
		delay := GetJitterDelay()
		ms := delay.Milliseconds()
		if ms < 100 || ms > 2000 {
			t.Errorf("GetJitterDelay() = %dms, want between 100ms and 2000ms", ms)
		}
	}
}

func TestGetHeartbeatInterval(t *testing.T) {
	InitObfuscation()

	for i := 0; i < 100; i++ {
		interval := GetHeartbeatInterval()
		seconds := interval.Seconds()
		if seconds < 30 || seconds > 90 {
			t.Errorf("GetHeartbeatInterval() = %.0fs, want between 30s and 90s", seconds)
		}
	}
}

func TestPaddingRoundTrip(t *testing.T) {
	InitObfuscation()

	packets := [][]byte{
		[]byte("a"),
		[]byte("hello world"),
		make([]byte, 64),
		make([]byte, 256),
		make([]byte, 1024),
		make([]byte, 1500),
	}

	for _, pkt := range packets {
		padded := AddPadding(pkt)
		recovered, err := RemovePadding(padded)
		if err != nil {
			t.Errorf("Round trip failed for len(%d): %v", len(pkt), err)
			continue
		}
		if string(recovered) != string(pkt) {
			t.Errorf("Round trip mismatch for len(%d): got %d bytes", len(pkt), len(recovered))
		}
	}
}
