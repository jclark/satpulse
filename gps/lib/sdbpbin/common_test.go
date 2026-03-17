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
