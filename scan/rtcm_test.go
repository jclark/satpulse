package scan

import (
	"context"
	"io"
	"strings"
	"testing"
)

// Example from RTCM 10403.2, section 4.2
// Note that \x escape in a Go string literal represents a byte not a rune.
const rtcmEx = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestGoodRTCM(t *testing.T) {
	rtcmOK(t, 1005, string(rtcmEx))
}

func rtcmOK(t *testing.T, msgNum uint16, data string) {
	ctx := context.Background()
	r := strings.NewReader(data)
	s := New(r, 64)
	f, err := s.Scan(ctx)
	if err != nil {
		t.Fatalf(`error reading RTCM %d`, msgNum)
	}
	if f.Kind != RTCM {
		t.Fatalf(`RTCM %d not recognized as valid`, msgNum)
	}
	if f.Data != data {
		t.Fatalf(`wrong data for %q`, msgNum)
	}
	f, err = s.Scan(ctx)
	if err != io.EOF || f.Kind != Invalid || f.Data != "" {
		t.Fatalf(`did not get EOF with no data after RTCM %d`, msgNum)
	}
	_, ok, n := RTCMMsg(data)
	if !ok {
		t.Fatalf(`checksum of RTCM %d not recognized as valid`, msgNum)
	}
	if n != msgNum {
		t.Fatalf(`wrong message number for RTCM %d`, msgNum)
	}
}
