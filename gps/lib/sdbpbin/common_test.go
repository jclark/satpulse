package sdbpbin

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func stripSpaces(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			result = append(result, s[i])
		}
	}
	return string(result)
}

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(stripSpaces(s))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// Packets from the SDBP protocol doc.
var docPackets = []struct {
	name  string
	hex   string
	msgID string
}{
	{"QUE-VER", "233e 0501 0000 0617", "QUE-VER"},
}

// Captured packets from Taidou T303-5D (2026-03-17).
var capturedPackets = []struct {
	name  string
	hex   string
	msgID string
}{
	{"DAT-UTCT2", "233e061f1f0048002d0d0b02ea0703110d173741a002000400000012ffffffff000000000028d8", "DAT-UTCT2"},
	{"DAT-GPSU", "233e062d230000000000000030be000000000000e0bc0000000000000000003006006a09128909071246da", "DAT-GPSU"},
	{"DAT-BDSU", "233e062c2300000000000000d8bd00000000000007bd0000000000000000b04e03001e04043d0006041cb5", "DAT-BDSU"},
}

func TestParseCapturedPackets(t *testing.T) {
	for _, tc := range capturedPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt := decodeHex(t, tc.hex)
			msg, err := ParseMsg(string(pkt))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			if got := msg.ID().String(); got != tc.msgID {
				t.Errorf("ID = %q, want %q", got, tc.msgID)
			}
			// Serialize round-trip
			pkt2, err := Serialize(msg)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if !bytes.Equal(pkt, pkt2) {
				t.Errorf("round-trip mismatch:\n  got  %x\n  want %x", pkt2, pkt)
			}
		})
	}
}

func TestCapturedUTCT2Fields(t *testing.T) {
	pkt := decodeHex(t, capturedPackets[0].hex)
	msg, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	m := msg.(*DatUTCT2)
	if m.Year != 2026 || m.Month != 3 || m.Day != 17 {
		t.Errorf("date = %d-%d-%d, want 2026-3-17", m.Year, m.Month, m.Day)
	}
	if m.Hour != 13 || m.Min != 23 || m.Sec != 55 {
		t.Errorf("time = %d:%d:%d, want 13:23:55", m.Hour, m.Min, m.Sec)
	}
	if m.LeapSec != 18 {
		t.Errorf("LeapSec = %d, want 18", m.LeapSec)
	}
	if m.RefGrid != DatUTCT2RefGPS {
		t.Errorf("RefGrid = %d, want GPS (%d)", m.RefGrid, DatUTCT2RefGPS)
	}
	if m.Accuracy != 4 {
		t.Errorf("Accuracy = %d, want 4", m.Accuracy)
	}
}

func TestCapturedGPSUFields(t *testing.T) {
	pkt := decodeHex(t, capturedPackets[1].hex)
	msg, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	m := msg.(*DatGPSU)
	if m.DeltaTLS != 18 {
		t.Errorf("DeltaTLS = %d, want 18", m.DeltaTLS)
	}
	if m.DeltaTLSF != 18 {
		t.Errorf("DeltaTLSF = %d, want 18", m.DeltaTLSF)
	}
	if m.WNOT != 2410 {
		t.Errorf("WNOT = %d, want 2410", m.WNOT)
	}
}

func TestParseDocPackets(t *testing.T) {
	for _, tc := range docPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt := decodeHex(t, tc.hex)
			msg, err := ParseMsg(string(pkt))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			if got := msg.ID().String(); got != tc.msgID {
				t.Errorf("ID = %q, want %q", got, tc.msgID)
			}
		})
	}
}

