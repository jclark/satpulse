package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var casicReplayFiles = []string{
	"atgm332d-5n71-noop",
	"atgm332d-5n71-binary",
	"atgm332d-5n71-signal",
	"atgm332d-5n71-pps",
	"atgm332d-5n71-pvt-out",
	"atgm332d-5n71-tmode",
	"atgm332d-5n71-min-elev",
	"atgm332d-5n71-show-port",
}

func TestReplayCASIC(t *testing.T) {
	for _, filename := range casicReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "zhongke/"+filename, casicPacketsEqual)
		})
	}
}

func casicPacketsEqual(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()
	if actualStr == expectedStr {
		return true, false
	}
	t.Errorf("%s: packets differ: actual %q, expected %q", msgID, actualStr, expectedStr)
	return false, false
}
