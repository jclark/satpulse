package asbin

import "testing"

func TestAckNak(t *testing.T) {
	// Example from spec: NAK for CFG-MSG with invalid payload
	runTests(t, []testCase{{
		name:   "cfg_msg_invalid",
		packet: "F1D9050002 000601 0E33",
		wantID: AckNakID,
		wantMsg: &AckNak{
			MsgClass: 0x06,
			MsgID:    0x01,
		},
	}})
}

func TestAckAck(t *testing.T) {
	// Example from spec: ACK for CFG-SIMPLERST
	runTests(t, []testCase{{
		name:   "cfg_simplerst",
		packet: "F1D9050102 000640 4E77",
		wantID: AckAckID,
		wantMsg: &AckAck{
			MsgClass: 0x06,
			MsgID:    0x40,
		},
	}})
}
