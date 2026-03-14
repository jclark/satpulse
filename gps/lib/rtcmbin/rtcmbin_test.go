package rtcmbin

import "testing"

// rtcm1005 is a real RTCM 1005 (Station ARP) packet.
// Message type 1005 = 0x3ED => byte 3 = 0x3E, byte 4 high nibble = 0xD.
// Station ID in byte 4 low nibble + byte 5 = 0x7D3 = 2003.
const rtcm1005 = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

// makeMSM builds a minimal 10-byte+ buffer encoding the given message
// type and Multiple Message Bit.  The packet is not valid RTCM (no
// real payload or checksum) but is long enough for field extraction.
func makeMSM(msgType uint16, mmb bool) []byte {
	// bytes 0-2: preamble + length (don't matter for extraction)
	pkt := make([]byte, 12)
	pkt[0] = 0xD3
	// bytes 3-4: message type in upper 12 bits
	pkt[3] = byte(msgType >> 4)
	pkt[4] = byte(msgType<<4) & 0xF0
	// bytes 4-5: station ID in lower 12 bits (set to 42)
	pkt[4] |= 0x00 // upper 4 bits of station ID = 0
	pkt[5] = 42     // lower 8 bits of station ID = 42
	// byte 9: MMB is bit 1 (0x02)
	if mmb {
		pkt[9] = 0x02
	}
	return pkt
}

func TestMultipleMessageBit(t *testing.T) {
	tests := []struct {
		name    string
		pkt     []byte
		wantMMB bool
		wantOK  bool
	}{
		{"non-MSM 1005", []byte(rtcm1005), false, false},
		{"MSM7 GPS mmb set", makeMSM(1077, true), true, true},
		{"MSM7 GPS mmb clear", makeMSM(1077, false), false, true},
		{"MSM4 Galileo mmb set", makeMSM(1094, true), true, true},
		{"MSM4 BeiDou mmb clear", makeMSM(1124, false), false, true},
		{"too short", []byte{0xD3, 0x00, 0x01, 0x43, 0x50}, false, false},
		{"empty", []byte{}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mmb, ok := MultipleMessageBit(tt.pkt)
			if ok != tt.wantOK || mmb != tt.wantMMB {
				t.Errorf("MultipleMessageBit() = (%v, %v), want (%v, %v)",
					mmb, ok, tt.wantMMB, tt.wantOK)
			}
		})
	}
}

func TestMultipleMessageBitString(t *testing.T) {
	// Verify the string variant works identically.
	pkt := makeMSM(1077, true)
	mmb, ok := MultipleMessageBit(string(pkt))
	if !ok || !mmb {
		t.Errorf("MultipleMessageBit(string) = (%v, %v), want (true, true)", mmb, ok)
	}
}

func TestReferenceStationID(t *testing.T) {
	tests := []struct {
		name   string
		pkt    []byte
		wantID uint16
		wantOK bool
	}{
		{"1005 station 2003", []byte(rtcm1005), 2003, true},
		{"MSM7 station 42", makeMSM(1077, false), 42, true},
		{"too short", []byte{0xD3, 0x00, 0x01, 0x43}, 0, false},
		{"unknown type", []byte{0xD3, 0x00, 0x01, 0x00, 0x10, 0xFF}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := ReferenceStationID(tt.pkt)
			if ok != tt.wantOK || id != tt.wantID {
				t.Errorf("ReferenceStationID() = (%v, %v), want (%v, %v)",
					id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}
