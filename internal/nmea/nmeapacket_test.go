package nmea

import (
	"slices"
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


type altChecksumUsage int

const (
	altNotAvailable altChecksumUsage = iota
	altAvailableNotUsed
	altUsed
)

func TestAltChecksum(t *testing.T) {
	testCases := []struct {
		packet   string
		expected altChecksumUsage
	}{
		// Standard GNSS packets should not have alt checksum available
		{"$GNRMC,071109.00,A,1343.91012,N,10038.68403,E,0.035,,100625,,,A,V*11\r\n", altNotAvailable},
		{"$GNVTG,,T,,M,0.035,N,0.065,K,A*38\r\n", altNotAvailable},
		{"$GNGLL,1343.91012,N,10038.68403,E,071109.00,A,A*74\r\n", altNotAvailable},
		{"$GPGSV,1,1,00,0*65\r\n", altNotAvailable},
		{"$GNGSA,M,1,,,,,,,,,,,,,99.99,99.99,99.99,1*3F\r\n", altNotAvailable},
		
		// Unicore packets use alt checksum
		{"$command,VERSIONB,response: OK*46\r\n", altUsed},
		{"$command,CONFIG,response: OK*54\r\n", altUsed},
		{"$command,MASK,response: OK*4A\r\n", altUsed},
		{"$command,MODE,response: OK*5D\r\n", altUsed},
		{"$command,CONFIG PPS ENABLE GPS POSITIVE 100000 1000 0 0,response: OK*49\r\n", altUsed},
		{"$command,MODE BASE TIME 2000,response: OK*7F\r\n", altUsed},
		
		// Unicore Firebird packet PDTINFO uses normal checksum even though is alt available
		{"$PDTINFO,UM621-02,G1B1L1E1,V1.2,R6.0.0.0Build2810,2310414000033,000101114303845*10\r\n", altAvailableNotUsed},
	}

	for _, tc := range testCases {
		pkt := []byte(tc.packet)
		extracted := PacketFormat.ExtractChecksum(pkt)
		normal := PacketFormat.ComputeChecksum(pkt)
		alt := PacketFormat.(gpsprot.AltChecksumPacketFormat).ComputeAltChecksum(pkt)
		
		normalOK := slices.Equal(extracted, normal)
		var altOK bool
		if alt != nil {
			altOK = slices.Equal(extracted, alt)
		}
		var actual altChecksumUsage

		if normalOK {
			if alt == nil {
				actual = altNotAvailable
			} else {
				actual = altAvailableNotUsed
			}
		} else if altOK {
			actual = altUsed
		} else {
			t.Errorf("Packet %s did not match normal nor alt checksum", tc.packet)
			continue
		}
		if actual != tc.expected {
			t.Errorf("Alt checksum usage mismatch for %s: got %v, expected %v", tc.packet, actual, tc.expected)
		}
	}
}