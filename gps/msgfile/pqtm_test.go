package msgfile

import "testing"

func TestCorrelatorPQTM(t *testing.T) {
	runCorrelatorTests(t, "pqtm-test.toml", []correlatorTest{
		{
			name: "write command ACK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "write command NAK",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,ERROR,1"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "write command no response",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "query ACK with data",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK,1,1,100000,1000,0,0"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query bare OK still completes",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				// Bare OK for a query is unusual but the correlator
				// treats any ACK as data-received for expectDataWithAck.
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelSoleResponse, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query NAK",
			tags: []string{"get-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGPPS,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO data without OK",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO NAK",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "PQTMVERNO no response",
			tags: []string{"get-version"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{data: []int{0}},
			},
		},
		{
			name: "two different write commands pacing OK",
			tags: []string{"set-pps", "set-fixrate"},
			events: []event{
				sendEvent{},
				readyToSend{want: true}, // different sentence name, no conflict
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PQTMCFGFIXRATE,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same write command pacing blocks",
			tags: []string{"set-pps", "set-pps-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false}, // same sentence PQTMCFGPPS
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("PQTMCFGPPS,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "unrelated PQTM not matched",
			tags: []string{"set-pps"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMCFGFIXRATE,OK"),
				expect{relevance: LevelNotResponse},
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
			name: "PQTMVERNO pacing unblocks after data received",
			tags: []string{"get-version", "get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
				readyToSend{want: true}, // completed request must not block
			},
		},
		{
			name: "PQTMVERNO NAK after first completed by data",
			tags: []string{"get-version", "get-version"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07"),
				expect{relevance: LevelSoleResponse},
				readyToSend{want: true},
				sendEvent{},
				// NAK must match the second request, not ambiguously match both.
				recvNMEA("PQTMVERNO,ERROR,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "savepar command ACK",
			tags: []string{"savepar"},
			events: []event{
				sendEvent{},
				recvNMEA("PQTMSAVEPAR,OK"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
	})
}
