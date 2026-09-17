package term

import "testing"

func TestModemControlPinStateAsserted(t *testing.T) {
	state := modemControlPinState(ModemCTS, ModemDSR)
	tests := []struct {
		pin  ModemControlPin
		want bool
	}{
		{ModemCTS, true},
		{ModemDCD, false},
		{ModemDSR, true},
		{ModemRI, false},
		{ModemControlPin(-1), false},
		{ModemRI + 1, false},
	}
	for _, tc := range tests {
		if got := state.Asserted(tc.pin); got != tc.want {
			t.Errorf("Asserted(%d) = %v, want %v", tc.pin, got, tc.want)
		}
	}
}
