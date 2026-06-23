package spartnbin

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// realFrames holds real, unencrypted SPARTN frames captured from a u-blox
// PointPerfect IP service: CRC-24, with 16-bit time tags for the OCB (type 0)
// messages and 32-bit tags for the HPAC (type 1) and GAD (type 2) messages.
var realFrames = []struct {
	name    string
	hexstr  string
	msg     string // expected MsgID ("type-subtype")
	crcType uint8
	ttType  uint8
	payLen  int
}{
	{"0-0", "73000f21036ce013ce8021a0811017f0b1a2fdf3245fbf7a8bf7f2117f0f822fddb345fc22e88506dd", "0-0", 2, 0, 30},
	{"0-1", "73000d2014bdd013cec0030a0617f05ea25e8e745fb6808bf623117f3d222fdcbe403e2af6", "0-1", 2, 0, 26},
	{"0-2", "73000fa8236ce013ce81001c4020a0be7f7117d274a2f9f0645fbf628bf8c9917ef71a2f9e9d40c5db1f", "0-2", 2, 0, 31},
	{"0-3", "73001027336c7013ce8020810006018815b0baa2fe08545fc0de8bf7a9d17efe5a2fdf8145fc13289dd5f3", "0-3", 2, 0, 32},
	{"1-0", "73020da208f7e7f98013ce8013a0141804e021a0811009f7054f82a7c15650a67091802a95578310", "1-0", 2, 1, 27},
	{"1-1", "73020cab18f7e94a7013ce8013a0141804e0030a0633511b4a832e35978eda089913e0ecdcb1", "1-1", 2, 1, 25},
	{"1-2", "73020e2328f7e7f98013ce8013a0141804e00038804140a5f58ff627311485b2a40d958894301ef12f", "1-2", 2, 1, 28},
	{"1-3", "73020eaa38f7e7f91013ce8013a0141804e08204001806202b3b54a1aaab4529e2ac24575a29348a7093", "1-3", 2, 1, 29},
	{"2-0", "730404a108f7e7f98013ce8013b651bc9b43405f9140", "2-0", 2, 1, 9},
}

func TestParseRealFrames(t *testing.T) {
	for _, tt := range realFrames {
		t.Run(tt.name, func(t *testing.T) {
			pkt := mustHexDecode(tt.hexstr)
			f, err := Parse(pkt)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if f.MsgID() != tt.msg {
				t.Errorf("MsgID = %q, want %q", f.MsgID(), tt.msg)
			}
			if f.CRCType != tt.crcType {
				t.Errorf("CRCType = %d, want %d", f.CRCType, tt.crcType)
			}
			if f.TimeTagType != tt.ttType {
				t.Errorf("TimeTagType = %d, want %d", f.TimeTagType, tt.ttType)
			}
			if len(f.Payload) != tt.payLen {
				t.Errorf("payload len = %d, want %d", len(f.Payload), tt.payLen)
			}
			if total, ok := FrameLen(pkt); !ok || total != len(pkt) {
				t.Errorf("FrameLen = %d, %v, want %d, true", total, ok, len(pkt))
			}
			if !VerifyHeaderCRC(pkt) {
				t.Errorf("TF006 header CRC not verified")
			}
			if !bytes.Equal(ComputeChecksum(pkt), ExtractChecksum(pkt)) {
				t.Errorf("TF018 checksum mismatch: got %x want %x",
					ComputeChecksum(pkt), ExtractChecksum(pkt))
			}
		})
	}
}

// TestCRCVectors checks the MSB-first CRC variants against the standard "check"
// value (the CRC of the ASCII string "123456789") from the CRC catalogue, which
// pins down the polynomial and init/xor parameters for the cases the real D9S
// capture (all CRC-24) does not exercise.
func TestCRCVectors(t *testing.T) {
	data := []byte("123456789")
	tests := []struct {
		name  string
		width int
		poly  uint64
		init  uint64
		xor   uint64
		want  uint64
	}{
		{"CRC-8", 8, 0x07, 0, 0, 0xF4},                                 // CRC-8/SMBUS
		{"CRC-16", 16, 0x1021, 0, 0, 0x31C3},                           // CRC-16/XMODEM
		{"CRC-32", 32, 0x04C11DB7, 0xFFFFFFFF, 0xFFFFFFFF, 0xFC891918}, // CRC-32/BZIP2
	}
	for _, tt := range tests {
		if got := crcMSB(data, tt.width, tt.poly, tt.init, tt.xor); got != tt.want {
			t.Errorf("%s = %#x, want %#x", tt.name, got, tt.want)
		}
	}
}

func TestPackRoundTrip(t *testing.T) {
	tests := []*Frame{
		{Type: 1, Subtype: 0, CRCType: 0, TimeTag: 0x1234, Payload: []byte("hello")},                                        // CRC-8, 16-bit tag
		{Type: 2, Subtype: 0, CRCType: 1, TimeTagType: 1, TimeTag: 0xABCDEF, Payload: []byte{1, 2, 3}},                      // CRC-16, 32-bit tag
		{Type: 0, Subtype: 3, CRCType: 3, Payload: bytes.Repeat([]byte{0xA5}, 40)},                                          // CRC-32
		{Type: 4, Subtype: 0, EAF: true, CRCType: 2, EncryptionID: 5, EncryptionSeq: 9, AuthInd: 1, Payload: []byte("enc")}, // EAF, no embedded auth
	}
	for i, in := range tests {
		pkt, err := Pack(in)
		if err != nil {
			t.Fatalf("test %d: Pack: %v", i, err)
		}
		if !VerifyHeaderCRC(pkt) {
			t.Errorf("test %d: TF006 not valid", i)
		}
		if !bytes.Equal(ComputeChecksum(pkt), ExtractChecksum(pkt)) {
			t.Errorf("test %d: TF018 mismatch", i)
		}
		if total, ok := FrameLen(pkt); !ok || total != len(pkt) {
			t.Errorf("test %d: FrameLen = %d, %v, want %d", i, total, ok, len(pkt))
		}
		out, err := Parse(pkt)
		if err != nil {
			t.Fatalf("test %d: Parse: %v", i, err)
		}
		if out.Type != in.Type || out.Subtype != in.Subtype || out.EAF != in.EAF ||
			out.CRCType != in.CRCType || out.TimeTagType != in.TimeTagType ||
			out.TimeTag != in.TimeTag || !bytes.Equal(out.Payload, in.Payload) {
			t.Errorf("test %d: round trip mismatch:\n got %+v\nwant %+v", i, out, in)
		}
	}
}
