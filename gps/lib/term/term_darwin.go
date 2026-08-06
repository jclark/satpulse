package term

import (
	"strings"

	"golang.org/x/sys/unix"
)

var baudRates = []struct {
	b     uint32
	speed int
}{
	{unix.B50, 50},
	{unix.B75, 75},
	{unix.B110, 110},
	{unix.B134, 134},
	{unix.B150, 150},
	{unix.B200, 200},
	{unix.B300, 300},
	{unix.B600, 600},
	{unix.B1200, 1200},
	{unix.B1800, 1800},
	{unix.B2400, 2400},
	{unix.B4800, 4800},
	{unix.B9600, 9600},
	{unix.B19200, 19200},
	{unix.B38400, 38400},
	{unix.B57600, 57600},
	{unix.B115200, 115200},
	{unix.B230400, 230400},
	{460800, 460800},
	{921600, 921600},
}

func (t *unixTerm) DevKind() DevKind {
	before, after, ok := strings.Cut(t.path, ".")
	if ok && (before == "/dev/cu" || before == "/dev/tty") {
		if strings.HasPrefix(after, "usbmodem") {
			return DevUSB
		} else if strings.HasPrefix(after, "usbserial") {
			return DevUSBtoUART
		}
	}
	return DevUnknown
}
