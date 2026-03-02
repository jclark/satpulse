package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var uncReplayFiles = []string{
	"unc/show",
	"unc/gps",
}

func TestReplayUNC(t *testing.T) {
	for _, filename := range uncReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, filename, uncPacketsEqual)
		})
	}
}

func uncPacketsEqual(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()

	if actualStr == expectedStr {
		return true, false
	}

	t.Errorf("%s: packets differ: actual %q, expected %q", msgID, actualStr, expectedStr)
	return false, false
}
