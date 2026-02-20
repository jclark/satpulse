package quectel

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
	"github.com/jclark/satpulse/gps/ptime"
)

// Handler converts PQTM periodic messages to gpsprot messages.
type Handler struct{}

// NewHandler creates a new Quectel PQTM sentence handler.
func NewHandler() *Handler { return &Handler{} }

// HandleSentence parses a PQTM periodic message and returns gpsprot
// messages. It returns (nil, nil, nil) for non-PQTM or unrecognized
// sentences. EOE returns (bundle, nil, nil) to signal end-of-epoch.
func (h *Handler) HandleSentence(
	flags nmeamsg.SentenceSyntaxFlags,
	payload string,
	epoch *nmea.NavEpoch,
) (*gpsprot.MsgBundle, *nmea.NavEpoch, error) {
	if !flags.IsValidProprietaryNMEA() {
		return nil, nil, nil
	}
	msg, err := qtmmsg.ParsePeriodicMsg(payload)
	if err != nil {
		return nil, nil, err
	}
	if msg == nil {
		return nil, nil, nil
	}
	switch m := msg.(type) {
	case *qtmmsg.PVT:
		epoch = nmea.CheckEpoch(epoch, m.Time)
		b := msgBundlePVT(m)
		b.SetPriority(gpsprot.PriVendorLow)
		return b, epoch, nil
	case *qtmmsg.NAV:
		epoch = nmea.CheckEpoch(epoch, m.UTC)
		b := msgBundleNAV(m, epoch)
		b.SetPriority(gpsprot.PriVendorLow)
		return b, epoch, nil
	case *qtmmsg.VEL:
		epoch = nmea.CheckEpoch(epoch, m.Time)
		b := msgBundleVEL(m, epoch)
		b.SetPriority(gpsprot.PriVendorLow)
		return b, epoch, nil
	case *qtmmsg.EPE:
		epoch = nmea.CheckEpoch(epoch, "")
		accEPE(m, epoch)
		return &gpsprot.MsgBundle{}, epoch, nil
	case *qtmmsg.SVINStatus:
		epoch = nmea.CheckEpoch(epoch, "")
		return msgBundleSVIN(m), epoch, nil
	case *qtmmsg.EOE:
		return &gpsprot.MsgBundle{}, nil, nil
	default:
		return nil, nil, nil
	}
}

func msgBundlePVT(m *qtmmsg.PVT) *gpsprot.MsgBundle {
	b := &gpsprot.MsgBundle{}
	if m.FixType >= 2 {
		if utc, ok := parseDateTime(m.Date, m.Time); ok {
			tm := &gpsprot.TimeMsg{
				UTCTime:     &utc,
				Tag:         nmea.Tag,
				NativeMsgID: "PQTMPVT",
			}
			if m.LeapS.IsSet() {
				tm.UTCOffset = m.LeapS.Get() + ptime.TAIMinusGPS
			}
			b.Time = tm
		}
	}
	if m.Lat.IsSet() && m.Lon.IsSet() {
		pos := &gpsprot.PosGeoMsg{
			LatLon: [2]gpsprot.Angle{
				gpsprot.DegreesFromFloat(m.Lat.Get()),
				gpsprot.DegreesFromFloat(m.Lon.Get()),
			},
			Tag:         nmea.Tag,
			NativeMsgID: "PQTMPVT",
		}
		if m.Alt.IsSet() {
			pos.HeightMSL.Set(gpsprot.Meters(m.Alt.Get()))
			if m.Sep.IsSet() {
				pos.Height.Set(gpsprot.Meters(m.Alt.Get() + m.Sep.Get()))
			}
		}
		b.PosGeo = pos
	}
	if m.VelN.IsSet() && m.VelE.IsSet() && m.VelD.IsSet() {
		vel := &gpsprot.VelGeoMsg{
			Tag:         nmea.Tag,
			NativeMsgID: "PQTMPVT",
		}
		vel.VelNED.Set([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(m.VelN.Get()),
			gpsprot.MetersPerSecondFromFloat(m.VelE.Get()),
			gpsprot.MetersPerSecondFromFloat(m.VelD.Get()),
		})
		if m.Spd.IsSet() {
			vel.Speed3D.Set(gpsprot.MetersPerSecondFromFloat(m.Spd.Get()))
		}
		if m.Heading.IsSet() {
			vel.Course.Set(gpsprot.DegreesFromFloat(m.Heading.Get()))
		}
		b.VelGeo = vel
	}
	return b
}

