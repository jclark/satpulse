package spartnbin

import (
	"bytes"
	"testing"
)

// Real SPARTN frames captured from a u-blox D9S L-band receiver. Both are
// encrypted (EAF=1), CRC-24, 16-bit time tag.
const (
	frame01 = "\x73\x00\x1f\xed\x13\x2a\x38\x5c\x2b\xc8\x87\x01\xa8\xdd\x92\x95\x78\x80\x66\x33\x97\xa9\x3f\x46\xdc\x6b\x3a\x9a\x64\x19\x89\x58\xfd\x5e\xc6\x66\x2e\xcd\x50\xa5\x4e\xe3\x83\xff\xc9\xcd\xcb\x9d\x3f\x13\xac\x19\xf5\x09\x80\x66\xf4\x63\x71\x31\xcf\x98\xd8\xde\xd1\xb7\xa8\x17\xc2\x2e\xba\x31\x3a\xea\xf6\x44"
	frame02 = "\x73\x00\x11\x63\x21\xd9\x48\x5c\x2c\x48\x2e\xb5\xa8\x27\xb0\x4c\xdb\xf7\xd8\x0f\xce\xe7\x5d\x93\x34\xb3\x8c\x25\x5b\x05\x29\x62\x7a\xa8\x74\x80\xec\x9d\xa8\x7e\x25\xfd\xe7\x47\x6f\xd7\x36"
)

func TestParseRealFrames(t *testing.T) {
	tests := []struct {
		data    string
		msgID   string
		crcType uint8
		payLen  int
	}{
		{frame01, "0-1", 2, 63},
		{frame02, "0-2", 2, 34},
	}
	for _, tt := range tests {
		pkt := []byte(tt.data)
		f, err := Parse(pkt)
		if err != nil {
			t.Fatalf("Parse(%s): %v", tt.msgID, err)
		}
		if f.MsgID() != tt.msgID {
			t.Errorf("MsgID = %q, want %q", f.MsgID(), tt.msgID)
		}
		if !f.EAF {
			t.Errorf("%s: EAF = false, want true", tt.msgID)
		}
		if f.CRCType != tt.crcType {
			t.Errorf("%s: CRCType = %d, want %d", tt.msgID, f.CRCType, tt.crcType)
		}
		if len(f.Payload) != tt.payLen {
			t.Errorf("%s: payload len = %d, want %d", tt.msgID, len(f.Payload), tt.payLen)
		}
		if total, ok := FrameLen(pkt); !ok || total != len(pkt) {
			t.Errorf("%s: FrameLen = %d, %v, want %d, true", tt.msgID, total, ok, len(pkt))
		}
		if !VerifyHeaderCRC(pkt) {
			t.Errorf("%s: TF006 header CRC not verified", tt.msgID)
		}
		if !bytes.Equal(ComputeChecksum(pkt), ExtractChecksum(pkt)) {
			t.Errorf("%s: TF018 checksum mismatch: got %x want %x",
				tt.msgID, ComputeChecksum(pkt), ExtractChecksum(pkt))
		}
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
