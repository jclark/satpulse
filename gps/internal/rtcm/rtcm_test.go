package rtcm

import (
	"bytes"
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

func TestReferenceStationID(t *testing.T) {
	// rtcmEx is a 1005 message; pkt[4]=0xD7, pkt[5]=0xD3
	// station ID = (0x07 << 8) | 0xD3 = 2003
	id, ok := ReferenceStationID([]byte(rtcmEx))
	if !ok {
		t.Fatal("ReferenceStationID returned false for 1005 packet")
	}
	if id != 2003 {
		t.Errorf("ReferenceStationID = %d, want 2003", id)
	}
	// Too-short packet
	if _, ok := ReferenceStationID([]byte{0xD3, 0x00}); ok {
		t.Error("ReferenceStationID returned true for short packet")
	}
	// Unknown message type
	if _, ok := ReferenceStationID([]byte{0xD3, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}); ok {
		t.Error("ReferenceStationID returned true for unknown message type 0")
	}
}
