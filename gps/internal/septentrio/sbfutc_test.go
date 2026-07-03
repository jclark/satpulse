package septentrio

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// TestLeapGPSUtc builds a LeapSecondMsg from a real GPSUtc block: DeltaLS 18
// yields the current 37 s TAI-UTC offset, GNSS is GPS.
func TestLeapGPSUtc(t *testing.T) {
	b := findBlock(t, "nav-utc.jsonl", "GPSUtc")
	ls := leapGPSUtc(b, b.Params.(*sbfbin.GPSUtc))
	if ls == nil {
		t.Fatal("leapGPSUtc returned nil")
	}
	if ls.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want GPS", ls.GNSS)
	}
	if ls.UTCOffAfter != 37 {
		t.Errorf("UTCOffAfter = %d, want 37", ls.UTCOffAfter)
	}
}

// TestLeapGALUtc does the same for a real GALUtc block (GNSS GAL).
func TestLeapGALUtc(t *testing.T) {
	b := findBlock(t, "nav-utc.jsonl", "GALUtc")
	ls := leapGALUtc(b, b.Params.(*sbfbin.GALUtc))
	if ls == nil {
		t.Fatal("leapGALUtc returned nil")
	}
	if ls.GNSS != gpsprot.GAL {
		t.Errorf("GNSS = %v, want GAL", ls.GNSS)
	}
	if ls.UTCOffAfter != 37 {
		t.Errorf("UTCOffAfter = %d, want 37", ls.UTCOffAfter)
	}
}

// TestLeapBDSUtc does the same for a real BDSUtc block; BeiDou uses 0-based DN
// and a 33 s epoch offset, yielding the same 37 s current offset (DeltaLS 4).
func TestLeapBDSUtc(t *testing.T) {
	b := findBlock(t, "nav-utc.jsonl", "BDSUtc")
	ls := leapBDSUtc(b, b.Params.(*sbfbin.BDSUtc))
	if ls == nil {
		t.Fatal("leapBDSUtc returned nil")
	}
	if ls.GNSS != gpsprot.BDS {
		t.Errorf("GNSS = %v, want BDS", ls.GNSS)
	}
	if ls.UTCOffAfter != 37 {
		t.Errorf("UTCOffAfter = %d, want 37", ls.UTCOffAfter)
	}
}
