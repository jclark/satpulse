//go:build linux

package wakeup

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// LatencyResolution is the resolution of wakeup latency limits on Linux.
	LatencyResolution = time.Microsecond
	cpuDMALatencyPath = "/dev/cpu_dma_latency"
	maxLatencyMicros  = 1<<31 - 1
)

// RequestLatencyLimit requests that CPU idle-state exit add no more than max
// to wakeup latency. The request remains active until the returned closer is
// closed. Linux represents the request in whole microseconds, so max is
// rounded down to LatencyResolution.
func RequestLatencyLimit(max time.Duration) (io.Closer, error) {
	return requestLatencyLimit(cpuDMALatencyPath, max)
}

func requestLatencyLimit(path string, max time.Duration) (io.Closer, error) {
	if max < 0 {
		return nil, fmt.Errorf("wakeup latency limit must not be negative: %v", max)
	}
	micros := max / LatencyResolution
	if micros > maxLatencyMicros {
		return nil, fmt.Errorf("wakeup latency limit is too large: %v", max)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var value [4]byte
	binary.NativeEndian.PutUint32(value[:], uint32(micros))
	n, err := f.Write(value[:])
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("setting wakeup latency limit: %w", err)
	}
	if n != len(value) {
		_ = f.Close()
		return nil, fmt.Errorf("setting wakeup latency limit: %w", io.ErrShortWrite)
	}
	return f, nil
}
