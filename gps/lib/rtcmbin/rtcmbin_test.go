package rtcmbin

import (
	"encoding/hex"
	"testing"
)

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
		{"MSM4 station 1234", []byte(decodePkt(t, msmPackets[0].hex)), 1234, true},
		{"too short", []byte{0xD3, 0x00, 0x01, 0x43}, 0, false},
		{"unknown type", []byte{0xD3, 0x00, 0x01, 0x00, 0x10, 0xFF, 0xFF, 0xFF, 0xFF}, 0, false},
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

func TestParseMSM4(t *testing.T) {
	for _, tt := range msmPackets {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			result, err := ParseMsg(pkt)
			if err != nil {
				t.Fatal(err)
			}
			msg, ok := result.(*MSM)
			if !ok {
				t.Fatalf("ParseMsg returned %T, want *MSM", result)
			}
			if msg.MsgNum != tt.mt {
				t.Errorf("MsgNum = %d, want %d", msg.MsgNum, tt.mt)
			}
			if msg.GNSS() != tt.gnss {
				t.Errorf("GNSS() = %q, want %q", msg.GNSS(), tt.gnss)
			}
			if msg.MSMLevel() != 4 {
				t.Errorf("MSMLevel() = %d, want 4", msg.MSMLevel())
			}
			nsat := msg.Nsat()
			nsig := msg.Nsig()
			if nsat == 0 {
				t.Error("Nsat = 0")
			}
			if nsig == 0 {
				t.Error("Nsig = 0")
			}
			t.Logf("%s: station=%d nsat=%d nsig=%d sats=%v sigs=%v",
				tt.gnss, msg.StationID, nsat, nsig, msg.Satellites(), msg.Signals())
			if len(msg.Sat.RangeInt) != nsat {
				t.Errorf("RangeInt len = %d, want %d", len(msg.Sat.RangeInt), nsat)
			}
			if len(msg.Sat.RangeMod) != nsat {
				t.Errorf("RangeMod len = %d, want %d", len(msg.Sat.RangeMod), nsat)
			}
			if len(msg.Sat.ExtInfo) != 0 {
				t.Errorf("ExtInfo len = %d, want 0", len(msg.Sat.ExtInfo))
			}
			if len(msg.Sat.PhaseRate) != 0 {
				t.Errorf("PhaseRate len = %d, want 0", len(msg.Sat.PhaseRate))
			}
			ncell := len(msg.Sig.Pseudorange)
			if ncell == 0 {
				t.Error("Pseudorange len = 0")
			}
			if len(msg.Sig.PhaseRange) != ncell {
				t.Errorf("PhaseRange len = %d, want %d", len(msg.Sig.PhaseRange), ncell)
			}
			if len(msg.Sig.CNR) != ncell {
				t.Errorf("CNR len = %d, want %d", len(msg.Sig.CNR), ncell)
			}
			if len(msg.Sig.PhaseRate) != 0 {
				t.Errorf("Sig.PhaseRate len = %d, want 0", len(msg.Sig.PhaseRate))
			}
			wantID, idOK := ReferenceStationID([]byte(pkt))
			if !idOK {
				t.Fatal("ReferenceStationID returned false")
			}
			if msg.StationID != wantID {
				t.Errorf("StationID = %d, want %d", msg.StationID, wantID)
			}
		})
	}
}

func TestSerializeMsgMT1005(t *testing.T) {
	pkts := []struct {
		name string
		hex  string
	}{
		{"station 2003", hex.EncodeToString([]byte(rtcm1005))},
		{"station 1234", "d300133ed4d203bd55b51d7f8e2e1f5ad403808e27daf56b52"},
	}
	for _, tt := range pkts {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatal(err)
			}
			got, err := SerializeMsg(msg)
			if err != nil {
				t.Fatal(err)
			}
			if got != pkt {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, pkt)
			}
		})
	}
}

func TestSerializeMsgMT1230(t *testing.T) {
	pkt := decodePkt(t, "d3000c4ce4d28f000000000000000073929d")
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := SerializeMsg(msg)
	if err != nil {
		t.Fatal(err)
	}
	if got != pkt {
		t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, pkt)
	}
}

func TestSerializeMsgMSM4(t *testing.T) {
	for _, tt := range msmPackets {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatal(err)
			}
			got, err := SerializeMsg(msg)
			if err != nil {
				t.Fatal(err)
			}
			if got != pkt {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, pkt)
			}
		})
	}
}

func TestPackMsgTooLong(t *testing.T) {
	_, err := PackMsg(make([]byte, 1024))
	if err == nil {
		t.Error("PackMsg(1024 bytes) should fail")
	}
}

func TestParseMSM4StationID(t *testing.T) {
	pkt0 := decodePkt(t, msmPackets[0].hex)
	msg0, err := ParseMsg(pkt0)
	if err != nil {
		t.Fatal(err)
	}
	wantID := msg0.(*MSM).StationID
	for _, tt := range msmPackets[1:] {
		pkt := decodePkt(t, tt.hex)
		result, err := ParseMsg(pkt)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		msg := result.(*MSM)
		if msg.StationID != wantID {
			t.Errorf("%s: StationID = %d, want %d", tt.name, msg.StationID, wantID)
		}
	}
}
