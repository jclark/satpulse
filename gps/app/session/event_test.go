package session

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestEventWireCompatibility(t *testing.T) {
	timeMsg := &gpsprot.TimeMsg{UTCOffset: 37}
	pvt := gpsprot.PVMsgBundle{}
	corrPacket := &gpsprot.CorReportMsg{MsgID: "1005"}
	packet := gpsio.PacketLogEntry{Ascii: "$GPGGA"}

	tests := []struct {
		name   EventName
		event  Event
		legacy any
	}{
		{EventState, StateConnected, StateConnected},
		{EventReceiver, ReceiverEvent{OK: true}, ReceiverEvent{OK: true}},
		{EventSpeed, SpeedEvent(9600), int(9600)},
		{EventLog, LogEvent{Message: "connected"}, LogEvent{Message: "connected"}},
		{EventMsg, MsgEvent{Kind: "time", Msg: map[string]any{"valid": true}}, MsgEvent{Kind: "time", Msg: map[string]any{"valid": true}}},
		{EventTime, TimeEvent{TimeMsg: timeMsg}, timeMsg},
		{EventEpochPVT, EpochPVTEvent{PVMsgBundle: pvt}, pvt},
		{EventInitialPos, InitialPosEvent{13.75, 100.5}, [2]float64{13.75, 100.5}},
		{EventNMEAPosition, NMEAPositionEvent{Valid: true, Lat: 13.75, Lon: 100.5}, NMEAPositionEvent{Valid: true, Lat: 13.75, Lon: 100.5}},
		{EventMsgSend, MsgSendEvent{Session: 1, Status: "sent", Current: 1, Total: 2}, MsgSendEvent{Session: 1, Status: "sent", Current: 1, Total: 2}},
		{EventResponse, ResponseEvent{Session: 1, Kind: "ack", ResponseTo: 0}, ResponseEvent{Session: 1, Kind: "ack", ResponseTo: 0}},
		{EventCorrections, CorrEvent{State: "connected", Host: "example.com"}, CorrEvent{State: "connected", Host: "example.com"}},
		{EventCorrPacket, CorrPacketEvent{CorReportMsg: corrPacket}, corrPacket},
		{EventBaseARP, BaseARPEvent{StationID: 1, ECEF: [3]float64{1, 2, 3}}, BaseARPEvent{StationID: 1, ECEF: [3]float64{1, 2, 3}}},
		{EventPacket, PacketEvent{PacketLogEntry: packet}, packet},
	}

	for _, test := range tests {
		t.Run(string(test.name), func(t *testing.T) {
			if got := test.event.EventName(); got != test.name {
				t.Fatalf("EventName() = %q, want %q", got, test.name)
			}
			got, err := json.Marshal(test.event)
			if err != nil {
				t.Fatalf("marshal typed event: %v", err)
			}
			want, err := json.Marshal(test.legacy)
			if err != nil {
				t.Fatalf("marshal legacy payload: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("typed event JSON = %s, legacy payload JSON = %s", got, want)
			}
		})
	}
}
