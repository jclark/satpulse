package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNoParity(t *testing.T) {
	attr := Attr{
		dcb: windows.DCB{
			Parity: windows.EVENPARITY,
			Flags:  dcbParity | dcbBinary,
		},
	}
	if err := NoParity(&attr); err != nil {
		t.Fatalf("NoParity: %v", err)
	}
	if attr.dcb.Parity != windows.NOPARITY {
		t.Errorf("NoParity left Parity = %d, want NOPARITY", attr.dcb.Parity)
	}
	if attr.dcb.Flags&dcbParity != 0 {
		t.Error("NoParity left fParity enabled")
	}
	if attr.dcb.Flags&dcbBinary == 0 {
		t.Error("NoParity changed unrelated flags")
	}
}

func TestRawModeLeavesParityChecking(t *testing.T) {
	attr := Attr{dcb: windows.DCB{Flags: dcbParity}}
	if err := RawMode(&attr); err != nil {
		t.Fatalf("RawMode: %v", err)
	}
	if attr.dcb.Flags&dcbParity == 0 {
		t.Error("RawMode disabled input parity checking")
	}
}

func TestCommEventMask(t *testing.T) {
	tests := []struct {
		name      string
		pin       ModemControlPin
		expect    uint32
		expectErr bool
	}{
		{name: "CTS", pin: ModemCTS, expect: windows.EV_CTS},
		{name: "DCD", pin: ModemDCD, expect: windows.EV_RLSD},
		{name: "DSR", pin: ModemDSR, expect: windows.EV_DSR},
		{name: "RI", pin: ModemRI, expect: windows.EV_RING},
		{name: "invalid", pin: ModemControlPin(99), expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := commEventMask(tc.pin)
			if tc.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("commEventMask: %v", err)
			}
			if got != tc.expect {
				t.Errorf("got  %d\nwant %d", got, tc.expect)
			}
		})
	}
}

// TestPinWatchCancel checks that cancellation is sticky and is observed
// before WaitCommEvent: the watch holds an invalid handle, so anything
// reaching the wait would fail with a handle error instead.
func TestPinWatchCancel(t *testing.T) {
	w := &pinWatch{handle: windows.InvalidHandle, pin: ModemCTS}
	w.Cancel()
	if c, missed, err := w.Wait(); !errors.Is(err, ErrCancelled) || c != (ModemControlPinChange{}) || missed != 0 {
		t.Fatalf("ModemControlPinWatch.Wait after cancel = %+v, %d, %v; want zero change, 0, ErrCancelled", c, missed, err)
	}
}

// TestOpenFallbackPipe exercises the Windows named-pipe fallback end to end:
// a live pipe server on one end, term.OpenFallback (the daemon's path) on the
// other. It is the proof that os.NewFile gives an overlapped pipe handle a
// working SetReadDeadline (Go 1.25+): a read returns data when present, times
// out when the deadline passes with no data, and reports EOF on disconnect.
func TestOpenFallbackPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\satpulse-term-test-%d`, os.Getpid())
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	srv, err := windows.CreateNamedPipe(namePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatalf("CreateNamedPipe: %v", err)
	}
	srvOpen := true
	defer func() {
		if srvOpen {
			windows.CloseHandle(srv)
		}
	}()

	pf, wf, kind, err := OpenFallback(name, time.Second)
	if err != nil {
		t.Fatalf("OpenFallback: %v", err)
	}
	defer pf.Close()
	if kind != DevFIFO {
		t.Errorf("kind = %v, want DevFIFO", kind)
	}
	if wf != nil {
		t.Errorf("wf = %v, want nil (Windows returns an *os.File)", wf)
	}

	// The client's CreateFile has already connected, so ConnectNamedPipe
	// reports ERROR_PIPE_CONNECTED rather than blocking; treat that as success.
	if err := windows.ConnectNamedPipe(srv, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		t.Fatalf("ConnectNamedPipe: %v", err)
	}

	// Data present: a read within the deadline returns it.
	want := []byte("hello")
	var wrote uint32
	if err := windows.WriteFile(srv, want, &wrote, nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := pf.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 16)
	n, err := pf.Read(buf)
	if err != nil {
		t.Fatalf("Read with data: %v", err)
	}
	if got := buf[:n]; string(got) != string(want) {
		t.Errorf("Read = %q, want %q", got, want)
	}

	// No data: the read must return a timeout once the deadline passes, not
	// block forever -- this is the property the scan loop relies on.
	if err := pf.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if _, err := pf.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read with no data: err = %v, want deadline exceeded", err)
	}

	// Disconnect: closing the server end surfaces as EOF to the reader.
	windows.CloseHandle(srv)
	srvOpen = false
	if err := pf.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if _, err := pf.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("Read after disconnect: err = %v, want EOF", err)
	}
}