func msgBundleNAV(m *qtmmsg.NAV, epoch *nmea.NavEpoch) *gpsprot.MsgBundle {
	b := &gpsprot.MsgBundle{}
	if m.TimeStatus == 1 {
		if utc, ok := parseDateTime(m.Date, m.UTC); ok {
			tm := &gpsprot.TimeMsg{
				UTCTime:     &utc,
				Tag:         nmea.Tag,
				NativeMsgID: "PQTMNAV",
			}
			if m.WN.IsSet() && m.TOW.IsSet() {
				tm.TAITime = ptime.GPS(int16(m.WN.Get()), time.Duration(m.TOW.Get())*time.Millisecond)
			}
			if m.LeapSec.IsSet() {
				tm.UTCOffset = m.LeapSec.Get() + ptime.TAIMinusGPS
			}
			b.Time = tm
		}
	}
	if m.Lat.IsSet() && m.Lon.IsSet() {
		pos := &gpsprot.PosGeoMsg{
			LatLon: [2]gpsprot.Angle{
				gpsprot.DegreesFromFloat(m.Lat.Get()),
				gpsprot.DegreesFromFloat(m.Lon.Get()),
			},
			Tag:         nmea.Tag,
			NativeMsgID: "PQTMNAV",
		}
		if m.Alt.IsSet() {
			pos.HeightMSL.Set(gpsprot.Meters(m.Alt.Get()))
			if m.Sep.IsSet() {
				pos.Height.Set(gpsprot.Meters(m.Alt.Get() + m.Sep.Get()))
			}
		}
		b.PosGeo = pos
	}
	if m.HVel.IsSet() {
		vel := &gpsprot.VelGeoMsg{
			Tag:         nmea.Tag,
			NativeMsgID: "PQTMNAV",
		}
		vel.GroundSpeed.Set(gpsprot.MetersPerSecondFromFloat(m.HVel.Get()))
		if m.COG.IsSet() {
			vel.Course.Set(gpsprot.DegreesFromFloat(m.COG.Get()))
		}
		b.VelGeo = vel
	}
	if m.LatStd.IsSet() && m.LonStd.IsSet() {
		lat, lon := m.LatStd.Get(), m.LonStd.Get()
		epoch.Acc.Hor.Set(gpsprot.Meters(math.Sqrt(lat*lat + lon*lon)))
	}
	if m.AltStd.IsSet() {
		epoch.Acc.Vert.Set(gpsprot.Meters(m.AltStd.Get()))
	}
	if m.HVelStd.IsSet() {
		epoch.Acc.GroundSpeed.Set(gpsprot.MetersPerSecondFromFloat(m.HVelStd.Get()))
	}
	return b
}

func msgBundleVEL(m *qtmmsg.VEL, epoch *nmea.NavEpoch) *gpsprot.MsgBundle {
	b := &gpsprot.MsgBundle{}
	if m.VelN.IsSet() && m.VelE.IsSet() && m.VelD.IsSet() {
		vel := &gpsprot.VelGeoMsg{
			Tag:         nmea.Tag,
			NativeMsgID: "PQTMVEL",
		}
		vel.VelNED.Set([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(m.VelN.Get()),
			gpsprot.MetersPerSecondFromFloat(m.VelE.Get()),
			gpsprot.MetersPerSecondFromFloat(m.VelD.Get()),
		})
		if m.GrdSpd.IsSet() {
			vel.GroundSpeed.Set(gpsprot.MetersPerSecondFromFloat(m.GrdSpd.Get()))
		}
		if m.Spd.IsSet() {
			vel.Speed3D.Set(gpsprot.MetersPerSecondFromFloat(m.Spd.Get()))
		}
		if m.COG.IsSet() {
			vel.Course.Set(gpsprot.DegreesFromFloat(m.COG.Get()))
		}
		b.VelGeo = vel
	}
	if m.GrdSpdAcc.IsSet() {
		epoch.Acc.GroundSpeed.Set(gpsprot.MetersPerSecondFromFloat(m.GrdSpdAcc.Get()))
	}
	if m.SpdAcc.IsSet() {
		epoch.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(m.SpdAcc.Get()))
	}
	if m.HeadingAcc.IsSet() {
		epoch.Acc.Course.Set(gpsprot.DegreesFromFloat(m.HeadingAcc.Get()))
	}
	return b
}

func accEPE(m *qtmmsg.EPE, epoch *nmea.NavEpoch) {
	if m.EPE2D.IsSet() {
		epoch.Acc.Hor.Set(gpsprot.Meters(m.EPE2D.Get()))
	}
	if m.EPE3D.IsSet() {
		epoch.Acc.Pos.Set(gpsprot.Meters(m.EPE3D.Get()))
	}
	if m.EPEDown.IsSet() {
		epoch.Acc.Vert.Set(gpsprot.Meters(m.EPEDown.Get()))
	}
}

func msgBundleSVIN(m *qtmmsg.SVINStatus) *gpsprot.MsgBundle {
	sv := &gpsprot.SurveyMsg{
		ObsCount:   m.Obs,
		Valid:      m.Valid == 2,
		InProgress: m.Valid == 1,
	}
	if m.MeanX.IsSet() && m.MeanY.IsSet() && m.MeanZ.IsSet() {
		sv.Position = gpsprot.Point3D{
			gpsprot.Meters(m.MeanX.Get()),
			gpsprot.Meters(m.MeanY.Get()),
			gpsprot.Meters(m.MeanZ.Get()),
		}
	}
	if m.MeanAcc.IsSet() {
		sv.Accuracy = gpsprot.Meters(m.MeanAcc.Get())
	}
	return &gpsprot.MsgBundle{Survey: sv}
}

func parseDateTime(date, tod string) (ptime.UTCTime, bool) {
	if len(date) != 8 {
		return ptime.UTCTime{}, false
	}
	var y uint16
	var mo, d uint8
	if n, _ := fmt.Sscanf(date, "%04d%02d%02d", &y, &mo, &d); n != 3 {
		return ptime.UTCTime{}, false
	}
	h, mi, s, ns, ok := parseTimeOfDay(tod)
	if !ok {
		return ptime.UTCTime{}, false
	}
	return ptime.UTC(y, mo, d, h, mi, s, ns), true
}

func parseTimeOfDay(s string) (hour, min, sec uint8, nanos int32, ok bool) {
	fixed, frac, _ := strings.Cut(s, ".")
	if len(fixed) != 6 {
		return
	}
	if n, _ := fmt.Sscanf(fixed, "%02d%02d%02d", &hour, &min, &sec); n != 3 {
		return
	}
	if frac != "" {
		pad := 9 - len(frac)
		if pad < 0 {
			return
		}
		frac += strings.Repeat("0", pad)
		fmt.Sscanf(frac, "%d", &nanos)
	}
	ok = true
	return
}
