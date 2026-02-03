package asbin

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

// testCase defines a table-driven test case for message parsing.
type testCase struct {
	name    string // test name
	packet  string // hex-encoded packet (spaces allowed)
	wantID  MsgID  // expected message ID
	wantMsg Msg    // expected decoded message (compared with reflect.DeepEqual)
}

// runTest parses the packet and verifies the message ID and decoded payload.
func runTest(t *testing.T, tc testCase) {
	t.Helper()
	packetHex := strings.ReplaceAll(tc.packet, " ", "")
	packetBytes, err := hex.DecodeString(packetHex)
	if err != nil {
		t.Fatalf("invalid hex in test case: %v", err)
	}
	msg, err := ParseMsg(string(packetBytes))
	if err != nil {
		t.Fatalf("ParseMsg failed: %v", err)
	}
	if msg.ID() != tc.wantID {
		t.Errorf("ID = %v, want %v", msg.ID(), tc.wantID)
	}
	if !reflect.DeepEqual(msg, tc.wantMsg) {
		t.Errorf("message mismatch:\ngot:  %+v\nwant: %+v", msg, tc.wantMsg)
	}
}

// runTests runs a slice of test cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, tc)
		})
	}
}
