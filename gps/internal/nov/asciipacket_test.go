package nov

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestAsciiPacketValidation(t *testing.T) {
	// Most of the tests are in unc/asciipacket_test.go
	tests := []struct {
		name   string
		packet string
		valid  bool
	}{
		{
			"valid NOVA packet with 8-digit CRC32",
			"#TIMEA,COM1,13504,97.0,FINE,2380,442654.000,562843644,14,18;VALID,-6.244420740e-05,9.075413271e-09,-18.00000000000,2025,8,22,2,57,16000,VALID*70144497\r\n",
			true,
		},
		{
			"UNCA packet should not be recognized as NOVA",
			"#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000*0bbaac1a\r\n",
			false, // First field after comma is '93' which is numeric, not alphabetic
		},
		{
			"packet with 2-digit XOR checksum should be rejected",
			"#BESTPOSA,COM1,0,0.0,FINE,1975,393343.000,00000000,0000,113;data*1B\r\n",
			false, // 2-digit checksums not allowed for NOVA
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gpsprot.IsValidPacket(AsciiPacketFormat, []byte(tt.packet))
			if result != tt.valid {
				t.Errorf("IsValidPacket() = %v, want %v for packet: %q",
					result, tt.valid, tt.packet)
			}
		})
	}
}