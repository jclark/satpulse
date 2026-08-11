//go:build windows

package timeprov

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/sockrefclock"
)

// TestRecord checks the wire layout against independently computed
// field values at their documented offsets (pipe-timeprov wire format
// version 1).
func TestRecord(t *testing.T) {
	tests := []struct {
		name       string
		sys        ntime.Time
		offset     float64
		dispersion uint64
		leap       sockrefclock.Leap
		expectSys  uint64
		expectOff  int64
	}{
		{
			name:      "zero",
			expectSys: 116444736000000000,
		},
		{
			name:       "positive offset",
			sys:        ntime.Time(1723276800123456789),
			offset:     0.25,
			dispersion: 10000,
			leap:       sockrefclock.LeapInsert,
			expectSys:  116444736000000000 + 17232768001234567,
			expectOff:  2500000,
		},
		{
			name:      "negative offset",
			sys:       ntime.Time(1000000000000000000),
			offset:    -0.0001235,
			expectSys: 116444736000000000 + 10000000000000000,
			expectOff: -1235,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expect := make([]byte, 32)
			le := binary.LittleEndian
			le.PutUint32(expect[0:], 0x31565054)
			expect[4] = byte(tc.leap)
			le.PutUint64(expect[8:], tc.expectSys)
			le.PutUint64(expect[16:], uint64(tc.expectOff))
			le.PutUint64(expect[24:], tc.dispersion)
			got := record(tc.sys, tc.offset, tc.dispersion, tc.leap)
			if !reflect.DeepEqual(got, expect) {
				t.Errorf("got  %x\nwant %x", got, expect)
			}
		})
	}
}
