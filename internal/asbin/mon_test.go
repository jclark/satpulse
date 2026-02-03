package asbin

import "testing"

func TestMonVer(t *testing.T) {
	// Captured from TAU1201: SW 3.018.aab95e7, HW HD8040D.9529b663
	runTests(t, []testCase{{
		name:   "tau1201",
		packet: "f1d90a042000 332e3031382e61616239356537000000 484438303430442e3935323962363633 2821",
		wantID: MonVerID,
		wantMsg: &MonVer{
			SwVersion: [16]byte{'3', '.', '0', '1', '8', '.', 'a', 'a', 'b', '9', '5', 'e', '7', 0, 0, 0},
			HwVersion: [16]byte{'H', 'D', '8', '0', '4', '0', 'D', '.', '9', '5', '2', '9', 'b', '6', '6', '3'},
		},
	}})
}
