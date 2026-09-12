package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var septentrioReplayFiles = []string{
	"g5-noop",
	"g5-show-port",
	"g5-binary",
	"g5-min-elev",
	"g5-pps",
	"g5-signal",
	"g5-pvt-out",
	"g5-raw-out",
	"g5-rtcm-out",
	"g5-reload",
	"g5-tmode",
}

func TestReplaySeptentrio(t *testing.T) {
	for _, filename := range septentrioReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "septentrio/"+filename, septentrioPacketsEqual)
		})
	}
}

func septentrioPacketsEqual(t *testing.T, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()

	if actualStr == expectedStr {
		return true, false
	}

	t.Errorf("packets differ: actual %q, expected %q", actualStr, expectedStr)
	return false, false
}
