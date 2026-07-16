package septentrio

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// gpsHeaderTime converts a block-header (TOW, WNc) pair to a ptime.Time.
// Every SBF block header is on the GPS time convention regardless of the
// constellation the block otherwise describes (guide sec 4.1.3).
func gpsHeaderTime(ts sbfbin.TimeStamp) ptime.Time {
	return ptime.GPS(int16(ts.WNc), time.Duration(ts.TOW)*time.Millisecond)
}

// headerTimeValid reports whether the block-header timestamp is usable
// (neither TOW nor WNc at its do-not-use sentinel).
func headerTimeValid(ts sbfbin.TimeStamp) bool {
	return ts.TOW != sbfbin.TOWDNU && ts.WNc != sbfbin.WNcDNU
}

// timeSystemGNSS maps a PVT TimeSystem to a gpsprot.GNSS, reporting false for
// time systems with no GNSS-time mapping (unassigned, GLONASS, Fugro, DNU).
func timeSystemGNSS(ts sbfbin.TimeSystem) (gpsprot.GNSS, bool) {
	switch ts {
	case sbfbin.TimeSystemGPS:
		return gpsprot.GPS, true
	case sbfbin.TimeSystemGalileo:
		return gpsprot.GAL, true
	case sbfbin.TimeSystemBeiDou:
		return gpsprot.BDS, true
	case sbfbin.TimeSystemQZSS:
		return gpsprot.QZSS, true
	}
	return 0, false
}

// timePVT builds the sub-millisecond TAITime source shared by PVTGeodetic and
// PVTCartesian. The header (TOW, WNc) is always on the GPS convention; the
// RxClkBias correction and the GNSS label come from TimeSystem.
func timePVT(ts sbfbin.TimeStamp, rxClkBias float64, timeSystem sbfbin.TimeSystem, nativeMsgID string) *gpsprot.TimeMsg {
	if !headerTimeValid(ts) || rxClkBias == sbfbin.PVTClockDNU {
		return nil
	}
	gnss, ok := timeSystemGNSS(timeSystem)
	if !ok {
		return nil
	}
	tai := gpsHeaderTime(ts).Add(-time.Duration(rxClkBias * float64(time.Millisecond)))
	return &gpsprot.TimeMsg{
		TAITime:     tai,
		GNSS:        gnss,
		Ref:         gpsprot.NavSolution,
		NativeMsgID: nativeMsgID,
	}
}

func timePVTGeodetic(b *sbfbin.Block, m *sbfbin.PVTGeodetic) *gpsprot.TimeMsg {
	return timePVT(b.TimeStamp, m.RxClkBias, m.TimeSystem, "PVTGeodetic")
}

func timePVTCartesian(b *sbfbin.Block, m *sbfbin.PVTCartesian) *gpsprot.TimeMsg {
	return timePVT(b.TimeStamp, m.RxClkBias, m.TimeSystem, "PVTCartesian")
}

// timeReceiverTime builds a TimeMsg from a ReceiverTime block: a
// whole-millisecond TAITime plus native UTC time and UTC offset.
func timeReceiverTime(b *sbfbin.Block, m *sbfbin.ReceiverTime) *gpsprot.TimeMsg {
	if !headerTimeValid(b.TimeStamp) {
		return nil
	}
	tm := &gpsprot.TimeMsg{
		TAITime:     gpsHeaderTime(b.TimeStamp),
		Ref:         gpsprot.NavSolution,
		NativeMsgID: "ReceiverTime",
	}
	if m.UTCYear != sbfbin.UTCComponentDNU {
		tm.UTCTime.Set(ptime.UTC(uint16(2000+int(m.UTCYear)),
			uint8(m.UTCMonth), uint8(m.UTCDay),
			uint8(m.UTCHour), uint8(m.UTCMin), uint8(m.UTCSec), 0))
	}
	if m.DeltaLS != sbfbin.UTCComponentDNU {
		tm.UTCOffset = uint8(int(m.DeltaLS) + ptime.TAIMinusGPS)
	}
	return tm
}

// ppsTimescaleGNSS maps an xPPSOffset Timescale to a gpsprot.GNSS, reporting
// false for timescales with no GNSS label (UTC, Receiver, Fugro).
func ppsTimescaleGNSS(ts sbfbin.PPSTimescale) (gpsprot.GNSS, bool) {
	switch ts {
	case sbfbin.PPSTimescaleGPS:
		return gpsprot.GPS, true
	case sbfbin.PPSTimescaleGLONASS:
		return gpsprot.GLO, true
	case sbfbin.PPSTimescaleGalileo:
		return gpsprot.GAL, true
	case sbfbin.PPSTimescaleBeiDou:
		return gpsprot.BDS, true
	}
	return 0, false
}

// timeXPPSOffset builds the PostPulse TimeMsg from an xPPSOffset block. Offset
// is in nanoseconds and negated: the guide defines it negative when the pulse
// is early, whereas TimeMsg.PulseOffset is positive-early (trueTime =
// pulseTime + PulseOffset).
func timeXPPSOffset(b *sbfbin.Block, m *sbfbin.XPPSOffset) *gpsprot.TimeMsg {
	if !headerTimeValid(b.TimeStamp) {
		return nil
	}
	tm := &gpsprot.TimeMsg{
		TAITime:     gpsHeaderTime(b.TimeStamp),
		PulseOffset: opt.Make(-float64(m.Offset)),
		Ref:         gpsprot.PostPulse,
		NativeMsgID: "xPPSOffset",
	}
	if gnss, ok := ppsTimescaleGNSS(m.TimeScale); ok {
		tm.GNSS = gnss
	}
	return tm
}
