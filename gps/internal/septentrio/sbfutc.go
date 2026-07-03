package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// leapSecond builds a LeapSecondMsg from the re-broadcast leap-second event
// fields common to GPSUtc/GALUtc/BDSUtc. conv resolves the boundary date on
// the GNSS's own convention; now is the header instant (always GPS). Dispatch
// is suppressed when no valid, unambiguous boundary date is found.
func leapSecond(now ptime.Time, g ptime.GNSSLeapSecond, gnss gpsprot.GNSS,
	conv func(ptime.GNSSLeapSecond, ptime.Time) (ptime.LeapSecond, error)) *gpsprot.LeapSecondMsg {
	ls, err := conv(g, now)
	if err != nil {
		return nil
	}
	return &gpsprot.LeapSecondMsg{LeapSecond: ls, GNSS: gnss}
}

func leapGPSUtc(b *sbfbin.Block, m *sbfbin.GPSUtc) *gpsprot.LeapSecondMsg {
	g := ptime.GNSSLeapSecond{WNLSF: m.WN_LSF, DN: m.DN, DeltaLS: m.DEL_t_LS, DeltaLSF: m.DEL_t_LSF}
	return leapSecond(gpsHeaderTime(b.TimeStamp), g, gpsprot.GPS, ptime.GPSLeapSecond)
}

func leapGALUtc(b *sbfbin.Block, m *sbfbin.GALUtc) *gpsprot.LeapSecondMsg {
	g := ptime.GNSSLeapSecond{WNLSF: m.WN_LSF, DN: m.DN, DeltaLS: m.DEL_t_LS, DeltaLSF: m.DEL_t_LSF}
	return leapSecond(gpsHeaderTime(b.TimeStamp), g, gpsprot.GAL, ptime.GalileoLeapSecond)
}

func leapBDSUtc(b *sbfbin.Block, m *sbfbin.BDSUtc) *gpsprot.LeapSecondMsg {
	g := ptime.GNSSLeapSecond{WNLSF: m.WN_LSF, DN: m.DN, DeltaLS: m.DEL_t_LS, DeltaLSF: m.DEL_t_LSF}
	return leapSecond(gpsHeaderTime(b.TimeStamp), g, gpsprot.BDS, ptime.BeiDouLeapSecond)
}
