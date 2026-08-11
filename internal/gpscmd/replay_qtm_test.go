package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var qtmReplayFiles = []string{
	"lg290p-noop",
	"lg290p-nmea-out",
	"lg290p-pvt-out",
	"lg290p-sats-out",
	"lg290p-rtcm-out",
	"lg290p-min-elev",
	"lg290p-pps",
	"lg290p-speed",
	"lg290p-reload",
	"lg290p-show-port",
	"lg290p-signals",
	"lg290p-mode",
}

func TestReplayQTM(t *testing.T) {
	for _, filename := range qtmReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "quectel/"+filename, qtmPacketsEqual)
		})
	}
}

// qtmPacketsEqual compares Quectel PQTM packets, which are plain NMEA
// sentences, so an exact byte comparison suffices.
func qtmPacketsEqual(t *testing.T, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()
	if actualStr == expectedStr {
		return true, false
	}
	t.Errorf("packets differ: actual %q, expected %q", actualStr, expectedStr)
	return false, false
}
