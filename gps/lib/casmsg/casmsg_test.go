package casmsg

import "testing"

func TestSentences(t *testing.T) {
	tests := []struct {
		name   string
		got    string
		expect string
	}{
		// Expected strings verified on ATGM332D-5N71 hardware.
		{"query version", Query(QueryFirmwareVersion), "$PCAS06,0*1B\r\n"},
		{"query mode", Query(QueryWorkingMode), "$PCAS06,2*19\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expect {
				t.Errorf("got  %q\nwant %q", tc.got, tc.expect)
			}
		})
	}
}

func TestParseTxtInfo(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		expectKey   string
		expectValue string
		expectOK    bool
	}{
		{
			name:        "SW version reply",
			payload:     "GPTXT,01,01,02,SW=URANUS5,V5.3.0.0",
			expectKey:   "SW",
			expectValue: "URANUS5,V5.3.0.0",
			expectOK:    true,
		},
		{
			name:        "MO mode reply",
			payload:     "GPTXT,01,01,02,MO=GBR",
			expectKey:   "MO",
			expectValue: "GBR",
			expectOK:    true,
		},
		{
			name:     "antenna status is not a query reply",
			payload:  "GPTXT,01,01,01,ANTENNA OK",
			expectOK: false,
		},
		{
			name:     "not GPTXT",
			payload:  "GNRMC,054912.000,A",
			expectOK: false,
		},
		{
			name:     "too few fields",
			payload:  "GPTXT,01,01",
			expectOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, value, ok := ParseTxtInfo(tc.payload)
			if ok != tc.expectOK || key != tc.expectKey || value != tc.expectValue {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					key, value, ok, tc.expectKey, tc.expectValue, tc.expectOK)
			}
		})
	}
}
