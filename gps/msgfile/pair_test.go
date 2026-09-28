package msgfile

import "testing"

func TestCorrelatorPAIR(t *testing.T) {
	runCorrelatorTests(t, "pair-test.toml", []correlatorTest{
		{
			name: "set command ACK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command NAK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,3"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "set command no response",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
			},
		},
		{
			name: "set command wait then ACK",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,072,1"),
				expect{ack: AckOther, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query ACK then data",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PAIR073,5"),
				expect{relevance: LevelSoleResponse},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query data before ACK",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR073,5"),
				expect{relevance: LevelSoleResponse},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query NAK no data",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,4"),
				expect{ack: AckNak, relevance: LevelAckOnly, msgIndex: intptr(0)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "query no response",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				checkDone{canAcceptMore: true},
				checkMissing{ack: []int{0}, data: []int{0}},
			},
		},
		{
			name: "two different set commands pacing OK",
			tags: []string{"set-elev", "set-nmea-ver"},
			events: []event{
				sendEvent{},
				readyToSend{want: true}, // different command IDs
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				recvNMEA("PAIR001,100,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "same set command pacing blocks",
			tags: []string{"set-elev", "set-elev-dup"},
			events: []event{
				sendEvent{},
				readyToSend{want: false}, // same command ID 072
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(0)},
				readyToSend{want: true},
				sendEvent{},
				recvNMEA("PAIR001,072,0"),
				expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: intptr(1)},
				checkDone{canAcceptMore: false},
			},
		},
		{
			name: "unrelated PAIR001 not matched",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,100,0"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "GNSS talker NMEA not a response",
			tags: []string{"set-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("GPRMC,123456.00,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"),
				expect{relevance: LevelNotResponse},
			},
		},
		{
			name: "query data does not match wrong command",
			tags: []string{"get-elev"},
			events: []event{
				sendEvent{},
				recvNMEA("PAIR001,073,0"),
				expect{ack: AckAck},
				recvNMEA("PAIR101,1"),
				expect{relevance: LevelNotResponse},
			},
		},
	})
}
