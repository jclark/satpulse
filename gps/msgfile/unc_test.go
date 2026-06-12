package msgfile

import (
	"fmt"
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
)

// NovAtel abbreviated ASCII recv helper.

type recvNOVAAEvent struct{ content string }

func recvNOVAA(content string) recvNOVAAEvent {
	return recvNOVAAEvent{content: content}
}

func (e recvNOVAAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagNovAtelAbbrevAscii, e.content)
}

// Unicore ASCII recv helper.

type recvUNCAEvent struct{ content string }

// recvUnicoreAscii constructs a UNCA packet from the content (including #).
func recvUnicoreAscii(content string) recvUNCAEvent {
	return recvUNCAEvent{content: content}
}

func makeUNCA(content string) string {
	// content starts with '#', checksum covers everything after '#'
	body := content[1:]
	var ck byte
	for i := 0; i < len(body); i++ {
		ck ^= body[i]
	}
	return fmt.Sprintf("%s*%02x\r\n", content, ck)
}

func (e recvUNCAEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	tc.last = tc.cor.CorrelatePacket(gpsreg.TagUnicoreAscii, makeUNCA(e.content))
}

func TestCorrelatorUnicore(t *testing.T) {
	runCorrelatorTests(t, "unc-test.toml", []correlatorTest{
		{
			name: "set command ACK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command NAK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: not support"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command no response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "CONFIG query ACK then data",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0"),
				expect{relevance: LevelMultiResponse},
				recvNMEA("CONFIG,SIGNALGROUP,CONFIG SIGNALGROUP 2"),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true}, // multiple data, completion never known
				checkMissing{},                 // not missing: data was received
			},
		},
		{
			name: "CONFIG query NAK",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: unknown command"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "CONFIG query missing ACK and data",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "MASK query ACK then data",
			tags: []string{"get-mask"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MASK,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("CONFIG,MASK,MASK 5.000000"),
				expect{relevance: LevelMultiResponse},
				recvNMEA("CONFIG,MASK,MASK GPS"),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true},
			},
		},
		{
			name: "CONFIG data does not match MASK query",
			tags: []string{"get-mask"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MASK,response: OK"),
				expect{ack: AckAck},
				recvNMEA("CONFIG,PPS,CONFIG PPS ENABLE"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "MASK data does not match CONFIG query",
			tags: []string{"get-config"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG,response: OK"),
				expect{ack: AckAck},
				recvNMEA("CONFIG,MASK,MASK GPS"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "MODE query ACK then UNCA data",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#MODE,81;MODE ROVER SURVEY,"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "MODE query NAK",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: unknown command"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "two different set commands pacing OK",
			tags: []string{"set-pps", "set-sg"},
			events: []event{
				sendEvent{},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("command,CONFIG SIGNALGROUP 2,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same set command pacing blocks",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "GNSS talker NMEA not a response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("GPRMC,123456.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "UNCA data for non-MODE does not match MODE query",
			tags: []string{"get-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE,response: OK"),
				expect{ack: AckAck},
				recvUnicoreAscii("#BESTNAV,81;some data,"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "VERSION query ACK then UNCA data",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("command,VERSION,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#VERSION,97;UM980,R4.10Build17548,"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "data output command one-shot ambiguous",
			tags: []string{"get-versiona"},
			events: []event{
				sendEvent{},
				recvNMEA("command,VERSIONA,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#VERSIONA,97;UM980,R4.10Build17548,"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "data output command periodic ambiguous",
			tags: []string{"set-rectimeb"},
			events: []event{
				sendEvent{},
				recvNMEA("command,RECTIMEB 1,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvUnicoreAscii("#RECTIMEB,97;some data,"),
				expect{relevance: LevelMaybeResponse},
			},
		},
		{
			name: "MODE set command ACK only",
			tags: []string{"set-mode"},
			events: []event{
				sendEvent{},
				recvNMEA("command,MODE ROVER SURVEY,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "COM port speed change no ACK expected",
			tags: []string{"speed-com3"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true}, // silent success is normal
				checkMissing{},                 // not reported as missing
			},
		},
		{
			name: "COM port speed change NAK still reported",
			tags: []string{"speed-com3"},
			events: []event{
				sendEvent{},
				recvNMEA("command,CONFIG COM3 460800,response: not support"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "LOGLIST query ACK then NOVAA data",
			tags: []string{"get-loglist"},
			events: []event{
				sendEvent{},
				recvNMEA("command,LOGLIST,response: OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNOVAA("<LOGLIST COM3 17548 95.000000 FINE 2413 3196.000000 42155794 830 18\r\n"),
				expect{relevance: LevelMultiResponse},
				recvNOVAA("<\t1\r\n"),
				expect{relevance: LevelMultiResponse},
				recvNOVAA("<\tRECTIMEB COM3 1\r\n"),
				expect{relevance: LevelMultiResponse},
				checkDone{canAcceptMore: true},
				checkMissing{},
			},
		},
		{
			name: "LOGLIST query non-NOVAA data ignored",
			tags: []string{"get-loglist"},
			events: []event{
				sendEvent{},
				recvNMEA("command,LOGLIST,response: OK"),
				expect{ack: AckAck},
				recvEmptyTag("some random line\r\n"),
				expect{relevance: LevelNotResponse},
				// untagged '<' fragments no longer count as LOGLIST data
				recvEmptyTag("<garbled fragment"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "LOGLIST query ignores UNCA packets",
			tags: []string{"get-loglist"},
			events: []event{
				sendEvent{},
				recvNMEA("command,LOGLIST,response: OK"),
				expect{ack: AckAck},
				recvUnicoreAscii("#RECTIMEA,95;some data,"),
				expect{relevance: LevelNotResponse},
			},
		},
	})
}
