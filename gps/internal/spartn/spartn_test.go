package spartn

import (
	"bytes"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/scantest"
)

// Real encrypted SPARTN frame (message 0-1) from a u-blox D9S receiver.
const spartnEx = "\x73\x00\x1f\xed\x13\x2a\x38\x5c\x2b\xc8\x87\x01\xa8\xdd\x92\x95\x78\x80\x66\x33\x97\xa9\x3f\x46\xdc\x6b\x3a\x9a\x64\x19\x89\x58\xfd\x5e\xc6\x66\x2e\xcd\x50\xa5\x4e\xe3\x83\xff\xc9\xcd\xcb\x9d\x3f\x13\xac\x19\xf5\x09\x80\x66\xf4\x63\x71\x31\xcf\x98\xd8\xde\xd1\xb7\xa8\x17\xc2\x2e\xba\x31\x3a\xea\xf6\x44"

func TestGoodSPARTN(t *testing.T) {
	buf := []byte(spartnEx)
	startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
	if startPos != 0 || endPos != len(buf) || !ok {
		t.Fatalf("failed to scan valid SPARTN packet: start=%d end=%d ok=%v", startPos, endPos, ok)
	}
	if id := PacketFormat.MsgID(buf); id != "0-1" {
		t.Fatalf("MsgID = %q, want %q", id, "0-1")
	}
	if !bytes.Equal(PacketFormat.ComputeChecksum(buf), PacketFormat.ExtractChecksum(buf)) {
		t.Fatalf("SPARTN checksum not recognized as correct")
	}
}

// TestHeaderCRCGate verifies the TF006 frame CRC-4 gates recognition: a frame
// with a corrupted transport header (which a stray preamble byte amounts to) is
// not recognized as valid, even though its framing arithmetic is unaffected.
func TestHeaderCRCGate(t *testing.T) {
	buf := []byte(spartnEx)
	if !gpsprot.IsValidPacket(PacketFormat, buf) {
		t.Fatalf("valid SPARTN frame not recognized")
	}
	bad := bytes.Clone(buf)
	bad[1] ^= 0x80 // corrupt TF002, invalidating the TF006 header CRC
	if gpsprot.IsValidPacket(PacketFormat, bad) {
		t.Fatalf("frame with bad header CRC should not be recognized")
	}
}

// TestBadChecksumNotFinal confirms a corrupted frame is still framed (Next is
// structural) but fails the checksum comparison.
func TestBadChecksumNotFinal(t *testing.T) {
	buf := []byte(spartnEx)
	bad := bytes.Clone(buf)
	bad[20] ^= 0xFF // corrupt a payload byte
	if _, _, ok := scantest.FindPacket(PacketFormat, bad); !ok {
		t.Fatalf("corrupted frame should still be structurally framed")
	}
	if bytes.Equal(PacketFormat.ComputeChecksum(bad), PacketFormat.ExtractChecksum(bad)) {
		t.Fatalf("corrupted frame unexpectedly passed checksum")
	}
	// A bad checksum must always trigger a rescan, so a bogus framed length
	// cannot skip a real frame that starts inside it.
	if !PacketFormat.RescanOnBadChecksum(true, bad) {
		t.Fatalf("RescanOnBadChecksum should always be true")
	}
}
