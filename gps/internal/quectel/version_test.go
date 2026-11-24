package quectel

import "testing"

func TestParseFwVersion(t *testing.T) {
	tests := []struct {
		name   string
		verStr string
		expect fwVersion
	}{
		{
			name:   "R02A01S", // verbatim from the connected unit
			verStr: "LG290P03AANR02A01S",
			expect: fwVersion{major: 2, minor: 1},
		},
		{
			name:   "R01A03S",
			verStr: "LG290P03AANR01A03S",
			expect: fwVersion{major: 1, minor: 3},
		},
		{
			name:   "other family module",
			verStr: "LG580P03AANR01A02S",
			expect: fwVersion{},
		},
		{
			name:   "unrecognized string",
			verStr: "MODULE-X 1.2.3",
			expect: fwVersion{},
		},
		{
			name:   "empty",
			verStr: "",
			expect: fwVersion{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFwVersion(tc.verStr); got != tc.expect {
				t.Errorf("got %+v, want %+v", got, tc.expect)
			}
		})
	}
}

func TestFwVersionHas(t *testing.T) {
	tests := []struct {
		name   string
		v      fwVersion
		min    fwVersion
		expect bool
	}{
		{name: "newer major", v: fwVersion{2, 1}, min: fwVersion{1, 6}, expect: true},
		{name: "equal", v: fwVersion{1, 5}, min: fwVersion{1, 5}, expect: true},
		{name: "older minor", v: fwVersion{1, 4}, min: fwVersion{1, 5}, expect: false},
		{name: "older major newer minor", v: fwVersion{1, 9}, min: fwVersion{2, 1}, expect: false},
		{name: "unknown is optimistic", v: fwVersion{}, min: fwVersion{2, 1}, expect: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.has(tc.min); got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}
