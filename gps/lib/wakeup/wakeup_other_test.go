//go:build !linux

package wakeup

import (
	"errors"
	"testing"
	"time"
)

func TestUnsupported(t *testing.T) {
	if LatencyResolution != time.Nanosecond {
		t.Errorf("LatencyResolution = %v, want %v", LatencyResolution, time.Nanosecond)
	}
	if _, err := RequestLatencyLimit(0); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("RequestLatencyLimit error = %v, want errors.ErrUnsupported", err)
	}
}
