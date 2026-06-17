//go:build linux

package phc

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/jclark/satpulse/gps/ptime"
	"golang.org/x/sys/unix"
)

// TestReadExttsTime checks that ReadExtts reconstructs the full int64 seconds
// of an external-timestamp event. On 32-bit builds an intermediate int32
// conversion truncated any time past 2038-01-19 (math.MaxInt32 seconds),
// silently corrupting every PPS/external timestamp from then on.
func TestReadExttsTime(t *testing.T) {
	type result struct {
		Time  ptime.Time
		Index uint32
	}
	tests := []struct {
		name   string
		sec    int64
		nsec   uint32
		index  uint32
		expect result
	}{
		{
			name:   "pre-2038",
			sec:    1735730167, // 2025-01-01
			nsec:   250000000,
			index:  1,
			expect: result{ptime.Unix(1735730167, 250000000), 1},
		},
		{
			name:   "post-2038",
			sec:    int64(1)<<31 + 5, // 2147483653, just past the int32 seconds limit
			nsec:   123456789,
			index:  2,
			expect: result{ptime.Unix(int64(1)<<31+5, 123456789), 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := unix.PtpExttsEvent{
				T:     unix.PtpClockTime{Sec: tc.sec, Nsec: tc.nsec},
				Index: tc.index,
			}
			fds := make([]int, 2)
			if err := unix.Pipe(fds); err != nil {
				t.Fatalf("pipe: %v", err)
			}
			defer unix.Close(fds[0])
			defer unix.Close(fds[1])
			buf := unsafe.Slice((*byte)(unsafe.Pointer(&event)), int(unsafe.Sizeof(event)))
			if _, err := unix.Write(fds[1], buf); err != nil {
				t.Fatalf("write: %v", err)
			}
			clk := &Clock{fd: fds[0]}
			gotTime, gotIndex, err := clk.ReadExtts()
			if err != nil {
				t.Fatalf("ReadExtts: %v", err)
			}
			got := result{gotTime, gotIndex}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v (%v)\nwant %+v (%v)", got, gotTime, tc.expect, tc.expect.Time)
			}
		})
	}
}
