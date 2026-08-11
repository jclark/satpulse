package timeprov

//go:generate sh -c "{ echo '//go:build windows'; echo; go tool cgo -godefs types_windows.go; } | gofmt > ztypes_windows.go && rm -rf _obj _cgo_*.o"

import (
	"fmt"
	"math"
	"os"
	"strings"
	"unsafe"

	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/sockrefclock"
)

// filetimeEpoch is the Unix epoch in 100 ns intervals since the NT
// epoch 1601-01-01 UTC.
const filetimeEpoch = 116444736000000000

// record builds a pipe-timeprov sample record. The chrony SOCK leap
// values used by the daemon coincide with the wire encoding (0 none,
// 1 insert, 2 delete). The struct layout is generated from wire.h, so
// on little-endian windows/amd64 the byte copy matches the wire order.
func record(sys ntime.Time, offset float64, dispersion uint64, leap sockrefclock.Leap) []byte {
	s := pipeSample{
		Magic:      recMagic,
		Leap:       uint8(leap),
		SysTime:    uint64(int64(sys)/100 + filetimeEpoch),
		Offset:     int64(math.Round(offset * 1e7)),
		Dispersion: dispersion,
	}
	var buf [recSize]byte
	type sampBufPtr *[recSize]byte
	buf = *(sampBufPtr)(unsafe.Pointer(&s))
	return buf[:]
}

type pipeConn struct {
	path string
}

func newPipeConn(path string) (*pipeConn, error) {
	if !strings.HasPrefix(path, `\\.\pipe\`) {
		return nil, fmt.Errorf(`timeprov pipe path %q must start with \\.\pipe\`, path)
	}
	return &pipeConn{path: path}, nil
}

// send opens the pipe anew for each sample, mirroring a fire-and-forget
// datagram send: no connection state outlives a sample, so the provider
// (and w32time with it) can restart freely, and a missing server fails
// only that sample. Each write to the message-mode pipe is delivered as
// one message.
func (c *pipeConn) send(pkt []byte) error {
	f, err := os.OpenFile(c.path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("could not open timeprov pipe: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(pkt); err != nil {
		return fmt.Errorf("could not send timeprov sample: %w", err)
	}
	return nil
}

func (c *pipeConn) close() error {
	return nil
}
