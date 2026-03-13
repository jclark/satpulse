package rtcm

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/scantest"
)

// Example from RTCM 10403.2, section 4.2
// Note that \x escape in a Go string literal represents a byte not a rune.
const rtcmEx = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestGoodRTCM(t *testing.T) {
	rtcmOK(t, 1005, string(rtcmEx))
}

func rtcmOK(t *testing.T, msgNum MsgType, data string) {
	buf := ([]byte)(data)
	startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
	if startPos != 0 || endPos != len(buf) || !ok {
		t.Fatalf("failed to scan valid RTCM packet")
	}
	m := ParseMessage(data)
	if m.MsgType != msgNum {
		t.Fatalf(`wrong message number for RTCM %d`, msgNum)
	}
	if !bytes.Equal(PacketFormat.ComputeChecksum(buf), PacketFormat.ExtractChecksum(buf)) {
		t.Fatalf(`checksum of RTCM %d not recognized as correct`, msgNum)
	}
}

func TestIsCommonMsgType(t *testing.T) {
	isCommon := map[MsgType]bool{
		1230: true,
		1074: true,
		1077: true,
		0000: false,
		9999: false,
		1000: false,
	}
	for mt, b := range isCommon {
		if isCommonMsgType(mt) != b {
			t.Fatalf("isCommonMsgType(%d) = %v, want %v", mt, !b, b)
		}
	}
}

func TestMSMMsgType(t *testing.T) {
	tests := []struct {
		name string
		gnss gpsprot.GNSS
		msm  int
		want MsgType
	}{
		// GPS MSM tests
		{"GPS MSM1", gpsprot.GPS, 1, 1071},
		{"GPS MSM4", gpsprot.GPS, 4, 1074},
		{"GPS MSM5", gpsprot.GPS, 5, 1075},
		{"GPS MSM7", gpsprot.GPS, 7, 1077},
		
		// GLONASS MSM tests
		{"GLONASS MSM1", gpsprot.GLO, 1, 1081},
		{"GLONASS MSM4", gpsprot.GLO, 4, 1084},
		{"GLONASS MSM5", gpsprot.GLO, 5, 1085},
		{"GLONASS MSM7", gpsprot.GLO, 7, 1087},
		
		// Galileo MSM tests
		{"Galileo MSM1", gpsprot.GAL, 1, 1091},
		{"Galileo MSM4", gpsprot.GAL, 4, 1094},
		{"Galileo MSM5", gpsprot.GAL, 5, 1095},
		{"Galileo MSM7", gpsprot.GAL, 7, 1097},
		
		// BeiDou MSM tests
		{"BeiDou MSM1", gpsprot.BDS, 1, 1121},
		{"BeiDou MSM4", gpsprot.BDS, 4, 1124},
		{"BeiDou MSM5", gpsprot.BDS, 5, 1125},
		{"BeiDou MSM7", gpsprot.BDS, 7, 1127},
		
		// NavIC MSM tests
		{"NavIC MSM1", gpsprot.NAVIC, 1, 1131},
		{"NavIC MSM4", gpsprot.NAVIC, 4, 1134},
		{"NavIC MSM5", gpsprot.NAVIC, 5, 1135},
		{"NavIC MSM7", gpsprot.NAVIC, 7, 1137},
		
		// Invalid MSM numbers
		{"GPS MSM0", gpsprot.GPS, 0, 0},
		{"GPS MSM8", gpsprot.GPS, 8, 0},
		{"GLONASS MSM-1", gpsprot.GLO, -1, 0},
		
		// Unsupported GNSS
		{"QZSS MSM4", gpsprot.QZSS, 4, 0},
		{"SBAS MSM4", gpsprot.SBAS, 4, 0},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MSMMsgType(tt.gnss, tt.msm)
			if got != tt.want {
				t.Errorf("MSMMsgType(%v, %d) = %d, want %d", tt.gnss, tt.msm, got, tt.want)
			}
		})
	}
}

func TestIsMSM(t *testing.T) {
	tests := map[MsgType]bool{
		1071: true,  // GPS MSM1
		1077: true,  // GPS MSM7
		1078: false, // not MSM
		1079: false, // not MSM
		1080: false, // not MSM (base, not x1-x7)
		1084: true,  // GLONASS MSM4
		1094: true,  // Galileo MSM4
		1124: true,  // BeiDou MSM4
		1137: true,  // NavIC MSM7
		1138: false, // out of range
		1005: false, // station ARP
		1230: false, // GLONASS bias
	}
	for mt, want := range tests {
		if mt.isMSM() != want {
			t.Errorf("MsgType(%d).isMSM() = %v, want %v", mt, !want, want)
		}
	}
}

func TestHasMore(t *testing.T) {
	// BeiDou MSM4 with multiple message bit set
	pkt1, _ := hex.DecodeString("d300e24640006b77b802002037e48000000000002082014171c71c31c71c71c3c3bbf3c3dc1bb3e41bcc816eeeaef290e61db81235f8eba4d75fa8f73c2677ccc783c6c7c98e5208dc0fe81926a27d238248048c08ca2aec5518a620e521c12363cbd7195f322593472a364a7f5dbc3d76aef5e207cf3ebf3cecbcf4f8079ff81e858079d1408cec02302808c86f4ff57d3fcd044c1414f0305432c2b8710ae4642b99806fdd41be0706f3d0313eb0c660831bf3f263bbc98a1f25f8fffffffffffffffffffffffffffffc00000017d9a5d86555755d6dcfbb0473597e30493cd55375f700fbba02")
	// BeiDou MSM4 with multiple message bit clear (last in group)
	pkt2, _ := hex.DecodeString("d301304640006b77b8000020000001580700001c208201416fbefbefbef96412425a925293fbdbebcbc5a4ca3d53b79797b096fefd37ffb4fd2dfcf3f0cfed5d7abb0cb6246b61d5d876c4f931f913cbc73cb5d76c64d94baef756f82d00b9e1a0829c021c1044231048b07e80ee3ed33d99fb84f4f1eac71dee1fcc2e62e344828886268c00b75803bf6010280040e600b89fae28feb7517ada5beb6ef7ae0fa1e6b707a7201eafd07a9a21e6d97ae8a3eba6c7ae8fdebaa4fae99404af28117080449481118002af90225d608c3682342208c370223c9fafd97ebbe1faf25febd29fb02179020be402ef8fc620ba6a02ead00bb0f84da5bffffffffffffffffffffffdd9ffffffffffff9fffe00000000005f8e395f7e35e16d96d35d18616e16c71c12453dd4d736385b6e304915fae3980599f30")
	if !hasMore(pkt1) {
		t.Error("expected HasMore true for packet 1")
	}
	if hasMore(pkt2) {
		t.Error("expected HasMore false for packet 2")
	}
	// Non-MSM message should return false
	if hasMore([]byte(rtcmEx)) {
		t.Error("expected HasMore false for non-MSM packet")
	}
	// Verify MsgID is unchanged (no suffix)
	if got := PacketFormat.MsgID(pkt1); got != "1124" {
		t.Errorf("MsgID(pkt1) = %q, want %q", got, "1124")
	}
}
