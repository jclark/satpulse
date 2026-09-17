package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var uncReplayFiles = []string{
	"um980-show",
	"um980-gps",
	"um980-noop",
	"um980-pvt-out",
	"um980-rtcm-out",
	"um980-binary",
	"um980-min-elev",
	"um980-pps",
	"um980-signal",
	"um980-tmode",
	"um980-raw-out",
}

func TestReplayUNC(t *testing.T) {
	for _, filename := range uncReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "unicore/"+filename, uncPacketsEqual)
		})
	}
}

func uncPacketsEqual(t *testing.T, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()

	if actualStr == expectedStr {
		return true, false
	}

	t.Errorf("packets differ: actual %q, expected %q", actualStr, expectedStr)
	return false, false
}
