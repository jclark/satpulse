package msgfile

import (
	"testing"

	"github.com/jclark/satpulse/gps/internal/septentrio"
)

// Septentrio reply recv helper.

type recvSEPTREvent struct{ content string }

// recvSeptReply feeds a whole Septentrio $R reply packet to the correlator.
func recvSeptReply(content string) recvSEPTREvent {
	return recvSEPTREvent{content: content}
}

func (e recvSEPTREvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(septentrio.TagReply, e.content)
}

// expectNakError asserts the NakError of the most recent Correlation.
type expectNakError struct{ want string }

func (e expectNakError) run(t *testing.T, tc *testContext) {
	t.Helper()
	if tc.last.NakError != e.want {
		t.Errorf("NakError = %q, want %q", tc.last.NakError, e.want)
	}
}

func TestCorrelatorSeptentrio(t *testing.T) {
	runCorrelatorTests(t, "sept-test.toml", []correlatorTest{
		{
			name: "set command ACK with readback",
			tags: []string{"set-nmea"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R: setNMEAOutput, Stream1, USB1, GGA, sec1\r\nNMEAOutput, Stream1, USB1, GGA, sec1\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command NAK carries error text",
			tags: []string{"set-nmea"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R? SBFOutput: Not authorized!\r\nCOM1>"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				expectNakError{want: "SBFOutput: Not authorized!"},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "login $R! treated as ack",
			tags: []string{"login"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R! LogIn\r\nUserLevel, User\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "lst $R; first unit treated as ack",
			tags: []string{"lst"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R; lstAsciiDisplay\r\n---->"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "factoryReset plain ack with trailing message",
			tags: []string{"factory-reset"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R: factoryReset: Resetting receiver to factory defaults.\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "single-flight pacing blocks second until first completes",
			tags: []string{"set-nmea", "set-elev"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvSeptReply("$R: setNMEAOutput, Stream1, USB1, GGA, sec1\r\nNMEAOutput, Stream1, USB1, GGA, sec1\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvSeptReply("$R: setElevationMask, all, 5\r\nElevationMask, all, 5\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
	})
}
