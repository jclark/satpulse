package quectel

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
	"github.com/jclark/satpulse/gps/ptime"
)

// Handler converts PQTM periodic messages to gpsprot messages.
type Handler struct{}

// NewHandler creates a new Quectel PQTM sentence handler.
func NewHandler() *Handler { return &Handler{} }

// HandleSentence parses a PQTM periodic message and returns gpsprot
// messages. It returns ErrNotHandled for non-PQTM or unrecognized
// sentences. EOE returns (nil, nil, nil) to signal end-of-epoch.
func (h *Handler) HandleSentence(
	flags nmeamsg.SentenceSyntaxFlags,
	payload string,
	epoch *nmea.NavEpoch,
) ([]gpsprot.Msg, *nmea.NavEpoch, error) {
	if !flags.IsValidProprietaryNMEA() {
		return nil, nil, gpsprot.ErrNotHandled
	}
	msg, err := qtmmsg.ParsePeriodicMsg(payload)
	if err != nil {
		return nil, nil, err
	}
	if msg == nil {
		return nil, nil, gpsprot.ErrNotHandled
	}
	switch m := msg.(type) {
	case *qtmmsg.PVT:
		epoch = nmea.CheckEpoch(epoch, m.Time)
		msgs := msgsPVT(m, epoch)
		gpsprot.SetMsgsPriority(msgs, gpsprot.PriVendorLow)
		return msgs, epoch, nil
	case *qtmmsg.NAV:
		epoch = nmea.CheckEpoch(epoch, m.UTC)
		msgs := msgsNAV(m, epoch)
		gpsprot.SetMsgsPriority(msgs, gpsprot.PriVendorLow)
		return msgs, epoch, nil
	case *qtmmsg.VEL:
		epoch = nmea.CheckEpoch(epoch, m.Time)
		msgs := msgsVEL(m, epoch)
		gpsprot.SetMsgsPriority(msgs, gpsprot.PriVendorLow)
		return msgs, epoch, nil
	case *qtmmsg.EPE:
		epoch = nmea.CheckEpoch(epoch, "")
		accEPE(m, epoch)
		return nil, epoch, nil
	case *qtmmsg.SVINStatus:
		epoch = nmea.CheckEpoch(epoch, "")
		return msgsSVIN(m), epoch, nil
	case *qtmmsg.DOP:
		epoch = nmea.CheckEpoch(epoch, "")
		dopQuality(m, epoch)
		return nil, epoch, nil
	case *qtmmsg.EOE:
		return nil, nil, nil
	default:
		return nil, nil, gpsprot.ErrNotHandled
	}
}

func msgsPVT(m *qtmmsg.PVT, epoch *nmea.NavEpoch) []gpsprot.Msg {
	var msgs []gpsprot.Msg
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
			msgs = append(msgs, tm)
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
		msgs = append(msgs, pos)
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
		msgs = append(msgs, vel)
	}
	// PVT quality is weaker than NAV (no correction info). Only
	// populate when NAV hasn't already set quality for this epoch.
	if epoch.FixLevel == 0 {
		switch m.FixType {
		case 0:
			epoch.FixLevel = gpsprot.FixLevelNone
		case 2:
			epoch.FixLevel = gpsprot.FixLevelCode
			epoch.SolutionDim = gpsprot.SolutionDim2D
		case 3:
			epoch.FixLevel = gpsprot.FixLevelCode
			epoch.SolutionDim = gpsprot.SolutionDim3D
		}
	}
	epoch.NumSVUsed = opt.Make(uint16(m.NumSV))
	if m.HDOP.IsSet() {
		epoch.DOP.Hor = opt.Make(m.HDOP.Get())
	}
	if m.PDOP.IsSet() {
		epoch.DOP.Pos = opt.Make(m.PDOP.Get())
	}
	return msgs
}

func msgsNAV(m *qtmmsg.NAV, epoch *nmea.NavEpoch) []gpsprot.Msg {
	var msgs []gpsprot.Msg
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
			msgs = append(msgs, tm)
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
		msgs = append(msgs, pos)
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
		msgs = append(msgs, vel)
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
	if m.SolType.IsSet() {
		if fl, fd, corr, ok := navSolQuality(m.SolType.Get()); ok {
			epoch.FixLevel = fl
			epoch.SolutionDim = fd
			epoch.Correction = corr
		}
	}
	if m.SatUsed.IsSet() {
		epoch.NumSVUsed = opt.Make(uint16(m.SatUsed.Get()))
	}
	if m.SatView.IsSet() {
		epoch.NumSVInView = opt.Make(uint16(m.SatView.Get()))
	}
	if m.DiffAge.IsSet() {
		epoch.DiffAge = opt.Make(gpsprot.Seconds(m.DiffAge.Get()))
	}
	if m.DiffID.IsSet() && m.DiffID.Get() <= 4095 {
		epoch.RTCMRefBaseID = opt.Make(m.DiffID.Get())
	}
	return msgs
}

func msgsVEL(m *qtmmsg.VEL, epoch *nmea.NavEpoch) []gpsprot.Msg {
	var msgs []gpsprot.Msg
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
		msgs = append(msgs, vel)
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
	return msgs
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

// navSolQuality maps PQTMNAV SolType to NavEpochMsg quality fields.
// Returns false for unrecognized SolType values so the caller can
// preserve existing epoch state rather than zeroing it.
func navSolQuality(solType uint8) (gpsprot.FixLevel, gpsprot.SolutionDim, gpsprot.CorrKind, bool) {
	switch solType {
	case 0:
		return gpsprot.FixLevelNone, 0, 0, true
	case 1:
		return gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, true
	case 2:
		return gpsprot.FixLevelCodeCorrected, gpsprot.SolutionDim3D,
			gpsprot.CorrSBAS | gpsprot.CorrSSR | gpsprot.CorrUsed, true
	case 5:
		return gpsprot.FixLevelCodeCorrected, gpsprot.SolutionDim3D,
			gpsprot.CorrOSR | gpsprot.CorrUsed, true
	case 8:
		return gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D,
			gpsprot.CorrOSR | gpsprot.CorrUsed, true
	case 12:
		return gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D,
			gpsprot.CorrOSR | gpsprot.CorrUsed, true
	default:
		return 0, 0, 0, false
	}
}

func dopQuality(m *qtmmsg.DOP, epoch *nmea.NavEpoch) {
	if m.GDOP.IsSet() {
		epoch.DOP.Geom = m.GDOP
	}
	if m.PDOP.IsSet() {
		epoch.DOP.Pos = m.PDOP
	}
	if m.TDOP.IsSet() {
		epoch.DOP.Time = m.TDOP
	}
	if m.VDOP.IsSet() {
		epoch.DOP.Vert = m.VDOP
	}
	if m.HDOP.IsSet() {
		epoch.DOP.Hor = m.HDOP
	}
}

func msgsSVIN(m *qtmmsg.SVINStatus) []gpsprot.Msg {
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
	return []gpsprot.Msg{sv}
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
