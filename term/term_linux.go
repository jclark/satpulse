package term

import (
	"fmt"
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
	{unix.B460800, 460800},
	{unix.B921600, 921600},
}

func Speed(speed int) AttrSetter {
	b, ok := speedToB(speed)
	if !ok {
		return func(*Attr) error {
			return fmt.Errorf("invalid terminal speed: %d", speed)
		}
	}
	return func(a *Attr) error {
		a.ts.Cflag &^= unix.CBAUD | unix.CBAUDEX
		a.ts.Cflag |= b
		a.ts.Ospeed = b
		a.ts.Ispeed = b
		return nil
	}
}

func (attr *Attr) speed() int {
	b := attr.ts.Ospeed
	if b == 0 {
		b = attr.ts.Cflag & unix.CBAUD
	}
	speed := bToSpeed(b)
	if speed <= 0 {
		return 0
	}
	return speed
}

func (t *Term) Flush() error {
	return t.wrapErr(unix.IoctlSetInt(t.fd, unix.TCFLSH, unix.TCIOFLUSH), "ioctl(TCFLSH)")
}

func (t *Term) setAttr(attr *unix.Termios) error {
	return t.wrapErr(unix.IoctlSetTermios(t.fd, unix.TCSETS, attr), "ioctl(TCSETS)")
}

func (t *Term) getAttr() (tp *unix.Termios, err error) {
	tp, err = unix.IoctlGetTermios(t.fd, unix.TCGETS)
	err = t.wrapErr(err, "ioctl(TCGETS)")
	return
}

// GetErrorCounts returns counts for serial errors that have occurred since the last call to GetErrorCounts.
// If the serial driver does not provide information about error flags, GetErrorCounts returns a zero value
func (t *Term) GetErrorCounts() (ec ErrorCounts) {
	icNew, err := IoctlGetSerialICounter(t.fd)
	if err != nil {
		t.iCount = nil
		return
	}
	if t.iCount == nil {
		t.iCount = icNew
		return
	}
	ic := t.iCount
	ec.FrameErrs = icNew.Frame - ic.Frame
	ec.OverrunErrs = icNew.Overrun - ic.Overrun
	ec.ParityErrs = icNew.Parity - ic.Parity
	ec.BreakErrs = icNew.Brk - ic.Brk
	ec.BufOverrunErrs = icNew.Buf_overrun - ic.Buf_overrun
	*ic = *icNew
	return
}

func (t *Term) DevKind() DevKind {
	s := unix.Stat_t{}
	err := unix.Fstat(t.fd, &s)
	if err != nil {
		return DevUnknown
	}
	// See https://www.kernel.org/doc/html/latest/admin-guide/devices.html
	switch unix.Major(s.Dev) {
	case 4, 5:
		if unix.Minor(s.Dev) >= 64 { // ttyS0, /dev/ttycua0
			return DevUART
		}
	case 166, 167: // USB ACM "modem" /dev/ttyACM0
		return DevUSB
	case 188, 189: // USB serial converter /dev/ttyUSB0
		return DevUSBtoUART
	case 204, 205: // low-density serial port (Raspberry Pi uses /dev/ttyAMA0)
		return DevUART
	case 216, 217: // Bluetooth RFCOMM /dev/rfcomm0
		return DevBT
	}
	return DevUnknown
}
