package nmea

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/scantest"
)


// TestPacketFormat tests PacketFormat.Next using gpsprot.IsValidPacket and scantest.IsValidPacketBetween.
// with all syntax test cases. Test cases with expected flags as 0 should be invalid packets,
// and others should be valid packets. This test will fail until we fix PacketFormat.Next for nmea-lax.
func TestPacketFormat(t *testing.T) {
	for _, tt := range syntaxTestCases {
		t.Run(tt.name, func(t *testing.T) {
			expectValidPacket := tt.expected != 0

			// Test gpsprot.IsValidPacket
			isValid := gpsprot.IsValidPacket(PacketFormat, []byte(tt.packet))
			if isValid != expectValidPacket {
				t.Errorf("gpsprot.IsValidPacket mismatch for %q: got %v, want %v", tt.packet, isValid, expectValidPacket)
			}

			// Test that IsValidPacket works even within a buffer
			for range 10 {
				buf, nRandom := scantest.InsertRandomPrefix(tt.packet, '$')
				isValid = scantest.IsValidPacketBetween(PacketFormat, buf, nRandom, nRandom+len(tt.packet))
				
				if isValid != expectValidPacket {
					t.Errorf("scantest.IsValidPacketBetween mismatch for %q: got %v, want %v", tt.packet, isValid, expectValidPacket)
				}
			}
		})
	}
}

func TestMsgID(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		want   string
	}{
		{"Standard NMEA with comma", "$GPRMC,123456.00,A,1234.5678,N*12\r\n", "GPRMC"},
		{"Standard NMEA with star only", "$GPGGA*12\r\n", "GPGGA"},
		{"Proprietary with P prefix", "$PMTK*12\r\n", "PMTK"},
		{"Longer message ID with comma", "$GPGSV,1,1,04*12\r\n", "GPGSV"},
		{"Extended message with comma", "$GNGLL,1234.5678,N*12\r\n", "GNGLL"},
		{"Extended message with P and comma", "$PUBX,40,GLL*12\r\n", "PUBX"},
		{"Long proprietary message", "$PQTMCFGSVIN,W,1,3600,1.2,-2519265.0514,4849534.9045,3277834.6432*01\r\n", "PQTMCFGSVIN"},
	}

	pf := PacketFormat
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pf.MsgID([]byte(tt.packet))
			if got != tt.want {
				t.Errorf("MsgID(%q) = %q, want %q", tt.packet, got, tt.want)
			}
		})
	}
}


