package gpscmd

import (
	"testing"

	casbin "github.com/jclark/satpulse/internal/casbin"
	"github.com/jclark/satpulse/internal/casic"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubx"
	"github.com/jclark/satpulse/internal/ubxbin"
)

func TestFormatPacket(t *testing.T) {
	nmeaPkt := func(data string) scan.Packet {
		return scan.Packet{Data: data, Format: nmea.PacketFormat}
	}
	ubxPkt := func(msg ubxbin.Msg) scan.Packet {
		data, _ := ubxbin.Serialize(msg)
		return scan.Packet{Data: string(data), Format: ubx.PacketFormat}
	}
	casbinPkt := func(msg casbin.Msg) scan.Packet {
		data, _ := casbin.Serialize(msg)
		return scan.Packet{Data: string(data), Format: casic.PacketFormat}
	}
	rp := &responsePrinter{}
	tests := []struct {
		name string
		pkt  scan.Packet
		want string
	}{
		{"GPGGA skipped", nmeaPkt("$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,47.0,M,,*47\r\n"), ""},
		{"GPRMC skipped", nmeaPkt("$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A\r\n"), ""},
		{"GNGGA skipped", nmeaPkt("$GNGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,47.0,M,,*55\r\n"), ""},
		{"GPTXT printed", nmeaPkt("$GPTXT,01,01,02,ROM CORE 3.01 (107888)*2B\r\n"), "$GPTXT,01,01,02,ROM CORE 3.01 (107888)*2B\n"},
		{"GNTXT printed", nmeaPkt("$GNTXT,01,01,02,some message*00\r\n"), "$GNTXT,01,01,02,some message*00\n"},
		{"proprietary printed", nmeaPkt("$PTEST,hello*00\r\n"), "$PTEST,hello*00\n"},
		{"strips CRLF", nmeaPkt("$PTEST,data*00\r\n"), "$PTEST,data*00\n"},
		{"strips LF only", nmeaPkt("$PTEST,data*00\n"), "$PTEST,data*00\n"},
		{"UBX ACK-ACK", ubxPkt(&ubxbin.AckAck{MsgID: ubxbin.CfgTp5ID}), "UBX-ACK-ACK: CFG-TP5\n"},
		{"UBX ACK-NAK", ubxPkt(&ubxbin.AckNak{MsgID: ubxbin.CfgTp5ID}), "UBX-ACK-NAK: CFG-TP5\n"},
		{"UBX unknown skipped", ubxPkt(&ubxbin.UnknownMsg{MsgID: ubxbin.CfgTp5ID}), ""},
		{"CASBIN ACK-ACK", casbinPkt(&casbin.AckAck{AckPayload: casbin.AckPayload{ClsID: 0x06, MsgID: 0x00}}), "CASBIN-ACK-ACK: CFG-PRT\n"},
		{"CASBIN ACK-NAK", casbinPkt(&casbin.AckNak{AckPayload: casbin.AckPayload{ClsID: 0x06, MsgID: 0x01}}), "CASBIN-ACK-NAK: CFG-MSG\n"},
		{"CASBIN unknown skipped", casbinPkt(&casbin.UnknownMsg{MsgID: casbin.MakeMsgID(0x01, 0x02)}), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rp.formatPacket(tt.pkt)
			if got != tt.want {
				t.Errorf("formatPacket() = %q, want %q", got, tt.want)
			}
		})
	}
}