func TestPacketMsgId(t *testing.T) {
	for _, tc := range docPackets {
		t.Run(tc.name, func(t *testing.T) {
			pkt := decodeHex(t, tc.hex)
			got := PacketMsgId(pkt).String()
			if got != tc.msgID {
				t.Errorf("PacketMsgId = %q, want %q", got, tc.msgID)
			}
		})
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Msg
	}{
		{"CtlRestart-cold", &CtlRestart{StartupMode: StartupCold, ResetMode: ResetSoft}},
		{"CtlRestart-hot", &CtlRestart{StartupMode: StartupHot, ResetMode: ResetHard}},
		{"CtlConfig-save", &CtlConfig{Action: ConfigSaveBRAMFlash}},
		{"CtlStandby", &CtlStandby{Mode: 1}},
		{"CfgGNSS-all", &CfgGNSS{ConstellationMask: GNSSBitBDS | GNSSBitGPS | GNSSBitGLO | GNSSBitGAL}},
		{"CfgUART", &CfgUART{PortID: PortUART1, Baud: Baud115200, StopBits: 1}},
		{"CfgRate-1Hz", &CfgRate{MeasInterval: 1000, PosFrequency: 1}},
		{"CfgGNSS-bds-gps", &CfgGNSS{ConstellationMask: GNSSBitBDS | GNSSBitGPS}},
		{"CfgNMEA-response", &CfgNMEA{SentenceID: NMEASentGGA, UART1: 1}},
		{"CfgSDBP-response", &CfgSDBP{MsgClass: 0x06, MsgID: 0x17, UART1: 1}},
		{"PubAck", &PubAck{Class: clsCFG, MsgID: 0x52}},
		{"PubNak", &PubNak{Class: clsCTL, MsgID: 0x01}},
		{"DatGPST", &DatGPST{DatGNSST{DatNavHeader{1000}, DatTimeTowValid | DatTimeWeekValid, 2410, 173735.5, 3}}},
		{"DatUTCT2", &DatUTCT2{
			DatNavHeader: DatNavHeader{1000},
			Valid:        DatUTCT2HMS | DatUTCT2YMD,
			RefGrid:      DatUTCT2RefGPS,
			Year:         2026, Month: 3, Day: 17,
			Hour: 0, Min: 15, Sec: 18,
			SecFrac: 500, Accuracy: 50,
			LeapSec: 18, LeapCountdown: -1, LeapChange: 1,
			LeapYear: 2025, LeapMonth: 6, LeapDay: 30,
		}},
		{"DatGPSU", &DatGPSU{DatGNSSU{A0: 1.5e-9, A1: 2.3e-15, DeltaTLS: 18, DeltaTLSF: 18}}},
		{"DatGALU", &DatGALU{DatGNSSU{DeltaTLS: 18, WNLSF: 77, DN: 1, DeltaTLSF: 19}}},
		{"DatBDSU", &DatBDSU{DatGNSSU{DeltaTLS: 4, DeltaTLSF: 4}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt, err := Serialize(tc.msg)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			// Verify sync bytes
			if pkt[0] != Sync1 || pkt[1] != Sync2 {
				t.Errorf("sync = %02x%02x, want %02x%02x", pkt[0], pkt[1], Sync1, Sync2)
			}
			// Verify checksum
			ckA, ckB := Checksum(pkt[2 : len(pkt)-2])
			if ckA != pkt[len(pkt)-2] || ckB != pkt[len(pkt)-1] {
				t.Errorf("checksum mismatch")
			}
			// Parse back
			msg2, err := ParseMsg(string(pkt))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			// Re-serialize and compare
			pkt2, err := Serialize(msg2)
			if err != nil {
				t.Fatalf("Serialize round-trip: %v", err)
			}
			if !bytes.Equal(pkt, pkt2) {
				t.Errorf("round-trip mismatch:\n  got  %x\n  want %x", pkt2, pkt)
			}
		})
	}
}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		cls, id byte
		want    string
	}{
		{0x01, 0x01, "PUB-ACK"},
		{0x01, 0x02, "PUB-NAK"},
		{0x02, 0x01, "CTL-RESTART"},
		{0x03, 0x11, "CFG-GNSS"},
		{0x03, 0x52, "CFG-SDBP"},
		{0x05, 0x01, "QUE-VER"},
		{0x06, 0x17, "DAT-GPST"},
		{0x06, 0x1F, "DAT-UTCT2"},
		{0x06, 0x2C, "DAT-BDSU"},
		{0x06, 0x2D, "DAT-GPSU"},
		{0x06, 0x2E, "DAT-GALU"},
		{0x06, 0x41, "DAT-TPPS"},
		{0x07, 0xFF, "0x07-0xFF"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := MakeMsgID(tc.cls, tc.id).String()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
