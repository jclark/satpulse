package logobs

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestUBXLogObserverMonCommsAllocWarnRateLimited(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	obs := NewUBXLogObserver(lg)
	tRead := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := &ubxbin.MonComms{
		MonCommsFixed: ubxbin.MonCommsFixed{
			TxErrors: ubxbin.MonCommsTxErrAlloc | ubxbin.MonCommsTxErrors(ubxbin.PortUSB+1)<<2,
		},
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead) {
		t.Fatal("NativeMsg returned false")
	}
	output := buf.String()
	if !strings.Contains(output, "GPS receiver reports its transmit buffer has overflowed") {
		t.Fatalf("missing overflow warning in log output %q", output)
	}

	buf.Reset()
	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead.Add(monCommsAllocWarnInterval-time.Second)) {
		t.Fatal("NativeMsg returned false")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning during rate limit interval: %q", buf.String())
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead.Add(monCommsAllocWarnInterval)) {
		t.Fatal("NativeMsg returned false")
	}
	if !strings.Contains(buf.String(), "GPS receiver reports its transmit buffer has overflowed") {
		t.Fatalf("missing overflow warning after rate limit interval in log output %q", buf.String())
	}
}

func TestUBXLogObserverMonCommsNoAllocNoWarn(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	obs := NewUBXLogObserver(lg)
	msg := &ubxbin.MonComms{
		MonCommsFixed: ubxbin.MonCommsFixed{
			TxErrors: ubxbin.MonCommsTxErrMem,
		},
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, time.Now()) {
		t.Fatal("NativeMsg returned false")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning without alloc bit: %q", buf.String())
	}
}
