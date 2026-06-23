package spartn

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/scantest"
)

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// spartnFrames holds real, unencrypted SPARTN frames captured from a u-blox
// PointPerfect IP service: OCB (type 0), HPAC (type 1), and GAD (type 2)
// messages across the GPS/GLONASS/Galileo/BeiDou subtypes.
var spartnFrames = []struct {
	name   string
	hexstr string
	msg    string // expected MsgID ("type-subtype")
}{
	{"ocb-gps", "73000f21036ce013ce8021a0811017f0b1a2fdf3245fbf7a8bf7f2117f0f822fddb345fc22e88506dd", "0-0"},
	{"ocb-glonass", "73000d2014bdd013cec0030a0617f05ea25e8e745fb6808bf623117f3d222fdcbe403e2af6", "0-1"},
	{"ocb-galileo", "73000fa8236ce013ce81001c4020a0be7f7117d274a2f9f0645fbf628bf8c9917ef71a2f9e9d40c5db1f", "0-2"},
	{"ocb-beidou", "73001027336c7013ce8020810006018815b0baa2fe08545fc0de8bf7a9d17efe5a2fdf8145fc13289dd5f3", "0-3"},
	{"hpac-gps", "73020da208f7e7f98013ce8013a0141804e021a0811009f7054f82a7c15650a67091802a95578310", "1-0"},
	{"hpac-glonass", "73020cab18f7e94a7013ce8013a0141804e0030a0633511b4a832e35978eda089913e0ecdcb1", "1-1"},
	{"hpac-galileo", "73020e2328f7e7f98013ce8013a0141804e00038804140a5f58ff627311485b2a40d958894301ef12f", "1-2"},
	{"hpac-beidou", "73020eaa38f7e7f91013ce8013a0141804e08204001806202b3b54a1aaab4529e2ac24575a29348a7093", "1-3"},
	{"gad", "730404a108f7e7f98013ce8013b651bc9b43405f9140", "2-0"},
}

func TestGoodSPARTN(t *testing.T) {
	for _, tt := range spartnFrames {
		t.Run(tt.name, func(t *testing.T) {
			buf := mustHexDecode(tt.hexstr)
			startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
			if startPos != 0 || endPos != len(buf) || !ok {
				t.Fatalf("failed to scan valid SPARTN packet: start=%d end=%d ok=%v", startPos, endPos, ok)
			}
			if id := PacketFormat.MsgID(buf); id != tt.msg {
				t.Fatalf("MsgID = %q, want %q", id, tt.msg)
			}
			if !bytes.Equal(PacketFormat.ComputeChecksum(buf), PacketFormat.ExtractChecksum(buf)) {
				t.Fatalf("SPARTN checksum not recognized as correct")
			}
		})
	}
}

// TestHeaderCRCGate verifies the TF006 frame CRC-4 gates recognition: a frame
// with a corrupted transport header (which a stray preamble byte amounts to) is
// not recognized as valid, even though its framing arithmetic is unaffected.
func TestHeaderCRCGate(t *testing.T) {
	buf := mustHexDecode(spartnFrames[0].hexstr)
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
	buf := mustHexDecode(spartnFrames[0].hexstr)
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
