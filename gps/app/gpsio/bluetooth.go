package gpsio

import (
	"github.com/jclark/satpulse/gps/lib/term"
)

// OpenBluetooth opens an RFCOMM SPP connection to the given Bluetooth
// Device Address. The remote device must already be paired.
func OpenBluetooth(addr term.BDAddr) (*SerialConn, error) {
	f, err := term.OpenBT(addr)
	if err != nil {
		return nil, err
	}
	return newSerialConn(newPollingFile(f, readTimeout), term.DevBT), nil
}
