package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

var casicReplayFiles = []string{
	// ATGM332D-5N71 (V5)
	"atgm332d-5n71-noop",
	"atgm332d-5n71-signal",
	"atgm332d-5n71-pps",
	"atgm332d-5n71-min-elev",
	"atgm332d-5n71-show-port",
	// AT6558D (V5). The message-enabling recordings were invalidated
	// when message enables started forcing CFG-RATE; the 5N71 is no
	// longer attached, so they were re-recorded on this module.
	"at6558d-binary",
	"at6558d-pvt-out",
	"at6558d-tmode",
	// AT362-AT6668-6T-30 / AT632 (V6 timing)
	"at632-noop",
	"at632-binary",
	"at632-signal",
	"at632-pps",
	"at632-pvt-out",
	"at632-tmode",
	"at632-min-elev",
	"at632-show-port",
	"at632-raw-out",
	"at632-rtcm-out",
	// ATGM332D-AT9880-F8N-76 (V6 dual-band nav)
	"atgm332d-f8n-noop",
	"atgm332d-f8n-binary",
	"atgm332d-f8n-signal",
	"atgm332d-f8n-pps",
	"atgm332d-f8n-pvt-out",
	"atgm332d-f8n-tmode",
	"atgm332d-f8n-min-elev",
	"atgm332d-f8n-show-port",
	"atgm332d-f8n-raw-out",
	"atgm332d-f8n-rtcm-out",
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
