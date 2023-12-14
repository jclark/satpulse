package phc

import (
	"testing"
)

func TestParsePhyID(t *testing.T) {
	testCases := []struct {
		s  string
		ok bool
		id PhyID
	}{
		{"0x600d84a2\n", true, 0x600d84a2},
		{"0xF600d84a0\n", false, 0},
		{"0xabcdef12\nextra junk", true, 0xabcdef12},
		{"not a hex number\n", false, 0},
		{"", false, 0},
	}

	for _, tc := range testCases {
		id, err := parsePhyID(tc.s)
		if (err == nil) != tc.ok {
			t.Errorf("parsePhyID(%q) returned error: %v, want error: %v", tc.s, err, !tc.ok)
		}
		if id != tc.id {
			t.Errorf("parsePhyID(%q) = %v, want %v", tc.s, id, tc.id)
		}
	}
}

func TestPhyIDDriverFlags(t *testing.T) {
	id, err := parsePhyID("0x600d84a2\n")
	if err != nil {
		t.Fatal(err)
	}
	if PhyIDDriverFlags(id)&(DriverKnown|DriverBothEdges|DriverPoll4Hz) != DriverKnown|DriverPoll4Hz {
		t.Errorf("PhyIDDriverFlags(%v) = %v, want %v", id, PhyIDDriverFlags(id), DriverKnown|DriverPoll4Hz)	
	}
}
