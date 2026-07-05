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
			name: "lst reply: opener acks, real prompt completes",
			tags: []string{"lst"},
			events: []event{
				sendEvent{},
				// The "$R;" opener is the ack: OK is reported now, but the
				// command stays open for the "$--BLOCK" output still to come.
				recvSeptReply("$R; lstAsciiDisplay\r\n---->"),
				expect{ack: AckAck, relevance: LevelMaybeResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: true},
				// The final "$--BLOCK" section ends at the real prompt: it
				// completes the command and is shown without a second ack line.
				recvSeptReply("$-- BLOCK 1 / 1\r\nAsciiDisplay contents\r\nCOM1>"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "lst reply: multiple blocks",
			tags: []string{"lst"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R; lstAsciiDisplay\r\n---->"),
				expect{ack: AckAck, relevance: LevelMaybeResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: true},
				recvSeptReply("$-- BLOCK 1 / 2\r\npart one\r\n---->"),
				expect{relevance: LevelMaybeResponse},
				checkDone{canAcceptMore: true},
				recvSeptReply("$-- BLOCK 2 / 2\r\npart two\r\nCOM1>"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "lst reply holds single-flight until the prompt",
			tags: []string{"lst", "set-elev"},
			events: []event{
				sendEvent{},
				recvSeptReply("$R; lstAsciiDisplay\r\n---->"),
				expect{ack: AckAck, relevance: LevelMaybeResponse, msgIndex: intptr(0)},
				// Blocks are still streaming: the next command must wait.
				readyToSend{want: false},
				recvSeptReply("$-- BLOCK 1 / 1\r\ncontents\r\nCOM1>"),
				// The real prompt completed the lst: the next command may send.
				readyToSend{want: true},
				sendEvent{},
				recvSeptReply("$R: setElevationMask, all, 5\r\nElevationMask, all, 5\r\nCOM1>"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(1)},
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
