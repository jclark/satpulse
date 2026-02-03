package asbin

import (
	"math"
	"testing"
)

const degToRad = math.Pi / 180

func TestCfgElev(t *testing.T) {
	// Captured from TAU1201: trkMask=1deg, naviMask=5deg
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d9060b080035fa8e3cc2b8b23d7be0",
		wantID: CfgElevID,
		wantMsg: &CfgElev{
			TrkMask:  float32(1 * degToRad),
			NaviMask: float32(5 * degToRad),
		},
	}})
}

func TestCfgNmeaVer(t *testing.T) {
	// Captured from TAU1201: NMEA V4.10
	runTests(t, []testCase{{
		name:   "captured",
		packet: "f1d906430100034d30",
		wantID: CfgNmeaVerID,
		wantMsg: &CfgNmeaVer{
			Version: CfgNmeaVerV410,
		},
	}})
}
