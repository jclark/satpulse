package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

var casicReplayFiles = []string{
	// ATGM332D-5N71 (V5)
	"atgm332d-5n71-min-elev",
	// ATGM332D-5N31 (V5), effectively identical to the earlier 5N71.
	// Recordings invalidated by configurator changes (message enables
	// forcing CFG-RATE, then CFG serialization, then dropping the V5
	// signal verify readback) were re-recorded on this module; the 5N71
	// is no longer attached.
	"atgm332d-5n31-pps",
	"atgm332d-5n31-noop",
	"atgm332d-5n31-signal",
	"atgm332d-5n31-binary",
	"atgm332d-5n31-pvt-out",
	"atgm332d-5n31-tmode",
	"atgm332d-5n31-show-port",
	"atgm332d-5n31-speed",
	"atgm332d-5n31-reload",
	"atgm332d-5n31-save",
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
	"at632-speed",
	"at632-save",
	// ATGM332D-AT9880-F8N-76 (V6 dual-band nav)
	"atgm332d-f8n-noop",
	"atgm332d-f8n-binary",
	"atgm332d-f8n-signal",
	"atgm332d-f8n-pps",
	"atgm332d-f8n-pvt-out",
	"atgm332d-f8n-tmode",
	"atgm332d-f8n-min-elev",
	"atgm332d-f8n-show-port",
}

func TestReplayCASIC(t *testing.T) {
	for _, filename := range casicReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, "zhongke/"+filename, casicPacketsEqual)
		})
	}
}

// casicPacketsEqual returns (equal, updatable). A mismatch between
// two CASIC packets with the same class+id is a content change, safe
// to auto-update in golden files; a different id, or a packet that is
// not CASIC binary (a PCAS text query), is structural.
func casicPacketsEqual(t *testing.T, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()
	if actualStr == expectedStr {
		return true, false
	}
	actualID, actualOK := casicMsgID(actualStr)
	expectedID, expectedOK := casicMsgID(expectedStr)
	if !actualOK || !expectedOK || actualID != expectedID {
		t.Errorf("packets differ: actual %q, expected %q", actualStr, expectedStr)
		return false, false
	}
	if am, err := casbin.ParseMsg(actualStr); err == nil {
		if em, err := casbin.ParseMsg(expectedStr); err == nil {
			t.Errorf("%v: packet content differs: actual %+v, expected %+v", actualID, am, em)
			return false, true
		}
	}
	t.Errorf("%v: packet content differs: actual %q, expected %q", actualID, actualStr, expectedStr)
	return false, true
}

// casicMsgID extracts the class+id of a CASIC binary packet; false if
// the packet is not CASIC binary.
func casicMsgID(s string) (casbin.MsgID, bool) {
	if len(s) < casbin.PacketMinLen || s[0] != 0xBA || s[1] != 0xCE {
		return 0, false
	}
	return casbin.MakeMsgID(s[4], s[5]), true
}
