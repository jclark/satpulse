package rtcm

import (
	"testing"

	"github.com/jclark/satpulse/internal/scantest"
)

// Example from RTCM 10403.2, section 4.2
// Note that \x escape in a Go string literal represents a byte not a rune.
const rtcmEx = "\xD3\x00\x13\x3E\xD7\xD3\x02\x02\x98\x0E\xDE\xEF\x34\xB4\xBD\x62\xAC\x09\x41\x98\x6F\x33\x36\x0B\x98"

func TestGoodRTCM(t *testing.T) {
	rtcmOK(t, 1005, string(rtcmEx))
}

func rtcmOK(t *testing.T, msgNum uint16, data string) {
	buf := ([]byte)(data)
	startPos, endPos, ok := scantest.FindPacket(PacketFormat, buf)
	if startPos != 0 || endPos != len(buf) || !ok {
		t.Fatalf("failed to scan valid RTCM packet")
	}
	_, ok, n := RTCMMsg(data)
	if !ok {
		t.Fatalf(`checksum of RTCM %d not recognized as valid`, msgNum)
	}
	if n != msgNum {
		t.Fatalf(`wrong message number for RTCM %d`, msgNum)
	}
}
