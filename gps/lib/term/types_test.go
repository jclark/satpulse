package term

import "testing"

func TestModemControlLineStateAsserted(t *testing.T) {
	state := modemControlLineState(ModemCTS, ModemDSR)
	tests := []struct {
		line ModemControlLine
		want bool
	}{
		{ModemCTS, true},
		{ModemDCD, false},
		{ModemDSR, true},
		{ModemRI, false},
		{ModemControlLine(-1), false},
		{ModemRI + 1, false},
	}
	for _, tc := range tests {
		if got := state.Asserted(tc.line); got != tc.want {
			t.Errorf("Asserted(%d) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
