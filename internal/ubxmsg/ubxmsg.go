package ubxmsg

import (
	"math"
	"time"

	"github.com/jclark/gps2phc/internal/gpsmsg"
	"github.com/jclark/gps2phc/internal/ptime"
	"github.com/jclark/gps2phc/internal/ubx"
)

type Message struct {
	um ubx.Msg
}

func Parse(frame string) (*Message, error) {
	um, err := ubx.ParseMsg(frame)
	if err != nil {
		return nil, err
	}
	return &Message{um}, nil
}

func (m *Message) UBX() ubx.Msg {
	return m.um
}

func (m *Message) Time() *gpsmsg.Time {
	switch u := m.um.(type) {
	case *ubx.NavTimeGPS:
		return timeNavTimeGPS(u)
	case *ubx.NavTimeUTC:
		return timeNavTimeUTC(u)
	case *ubx.TimTP:
		return timeTimTP(u)
	}
	return nil
}

func timeNavTimeGPS(m *ubx.NavTimeGPS) *gpsmsg.Time {
	t := gpsmsg.Time{}
	t.TAITime = ptime.GPS(m.Week, m.ITOW, m.FTOW)
	if (m.Valid & ubx.NavTimeGPSLeapSValid) != 0 {
		t.TAIMinusUTC = m.LeapS + ptime.TAIMinusGPS
	}
	t.Accuracy = time.Duration(m.TAcc)
	g := gpsmsg.GPS
	t.GNSS = &g
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func timeNavTimeUTC(m *ubx.NavTimeUTC) *gpsmsg.Time {
	if m.Valid&ubx.NavTimeUTCValidUTC == 0 {
		return nil
	}
	t := gpsmsg.Time{}
	u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, m.Nano)
	t.UTCTime = &u
	t.Accuracy = time.Duration(m.TAcc)
	t.GNSS = utcStandardToGNSS(m.Valid.UTCStandard())
	t.NavEpoch = iTOWEpoch(m.ITOW)
	return &t
}

func iTOWEpoch(iTOW uint32) uint32 {
	return iTOW + 1
}

func utcStandardToGNSS(u ubx.UTCStandard) *gpsmsg.MajorGNSS {
	g := gpsmsg.GPS
	switch u {
	case ubx.UTCStandardUSNO:
		g = gpsmsg.GPS
	case ubx.UTCStandardSU:
		g = gpsmsg.GLONASS
	case ubx.UTCStandardNTSC:
		g = gpsmsg.BeiDou
	case ubx.UTCStandardEU:
		g = gpsmsg.Galileo
	default:
		return nil
	}
	return &g
}

func timeTimTP(m *ubx.TimTP) *gpsmsg.Time {
	t := gpsmsg.Time{PrecedesPulse: true}
	t.TAITime = ptime.GPS(int16(m.Week), m.TOWMS, scaledMSToNS(m.TOWSubMS))
	t.PulseOffset = ptime.Picoseconds(m.QErr)
	if m.Flags&ubx.TimTPTimeBase == ubx.TimTPTimeBaseUTC {
		t.GNSS = utcStandardToGNSS(m.RefInfo.UTCStandard())
	} else {
		var g gpsmsg.MajorGNSS
		t.GNSS = &g
		switch m.RefInfo & ubx.TimTPTimeRefGNSS {
		case ubx.TimTPTimeRefGPS:
			g = gpsmsg.GPS
		case ubx.TimTPTimeRefGLONASS:
			g = gpsmsg.GLONASS
		case ubx.TimTPTimeRefBeiDou:
			g = gpsmsg.BeiDou
		case ubx.TimTPTimeRefGalileo:
			g = gpsmsg.Galileo
		default:
			t.GNSS = nil
		}
	}
	return &t
}

// XXX need unit test for this
func scaledMSToNS(ms uint32) int32 {
	return int32(math.Round(math.Ldexp(float64(ms), -32) * 1e6))
}
