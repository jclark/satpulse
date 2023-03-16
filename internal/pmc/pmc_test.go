package pmc

import (
	"testing"

	"golang.org/x/exp/slices"
)

var header string = "\x0d\x02\x00\x3e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x31\x4d\x00\x00\x04\x7f"
var body string = "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x01\x00"
var tlv string = "\x00\x01\x00\x0a\xc0\x01"
var grandmaster_settings_np string = "\x06\x23\xff\xff\x00\x25\x1c\xa0"

var msg = header + body + tlv + grandmaster_settings_np

var gsmData = GrandmasterSettings{
	ClockQuality: ClockQuality{
		ClockClass:              0x6,
		ClockAccuracy:           0x23,
		OffsetScaledLogVariance: 0xFFFF,
	},
	UTCOffset:  37,
	TimeFlags:  CurrentUTCOffsetValid | PTPTimescale | TimeTraceable,
	TimeSource: 0xA0, // what is this?
}

const testPID = 12621

func TestGSN(t *testing.T) {
	client := NewMgmtClient()
	client.portNumber = testPID
	gsm := NewGrandmasterSettingsMsg(client, gsmData)
	bytes, err := gsm.MarshalBinary()
	if err != nil {
		t.Fatalf("first MarshalBinaryFailed: %v", err)
	}
	if len(msg) != len(bytes) {
		t.Fatalf("wrong length: got %d, want %d", len(bytes), len(msg))
	}
	for i := 0; i < len(msg); i++ {
		if msg[i] != bytes[i] {
			t.Errorf("wrong byte at %d: got %02x, want %02x\n", i, bytes[i], msg[i])
		}
	}
	gsm.SetLength(uint16(len(bytes)))

	m, err := UnmarshalMgmtMsg(bytes)
	if err != nil {
		t.Fatalf("UnmarshalMgmtMsg failed: %v", err)
	}

	gsm2, ok := m.(*GrandmasterSettingsMsg)
	if !ok {
		t.Fatalf("wrong type: got %T, want *GrandmasterSettingsMsg", m)
	}
	if gsm.V.Data != gsm2.V.Data {
		t.Fatalf("data not round tripped: got %v, want %v", gsm2.V.Data, gsm.V.Data)
	}
	if gsm.MgmtMsgFixed != gsm2.MgmtMsgFixed {
		t.Fatalf("fixed part not round tripped: got %v, want %v", gsm2.MgmtMsgFixed, gsm.MgmtMsgFixed)
	}
	bytes2, err := gsm2.MarshalBinary()
	if err != nil {
		t.Fatalf("second MarshalBinary failed: %v", err)
	}
	if !slices.Equal(bytes, bytes2) {
		t.Fatalf("got different bytes: got %v, want %v", bytes2, bytes)
	}
}
