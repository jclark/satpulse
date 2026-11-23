package soc

import "testing"

func TestGPIONumber(t *testing.T) {
	tests := []struct {
		name     string
		expect   int
		expectOK bool
	}{
		{"GPIO0", 0, true},
		{"GPIO18", 18, true},
		{"GPIO018", 0, false},
		{"GPIO", 0, false},
		{"GPIO-1", 0, false},
		{"GPIO+1", 0, false},
		{"GPIO1A", 0, false},
		{"18", 0, false},
		{"GPIO99999999999999999999", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := GPIONumber(tc.name)
			if n != tc.expect || ok != tc.expectOK {
				t.Errorf("got %d, %v; want %d, %v", n, ok, tc.expect, tc.expectOK)
			}
		})
	}
}
