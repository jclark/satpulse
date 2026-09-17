package term

import "testing"

func TestDevKind_Darwin(t *testing.T) {
	tests := []struct {
		path     string
		expected DevKind
	}{
		// USB modem devices (direct USB GPS receivers)
		{"/dev/cu.usbmodem12345", DevUSB},
		{"/dev/cu.usbmodemAB0MHJAU1", DevUSB},
		{"/dev/tty.usbmodem54321", DevUSB},

		// USB-to-serial devices (like FTDI, CP210x, etc.)
		{"/dev/cu.usbserial-AB0MHJAU", DevUSBtoUART},
		{"/dev/cu.usbserial-1234", DevUSBtoUART},
		{"/dev/tty.usbserial-ABCD", DevUSBtoUART},
		{"/dev/cu.usbserialABC123", DevUSBtoUART},

		// Bluetooth devices
		{"/dev/cu.Bluetooth-Incoming-Port", DevUnknown},

		// Hardware UART devices
		{"/dev/cu.serial1", DevUnknown},

		// Invalid/unknown paths
		{"/dev/cu", DevUnknown},
		{"/dev/ttyS0", DevUnknown},
		{"/dev/cu.usbhid-something", DevUnknown},
		{"", DevUnknown},
		{"cu.usbserial-1234", DevUnknown},
		{"/dev/cu.usbserial.extra.parts", DevUSBtoUART},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// Create a minimal Unix terminal with just the path set.
			term := &unixTerm{path: tt.path}
			result := term.DevKind()

			if result != tt.expected {
				t.Errorf("DevKind() = %v, want %v", result, tt.expected)
			}
		})
	}
}
