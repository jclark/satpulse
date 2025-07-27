package unicore

import (
	"reflect"
	"testing"
)

var ppsStatusTests = []struct {
	name        string
	binPacket   []byte
	asciiPacket string
	value       PPSStatus
}{
	{
		name:        "PPSSTATUS with standard timing data",
		binPacket:   mustHexDecode("aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" + "80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" + "2bbcd2100100000000acecb02c000000000000000091a800a5"),
		asciiPacket: "#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n",
		value: PPSStatus{
			Status:       3,
			Week:         2376,
			MsSecInWeek:  540336000,
			PPSPulseErr:  -4,
			OffsetTime:   -27676000,
			ConfigInfo:   0x03E80020,
			Register:     0x00000015,
			TimeEstErr:   0,
			InnerQuality: 0x00666669,
			PPSInnerSta:  0x2B000000,
			PPSCfgSta:    0x0110D2BC,
			PPSStage:     0x00000000,
			Reserved1:    0x2CB0ECAC,
		},
	},
}

func TestPPSStatusBin(t *testing.T) {
	for _, tt := range ppsStatusTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing binary packet
			header, msg, err := ParseBinMsg(tt.binPacket)
			if err != nil {
				t.Fatalf("ParseBinMsg() error = %v", err)
			}

			ppsStatus, ok := msg.(*PPSStatus)
			if !ok {
				t.Fatalf("ParseBinMsg() returned wrong type: got %T, want *PPSSTATUS", msg)
			}

			checkPPSStatus(t, ppsStatus, &tt.value)

			// Test round-trip: value -> serialize -> parse
			serialized, err := SerializeBinMsg(header, &tt.value)
			if err != nil {
				t.Fatalf("SerializeBinMsg() error = %v", err)
			}

			_, msg2, err := ParseBinMsg(serialized)
			if err != nil {
				t.Fatalf("ParseBinMsg() on serialized packet error = %v", err)
			}

			ppsStatus2, ok := msg2.(*PPSStatus)
			if !ok {
				t.Fatalf("ParseBinMsg() on serialized returned wrong type: got %T, want *PPSSTATUS", msg2)
			}

			checkPPSStatus(t, ppsStatus2, &tt.value)
		})
	}
}

func TestPPSStatusAscii(t *testing.T) {
	for _, tt := range ppsStatusTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing ASCII packet
			header, msg, err := ParseAsciiMessage([]byte(tt.asciiPacket))
			if err != nil {
				t.Fatalf("ParseAsciiMessage() error = %v", err)
			}

			ppsStatus, ok := msg.(*PPSStatus)
			if !ok {
				t.Fatalf("ParseAsciiMessage() returned wrong type: got %T, want *PPSSTATUS", msg)
			}

			checkPPSStatus(t, ppsStatus, &tt.value)

			// Test round-trip: value -> serialize -> parse
			serialized, err := SerializeAsciiMsg(header, &tt.value)
			if err != nil {
				t.Fatalf("SerializeAsciiMsg() error = %v", err)
			}

			_, msg2, err := ParseAsciiMessage(serialized)
			if err != nil {
				t.Fatalf("ParseAsciiMessage() on serialized packet error = %v", err)
			}

			ppsStatus2, ok := msg2.(*PPSStatus)
			if !ok {
				t.Fatalf("ParseAsciiMessage() on serialized returned wrong type: got %T, want *PPSSTATUS", msg2)
			}

			checkPPSStatus(t, ppsStatus2, &tt.value)
		})
	}
}

func checkPPSStatus(t *testing.T, actual *PPSStatus, expected *PPSStatus) {
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("PPSSTATUS mismatch:\nGot:  %+v\nWant: %+v", *actual, *expected)
	}
}
