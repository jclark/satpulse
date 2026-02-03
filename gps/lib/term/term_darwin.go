package term

import "strings"

func (t *Term) DevKind() DevKind {
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
