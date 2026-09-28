package septentrio

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/ptime"
)

func pvtGeoBlock(ts sbfbin.TimeStamp, sys sbfbin.TimeSystem, rxClkBias float64) *sbfbin.Block {
	var m sbfbin.PVTGeodetic
	m.Mode = sbfbin.ModeStandalone
	m.TimeSystem = sys
	m.RxClkBias = rxClkBias
	return &sbfbin.Block{TimeStamp: ts, Params: &m}
}

// TestTimePVTAlwaysGPSHeader confirms the header timestamp is on the GPS
// convention regardless of TimeSystem: BeiDou and GPS must yield identical
// TAITime (only the GNSS label differs), never a whole-second BeiDou offset.
func TestTimePVTAlwaysGPSHeader(t *testing.T) {
	bias := -0.3957389614621168
	ts := sbfbin.TimeStamp{TOW: 465357000, WNc: 2425}
	bg := pvtGeoBlock(ts, sbfbin.TimeSystemGPS, bias)
	bb := pvtGeoBlock(ts, sbfbin.TimeSystemBeiDou, bias)
	gps := timePVTGeodetic(bg, bg.Params.(*sbfbin.PVTGeodetic))
	bds := timePVTGeodetic(bb, bb.Params.(*sbfbin.PVTGeodetic))
	if gps == nil || bds == nil {
		t.Fatal("timePVTGeodetic returned nil")
	}
	if gps.TAITime != bds.TAITime {
		t.Errorf("TAITime differs by TimeSystem: GPS=%v BDS=%v", gps.TAITime, bds.TAITime)
	}
	want := ptime.GPS(int16(ts.WNc), time.Duration(ts.TOW)*time.Millisecond).Add(-time.Duration(bias * float64(time.Millisecond)))
	if gps.TAITime != want {
		t.Errorf("TAITime = %v, want %v", gps.TAITime, want)
	}
	if gps.GNSS != gpsprot.GPS || bds.GNSS != gpsprot.BDS {
		t.Errorf("GNSS labels wrong: GPS=%v BDS=%v", gps.GNSS, bds.GNSS)
	}
	if gps.Ref != gpsprot.NavSolution {
		t.Errorf("Ref = %v, want NavSolution", gps.Ref)
	}
}

// TestTimePVTGating suppresses TimeMsg on DNU RxClkBias and on unmapped time
// systems (GLONASS/unassigned).
func TestTimePVTGating(t *testing.T) {
	ts := sbfbin.TimeStamp{TOW: 1000, WNc: 2425}
	if tm := timePVTGeodetic(pvtGeoBlock(ts, sbfbin.TimeSystemGPS, sbfbin.PVTClockDNU),
		pvtGeoBlock(ts, sbfbin.TimeSystemGPS, sbfbin.PVTClockDNU).Params.(*sbfbin.PVTGeodetic)); tm != nil {
		t.Error("DNU RxClkBias should suppress TimeMsg")
	}
	for _, sys := range []sbfbin.TimeSystem{sbfbin.TimeSystemGLONASS, sbfbin.TimeSystemReserved2, sbfbin.TimeSystemDNU} {
		b := pvtGeoBlock(ts, sys, 0)
		if tm := timePVTGeodetic(b, b.Params.(*sbfbin.PVTGeodetic)); tm != nil {
			t.Errorf("TimeSystem %d should suppress TimeMsg", sys)
		}
	}
}

// TestTimeReceiverTime checks UTC construction and the DeltaLS -> UTCOffset
// mapping against a real capture (DeltaLS 18 -> UTCOffset 37).
func TestTimeReceiverTime(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "ReceiverTime")
	tm := timeReceiverTime(b, b.Params.(*sbfbin.ReceiverTime))
	if tm == nil {
		t.Fatal("timeReceiverTime returned nil")
	}
	if tm.Ref != gpsprot.NavSolution || tm.NativeMsgID != "ReceiverTime" {
		t.Errorf("Ref/NativeMsgID = %v/%q", tm.Ref, tm.NativeMsgID)
	}
	if tm.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want 37", tm.UTCOffset)
	}
	if !tm.UTCTime.IsSet() {
		t.Fatal("UTCTime unset")
	}
	want := ptime.UTC(2026, 7, 3, 9, 15, 40, 0)
	if tm.UTCTime.Get() != want {
		t.Errorf("UTCTime = %v, want %v", tm.UTCTime.Get(), want)
	}
	if tm.TAITime != ptime.GPS(int16(b.WNc), time.Duration(b.TOW)*time.Millisecond) {
		t.Error("TAITime mismatch")
	}
}

// TestTimeReceiverTimeDNU leaves UTC and UTCOffset unset when the components
// are at their -128 do-not-use sentinel.
func TestTimeReceiverTimeDNU(t *testing.T) {
	utc := []int8{26, 7, 3, 9, 15, 40}
	for i := range utc {
		v := utc[i]
		utc[i] = sbfbin.UTCComponentDNU
		m := sbfbin.ReceiverTime{
			UTCYear: utc[0], UTCMonth: utc[1], UTCDay: utc[2],
			UTCHour: utc[3], UTCMin: utc[4], UTCSec: utc[5],
			DeltaLS: sbfbin.UTCComponentDNU,
		}
		b := &sbfbin.Block{TimeStamp: sbfbin.TimeStamp{TOW: 1000, WNc: 2425}, Params: &m}
		tm := timeReceiverTime(b, &m)
		if tm == nil {
			t.Fatal("nil")
		}
		if tm.UTCTime.IsSet() {
			t.Errorf("UTCTime set with UTC component %d at DNU", i)
		}
		if tm.UTCOffset != 0 {
			t.Errorf("UTCOffset = %d, want 0 on DNU", tm.UTCOffset)
		}
		utc[i] = v
	}
}

// TestTimeXPPSOffset checks the required sign flip and PostPulse ref.
func TestTimeXPPSOffset(t *testing.T) {
	b := findBlock(t, "daemon.jsonl", "xPPSOffset")
	m := b.Params.(*sbfbin.XPPSOffset)
	tm := timeXPPSOffset(b, m)
	if tm == nil {
		t.Fatal("nil")
	}
	if tm.Ref != gpsprot.PostPulse {
		t.Errorf("Ref = %v, want PostPulse", tm.Ref)
	}
	if !tm.PulseOffset.IsSet() {
		t.Fatal("PulseOffset unset")
	}
	if got, want := tm.PulseOffset.Get(), -float64(m.Offset); got != want {
		t.Errorf("PulseOffset = %v, want %v (sign flip)", got, want)
	}
	if tm.GNSS != gpsprot.GPS { // Timescale==1
		t.Errorf("GNSS = %v, want GPS", tm.GNSS)
	}
}
