package term

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRestoreSettingsMask(t *testing.T) {
	saved := unix.Termios{
		Iflag:  unix.ICRNL | unix.INPCK | unix.IXON | unix.IXOFF | unix.IXANY,
		Oflag:  unix.OPOST,
		Cflag:  unix.B9600 | unix.CS7 | unix.PARENB | unix.PARODD | unix.CMSPAR | unix.CSTOPB | unix.CRTSCTS | unix.HUPCL,
		Lflag:  unix.ICANON | unix.ECHO | unix.ISIG,
		Line:   7,
		Ispeed: 9600,
		Ospeed: 9600,
	}
	saved.Cc[unix.VMIN] = 1
	saved.Cc[unix.VTIME] = 5
	current := unix.Termios{
		Cflag:  unix.BOTHER | unix.BOTHER<<16 | unix.CS8 | unix.CLOCAL | unix.CREAD,
		Ispeed: 123457,
		Ospeed: 234567,
	}
	current.Cc[unix.VTIME] = 1
	want := saved
	want.Cflag = unix.BOTHER | unix.BOTHER<<16 | unix.CS8 | unix.HUPCL
	want.Iflag = unix.ICRNL | unix.INPCK
	want.Ispeed = 123457
	want.Ospeed = 234567
	if got := restoreSettings(saved, current); got != want {
		t.Errorf("restoreSettings = %+v, want %+v", got, want)
	}
	// Also test restoration with the saved and current attributes swapped.
	want = current
	want.Cflag = unix.B9600 | unix.CS7 | unix.PARENB | unix.PARODD | unix.CMSPAR | unix.CSTOPB | unix.CRTSCTS | unix.CLOCAL | unix.CREAD
	want.Iflag = unix.IXON | unix.IXOFF | unix.IXANY
	want.Ispeed = 9600
	want.Ospeed = 9600
	if got := restoreSettings(current, saved); got != want {
		t.Errorf("reverse restoreSettings = %+v, want %+v", got, want)
	}
}

func TestRestoreSettingsPTY(t *testing.T) {
	path := newTestPTY(t)
	fd := openTestTTY(t, path)
	defer unix.Close(fd)
	saved, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		t.Fatal(err)
	}
	port := &unixTerm{fd: fd, tsSaved: *saved}
	attr := Attr{*saved}
	for _, opt := range []AttrSetter{RawMode, NoFlowControl, Speed(38400), ReadTimeout(time.Second)} {
		if err := opt(&attr); err != nil {
			t.Fatal(err)
		}
	}
	if err := port.setAttrNow(&attr.ts); err != nil {
		t.Fatal(err)
	}
	current, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		t.Fatal(err)
	}
	if err := port.Restore(true); err != nil {
		t.Fatal(err)
	}
	got, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		t.Fatal(err)
	}
	if want := restoreSettings(*saved, *current); *got != want {
		t.Errorf("restored PTY = %+v, want %+v", *got, want)
	}
	checkTestSpeed(t, fd, unix.B38400, 0, 38400)
}
