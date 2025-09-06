package sdpcmd

import (
	"net"
	"testing"
)

// Test formatStatus helper
func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name  string
		flags net.Flags
		want  string
	}{
		{
			name:  "interface down",
			flags: 0,
			want:  "down",
		},
		{
			name:  "interface up no carrier",
			flags: net.FlagUp,
			want:  "up, no-carrier",
		},
		{
			name:  "interface up with carrier",
			flags: net.FlagUp | net.FlagRunning,
			want:  "up, carrier",
		},
		{
			name:  "interface with other flags",
			flags: net.FlagUp | net.FlagRunning | net.FlagBroadcast | net.FlagMulticast,
			want:  "up, carrier",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStatus(tt.flags)
			if got != tt.want {
				t.Errorf("formatStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}