package msgfile

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
)

// NovAtel ASCII recv helper.

type recvNOVAEvent struct{ content string }

// recvNOVA feeds a whole NovAtel ASCII log packet to the correlator.
func recvNOVA(content string) recvNOVAEvent {
	return recvNOVAEvent{content: content}
}

func (e recvNOVAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagNovAtelAscii, e.content)
}

// Packets and lines captured from a ByNav M10 (firmware V7.82_AB1AD3_T).
const (
	novOK       = "<OK\r\n"
	novErrNoLog = "<ERROR:Requested log does not exist\r\n"
	novPort     = "[COM1]\r\n"
	novComCfg   = "COM1 115200 N 8 1 IN:AUTO OUT:AUTO \r\n"
	novTimeA    = "#TIMEA,COM1,0,99.9,FINESTEERING,2437,419456.000,00000000,0000,782;VALID,-1.511092188e-04,0.000000000e+00,-18.00000000000,2026,9,24,20,30,38000,VALID*306576b7\r\n"
)

func TestCorrelatorNovAtel(t *testing.T) {
	runCorrelatorTests(t, "novatel-test.toml", []correlatorTest{
		{
			name: "command other than LOG is complete at OK",
			tags: []string{"pps"},
			events: []event{
				sendEvent{},
				recvNOVAA(novOK),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
				recvEmptyTag(novPort),
				expect{relevance: LevelNotResponse},
				recvNOVA(novTimeA),
				expect{relevance: LevelNotResponse},
				checkMissing{},
			},
		},
		{
			name: "ERROR carries error text",
			tags: []string{"unlog"},
			events: []event{
				sendEvent{},
				recvNOVAA(novErrNoLog),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				expectNakError{want: "Requested log does not exist"},
			},
		},
		{
			name: "no response reported missing",
			tags: []string{"pps"},
			events: []event{
				sendEvent{},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "one command at a time",
			tags: []string{"pps", "unlog"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvNOVAA(novOK),
				expect{ack: AckAck, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNOVAA(novErrNoLog),
				expect{ack: AckNak, msgIndex: intptr(1)},
			},
		},
		{
			name: "speed change needs no OK",
			tags: []string{"speed"},
			events: []event{
				sendEvent{},
				checkMissing{},
			},
		},
		{
			name: "plain text reply shown, port prompt not",
			tags: []string{"comconfig"},
			events: []event{
				sendEvent{},
				recvNOVAA(novOK),
				expect{ack: AckAck, msgIndex: intptr(0)},
				recvEmptyTag(novPort),
				expect{relevance: LevelNotResponse},
				recvEmptyTag(novComCfg),
				expect{relevance: LevelMaybeResponse},
				checkMissing{},
			},
		},
		{
			name: "LOG output shown as a possible reply",
			tags: []string{"timea"},
			events: []event{
				sendEvent{},
				recvNOVAA(novOK),
				checkDone{canAcceptMore: true},
				recvNOVA(novTimeA),
				expect{relevance: LevelMaybeResponse},
				checkMissing{},
			},
		},
		{
			name: "rejected LOG is complete and gets no data",
			tags: []string{"timea"},
			events: []event{
				sendEvent{},
				recvNOVAA(novErrNoLog),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
				recvNOVA(novTimeA),
				expect{relevance: LevelNotResponse},
				checkMissing{},
			},
		},
		{
			name: "OK is not an ack once requests are complete",
			tags: []string{"pps"},
			events: []event{
				sendEvent{},
				recvNOVAA(novOK),
				expect{ack: AckAck, msgIndex: intptr(0)},
				recvNOVAA(novOK),
				expect{relevance: LevelNotResponse},
			},
		},
	})
}
