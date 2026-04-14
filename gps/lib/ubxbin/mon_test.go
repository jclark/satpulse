package ubxbin

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/latin1z"
	"golang.org/x/exp/slices"
)

func TestMonVer(t *testing.T) {
	m := MonVer{
		MonVerFixed{latin1z.StringZ30{1, 2, 3}, latin1z.StringZ10{4, 5}},
		[]latin1z.StringZ30{{7, 8}, {9}},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonVer(&m, p2.(*MonVer)) {
		t.Fatalf("msg mon-ver not roundtripped %v => %v", &m, p2)
	}
}

func EqualMonVer(p1, p2 *MonVer) bool {
	return p1.MonVerFixed == p2.MonVerFixed && slices.Equal(p1.Extension, p2.Extension)
}

func TestMonComms(t *testing.T) {
	tests := []struct {
		name       string
		hex        string
		want       MonComms
		wantPort   PortID
		wantPortOK bool
	}{
		{
			// Captured from u-blox NEO-F10N (1 port)
			name: "f10n",
			hex:  "b5620a363000000100000001ffff000100006edd8840060f00008d010000000000000b00000000000000000000000000000014000000469b",
			want: MonComms{
				MonCommsFixed: MonCommsFixed{
					NPorts:  1,
					ProtIds: [4]byte{0, 1, 0xff, 0xff},
				},
				Ports: []MonCommsPort{
					{
						PortID:      MonCommsPortIDUART1,
						TxBytes:     1082711406,
						TxUsage:     6,
						TxPeakUsage: 15,
						RxBytes:     397,
						Msgs:        [4]uint16{11, 0, 0, 0},
						Skipped:     20,
					},
				},
			},
		},
		{
			// Captured from u-blox ZED-X20P (4 ports)
			name:       "x20p",
			wantPort:   PortUART2,
			wantPortOK: true,
			hex:        "b5620a36a80000040c00000105060000000000000000000a00000000000000000000000000000000000000000000000000000000000000010a03a1829f1e02020000590400000001000048000000000000000000000000000000000000000002ca1c77f8fc93161d0000a50c000000000000e4000000000000000000000000000000000000000003000000000000000a000000000000000000000000000000000000000000000000000000000000612f",
			want: MonComms{
				MonCommsFixed: MonCommsFixed{
					NPorts:   4,
					TxErrors: MonCommsTxErrors(PortUART2+1) << 2,
					ProtIds:  [4]byte{0, 1, 5, 6},
				},
				Ports: []MonCommsPort{
					{
						TxPeakUsage: 10,
					},
					{
						PortID:      MonCommsPortIDUART1,
						TxPending:   778,
						TxBytes:     513770145,
						TxUsage:     2,
						TxPeakUsage: 2,
						RxBytes:     1113,
						RxPeakUsage: 1,
						Msgs:        [4]uint16{72, 0, 0, 0},
					},
					{
						PortID:      MonCommsPortIDUART2,
						TxPending:   7370,
						TxBytes:     2482829431,
						TxUsage:     22,
						TxPeakUsage: 29,
						RxBytes:     3237,
						Msgs:        [4]uint16{228, 0, 0, 0},
					},
					{
						PortID:      MonCommsPortIDUSB,
						TxPeakUsage: 10,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, _ := hex.DecodeString(tt.hex)
			msg, err := ParseMsg(string(packet))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			m, ok := msg.(*MonComms)
			if !ok {
				t.Fatalf("expected *MonComms, got %T", msg)
			}
			if !reflect.DeepEqual(*m, tt.want) {
				t.Fatalf("got %+v, want %+v", *m, tt.want)
			}
			port, ok := m.TxErrors.OutputPort()
			if ok != tt.wantPortOK || port != tt.wantPort {
				t.Fatalf("OutputPort() = (%v, %v), want (%v, %v)", port, ok, tt.wantPort, tt.wantPortOK)
			}
			b, err := Serialize(m)
			if err != nil {
				t.Fatalf("serialize error: %v", err)
			}
			if string(b) != string(packet) {
				t.Fatalf("round-trip mismatch")
			}
		})
	}
}

func TestMonCommsPortID(t *testing.T) {
	tests := []struct {
		pid    MonCommsPortID
		want   PortID
		wantOK bool
	}{
		{MonCommsPortIDI2C, PortI2C, true},
		{MonCommsPortIDUART1, PortUART1, true},
		{MonCommsPortIDUART2, PortUART2, true},
		{MonCommsPortIDUSB, PortUSB, true},
		{MonCommsPortIDSPI, PortSPI, true},
		{0x0500, 0, false},  // out of range
		{0x0001, 0, false},  // low byte set
		{0x0101, 0, false},  // valid high byte but low byte set
		{0xFFFF, 0, false},  // all bits set
	}
	for _, tt := range tests {
		got, ok := tt.pid.PortID()
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("MonCommsPortID(%#x).PortID() = (%v, %v), want (%v, %v)",
				uint16(tt.pid), got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestMonVerJSON(t *testing.T) {
	m := MonVer{
		MonVerFixed: MonVerFixed{
			SwVersion: latin1z.StringZ30{'E', 'X', 'T', ' ', 'C', 'O', 'R', 'E', ' ', '1', '.', '0', '0'},
			HwVersion: latin1z.StringZ10{'0', '0', '1', '9', '0', '0', '0', '0'},
		},
		Extension: []latin1z.StringZ30{
			func() latin1z.StringZ30 {
				var b latin1z.StringZ30
				copy(b[:], "FWVER=SPG 1.00")
				return b
			}(),
		},
	}
	b, err := json.Marshal(&m)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"swVersion":"EXT CORE 1.00"`,
		`"hwVersion":"00190000"`,
		`"FWVER=SPG 1.00"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON %s\nmissing %s", s, want)
		}
	}
}

func TestMonGnssV0(t *testing.T) {
	testMsgType(t, MonGnss{
		Version:      0,
		Supported:    MonGnssGPS | MonGnssGalileo,
		DefaultGnss:  MonGnssGPS,
		Enabled:      MonGnssGPS | MonGnssBeidou,
		Simultaneous: 4,
	})
}

func TestMonGnssUnknownVersion(t *testing.T) {
	// Test that unknown version gets parsed as UnknownMsg
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x0A, 0x28, // class=MON(0x0A), id=GNSS(0x28)
		0x08, 0x00, // length = 8 bytes
		// Payload starts here:
		0x02,                   // version = 2 (unknown)
		0x0F, 0x0E, 0x0D, 0x0C, // Some data
		0x04, 0x00, 0x00, // More data
	}
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse MON-GNSS v2: %v", err)
	}

	// Should be parsed as UnknownMsg
	unknownMsg, ok := msg.(*UnknownMsg)
	if !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}

	if unknownMsg.MsgID != MonGnssID {
		t.Errorf("expected MsgID=%v, got %v", MonGnssID, unknownMsg.MsgID)
	}

	expectedPayload := string([]byte{0x02, 0x0F, 0x0E, 0x0D, 0x0C, 0x04, 0x00, 0x00})
	if unknownMsg.Payload != expectedPayload {
		t.Errorf("expected payload=%x, got %x", expectedPayload, unknownMsg.Payload)
	}
}

func TestMonGnss1(t *testing.T) {
	m := MonGnss1{
		MonGnss1Fixed: MonGnss1Fixed{
			Version:        1,
			NumPlans:       2,
			ActivePlanInfo: 0x0102,
		},
		Plans: []MonGnssPlan{
			{
				ID:       1,
				Name:     latin1z.StringZ5{'S', 'P', '1', 0, 0},
				GpsSup:   0x000D,
				GalSup:   0x000B,
				BdsSup:   0x001B,
				SbasSup:  0x0001,
				QzssSup:  0x0039,
				NavicSup: 0x0001,
			},
			{
				ID:       2,
				Name:     latin1z.StringZ5{'S', 'P', '2', 0, 0},
				GpsSup:   0x000D,
				GalSup:   0x000B,
				BdsSup:   0x001A,
				SbasSup:  0x0001,
				QzssSup:  0x0039,
				NavicSup: 0x0001,
			},
		},
	}
	p2 := testMsgType1[MonGnss1](t, m)
	m2 := p2.(*MonGnss1)
	if m2.Version != 1 || m2.NumPlans != 2 || m2.ActivePlanInfo != m.ActivePlanInfo {
		t.Fatalf("MonGnss1 header not roundtripped: %+v", m2.MonGnss1Fixed)
	}
	if len(m2.Plans) != len(m.Plans) {
		t.Fatalf("MonGnss1 plan count: got %d, want %d", len(m2.Plans), len(m.Plans))
	}
	for i := range m.Plans {
		if m2.Plans[i] != m.Plans[i] {
			t.Fatalf("MonGnss1 plan %d not roundtripped", i)
		}
	}
}
