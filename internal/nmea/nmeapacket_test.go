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


