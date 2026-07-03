package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var asReplayFiles = []string{
	"tau1201-noop",
	"tau1201-pvt-out",
	"tau1201-signal",
	"tau1201-pps",
	"tau1201-min-elev",
	"tau1201-rtcm-out",
	"tau1201-raw-out",
	"tau951m-noop",
	"tau951m-show-port",
	"tau951m-min-elev",
	"tau951m-pvt-out",
	"tau951m-signal",
	"tau951m-pps",
	"tau951m-raw-out",
	"tau951m-rtcm-out",
	"tau951m-tmode",
	"tau951m-reload",
	"tau951m-speed",
	"tau1302-noop",
	"tau1302-rtcm-out",
	"tau1302-signal",
}

func TestReplayAS(t *testing.T) {
	for _, filename := range asReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "allystar/"+filename, asPacketsEqual)
		})
	}
}

func asPacketsEqual(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()

	if actualStr == expectedStr {
		return true, false
	}

	t.Errorf("%s: packets differ: actual %q, expected %q", msgID, actualStr, expectedStr)
	return false, false
}
