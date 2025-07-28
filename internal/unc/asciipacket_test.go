package unc

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestAsciiPacketValidation(t *testing.T) {
	tests := []struct {
		name   string
		packet string
		valid  bool
	}{
		// Valid packets
		{
			"ValidWithCRC32",
			"#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000*0bbaac1a\r\n",
			true,
		},
		{
			"ValidWithXORChecksum",
			"#MODE,81,GPS,FINE,2230,547967000,0,0,18,518;MODE ROVER SURVEY*1b\r\n",
			true,
		},
		{
			"ValidNoChecksum",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;\r\n",
			true,
		},
		{
			"ValidMinimal",
			"#A;\r\n",
			true,
		},
		{
			"ValidLongName",
			"#VERYLONGMESSAGENAME,data,here;more,data*12345678\r\n",
			true,
		},
		// All printable ASCII characters (0x20-0x7E)
		{
			"ValidAllPrintableChars",
			"#TEST,!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~;data*ab\r\n",
			true,
		},

		// Invalid packets - wrong start
		{
			"InvalidStart",
			"$GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M*75\r\n",
			false,
		},
		{
			"NoStart",
			"GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M*75\r\n",
			false,
		},

		// Invalid packets - no semicolon
		{
			"NoSemicolon",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M*75\r\n",
			false,
		},

		// Invalid packets - wrong endings
		{
			"NoCRLF",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;",
			false,
		},
		{
			"OnlyCR",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;\r",
			false,
		},
		{
			"OnlyLF",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;\n",
			false,
		},
		{
			"WrongTermination",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;\n\r",
			false,
		},

		// Invalid packets - wrong checksum format
		{
			"StarNoChecksum",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*\r\n",
			false,
		},
		{
			"Star1Digit",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*a\r\n",
			false,
		},
		{
			"Star3Digits", // Not 2 or 8
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*abc\r\n",
			false,
		},
		{
			"Star7Digits", // Not 8
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*1234567\r\n",
			false,
		},
		{
			"Star9Digits", // Not 8
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*123456789\r\n",
			false,
		},
		{
			"StarNonHex",
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*1g\r\n",
			false,
		},
		{
			"StarUpperCase", // Should be lowercase
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*1A\r\n",
			false,
		},
		{
			"CRC32UpperCase", // Should be lowercase
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M;*1234567A\r\n",
			false,
		},

		// Invalid packets - unprintable characters
		{
			"ControlChar", // 0x01
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M\x01;data*ab\r\n",
			false,
		},
		{
			"DELChar", // 0x7F
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M\x7F;data*ab\r\n",
			false,
		},
		{
			"HighBitSet", // 0xFF
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M\xFF;data*ab\r\n",
			false,
		},
		{
			"Below0x20", // Just below printable range
			"#GPGGA,123045,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M\x1F;data*ab\r\n",
			false,
		},

		// Edge cases
		{
			"EmptyMessage",
			"#;\r\n",
			true,
		},
		{
			"EmptyAfterSemicolon",
			"#MSG;\r\n",
			true,
		},
		{
			"MultipleSemicolons", // First one is separator
			"#MSG,data;more;data*12\r\n",
			true,
		},
		{
			"StarInData", // Star before separator is allowed
			"#MSG,data*stuff;more*ab\r\n",
			true,
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

func TestAsciiPacketChecksum(t *testing.T) {
	t.Run("PPSSTATUSA_CRC32", func(t *testing.T) {
		pkt := []byte("#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n")
		extracted := AsciiPacketFormat.ExtractChecksum(pkt)
		computed := AsciiPacketFormat.ComputeChecksum(pkt)
		
		if string(extracted) != string(computed) {
			t.Errorf("Checksum mismatch: extracted=%X, computed=%X", extracted, computed)
		}
	})
}