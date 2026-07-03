package asbin

import "testing"

func TestRxmDumpRaw(t *testing.T) {
	// Both packets are examples from the protocol spec, section 5.2.1.
	runTests(t, []testCase{
		{
			name:    "enable",
			packet:  "F1D9020101 0001 0512",
			wantID:  RxmDumpRawID,
			wantMsg: &RxmDumpRaw{Enable: 1},
		},
		{
			name:    "disable",
			packet:  "F1D9020101 0000 0411",
			wantID:  RxmDumpRawID,
			wantMsg: &RxmDumpRaw{Enable: 0},
		},
	})
}
