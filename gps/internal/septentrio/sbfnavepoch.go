package septentrio

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// pvtCommon holds the solution-quality fields shared byte-for-byte between
// PVTGeodetic and PVTCartesian, so one accumulator serves both.
type pvtCommon struct {
	Mode        sbfbin.Mode
	NrSV        uint8
	WACorrInfo  sbfbin.WACorrInfo
	ReferenceID uint16
	MeanCorrAge uint16
	SignalInfo  uint32
	HAccuracy   uint16
	VAccuracy   uint16
}

func pvtGeodeticCommon(m *sbfbin.PVTGeodetic) pvtCommon {
	return pvtCommon{
		Mode: m.Mode, NrSV: m.NrSV, WACorrInfo: m.WACorrInfo,
		ReferenceID: m.ReferenceID, MeanCorrAge: m.MeanCorrAge,
		SignalInfo: m.SignalInfo, HAccuracy: m.HAccuracy, VAccuracy: m.VAccuracy,
	}
}

func pvtCartesianCommon(m *sbfbin.PVTCartesian) pvtCommon {
	return pvtCommon{
		Mode: m.Mode, NrSV: m.NrSV, WACorrInfo: m.WACorrInfo,
		ReferenceID: m.ReferenceID, MeanCorrAge: m.MeanCorrAge,
		SignalInfo: m.SignalInfo, HAccuracy: m.HAccuracy, VAccuracy: m.VAccuracy,
	}
}

// qualityPVT accumulates the PVT-family solution-quality fields into the epoch
// message. It is idempotent, so calling it for both PVTGeodetic and
// PVTCartesian in one epoch is safe. baseID is the last-seen BaseStation ID;
// baseRTCM says whether that BaseStation came from an RTCM correction source.
func qualityPVT(ne *gpsprot.NavEpochMsg, c pvtCommon, baseID opt.Val[uint16], baseRTCM bool) {
	ne.FixLevel, ne.SolutionDim = modeFixLevel(c.Mode)
	ne.Correction |= modeCorrection(c)
	if c.HAccuracy != sbfbin.PVTAccuracyDNU {
		ne.Acc.Hor.Set(accuracyLength(c.HAccuracy))
	}
	if c.VAccuracy != sbfbin.PVTAccuracyDNU {
		ne.Acc.Vert.Set(accuracyLength(c.VAccuracy))
	}
	if c.MeanCorrAge != sbfbin.PVTMeanCorrAgeDNU {
		ne.DiffAge = opt.Make(gpsprot.Duration(int64(c.MeanCorrAge) * int64(10*time.Millisecond)))
	}
	if c.NrSV != 255 {
		ne.NumSVUsed.Set(uint16(c.NrSV))
	}
	if rtcmRefBaseCurrent(c) && baseRTCM && baseID.IsSet() {
		ne.RTCMRefBaseID.Set(baseID.Get())
	} else if rtcmRefBaseCurrent(c) && c.ReferenceID != sbfbin.PVTReferenceIDDNU && c.ReferenceID != sbfbin.PVTReferenceIDMulti {
		ne.RTCMRefBaseID.Set(c.ReferenceID)
	}
	for i := uint(0); i < 32; i++ {
		if c.SignalInfo&(1<<i) == 0 {
			continue
		}
		if e, ok := sbfSignalNumber(uint8(i)); ok {
			ne.GNSSUsed |= gpsprot.GNSSSetOf(e.gnss)
			ne.BandsUsed |= e.band
		}
	}
}

func rtcmRefBaseCurrent(c pvtCommon) bool {
	bt := c.WACorrInfo.BaseType()
	return (bt == sbfbin.WACorrBasePhysical || bt == sbfbin.WACorrBaseVirtual) && !c.Mode.IsSBAS()
}

// modeFixLevel derives FixLevel and SolutionDim from the PVT Mode field.
func modeFixLevel(mode sbfbin.Mode) (gpsprot.FixLevel, gpsprot.SolutionDim) {
	var fix gpsprot.FixLevel
	switch mode.Fix() {
	case sbfbin.ModeNoPVT:
		fix = gpsprot.FixLevelNone
	case sbfbin.ModeStandalone, sbfbin.ModeDifferential, sbfbin.ModeSBAS:
		fix = gpsprot.FixLevelCode
	case sbfbin.ModeFixedLocation:
		fix = gpsprot.FixLevelNotMeasured
	case sbfbin.ModeRTKFixed, sbfbin.ModeMovingBaseRTKFixed:
		fix = gpsprot.FixLevelCarrierFixed
	case sbfbin.ModeRTKFloat, sbfbin.ModeMovingBaseRTKFloat:
		fix = gpsprot.FixLevelCarrierFloat
	case sbfbin.ModePPP:
		// SBF cannot distinguish converging vs converged PPP; stay conservative.
		fix = gpsprot.FixLevelCarrierFloat
	default:
		fix = gpsprot.FixLevelNone
	}
	if fix < gpsprot.FixLevelCode {
		return fix, 0
	}
	if mode.Is2D() {
		return fix, gpsprot.SolutionDim2D
	}
	return fix, gpsprot.SolutionDim3D
}

// modeCorrection derives Correction bits from WACorrInfo and Mode.
func modeCorrection(c pvtCommon) gpsprot.CorrKind {
	var corr gpsprot.CorrKind
	switch c.WACorrInfo.BaseType() {
	case sbfbin.WACorrBasePhysical, sbfbin.WACorrBaseVirtual:
		corr |= gpsprot.CorrOSR.Expand()
	case sbfbin.WACorrBaseSSR:
		corr |= gpsprot.CorrSSR.Expand()
	}
	switch c.Mode.Fix() {
	case sbfbin.ModeSBAS:
		corr |= gpsprot.CorrSBAS.Expand()
	case sbfbin.ModePPP:
		corr |= gpsprot.CorrPPP.Expand()
	}
	return corr
}

// accuracyLength converts a u2 0.01 m accuracy field to a Length.
func accuracyLength(v uint16) gpsprot.Length {
	return gpsprot.Length(v) * gpsprot.Centimeter
}

// dopDOP fills DOP figures (and the NumSVUsed fallback) from the DOP block.
func dopDOP(ne *gpsprot.NavEpochMsg, m *sbfbin.DOP) {
	ne.DOP.Pos = dopSBF(m.PDOP)
	ne.DOP.Hor = dopSBF(m.HDOP)
	ne.DOP.Vert = dopSBF(m.VDOP)
	ne.DOP.Time = dopSBF(m.TDOP)
	if m.NrSV != 0 {
		ne.NumSVUsed.Fill(opt.Make(uint16(m.NrSV)))
	}
}

// dopSBF converts a u2 x0.01 DOP field to a float; 0 means "not available".
func dopSBF(v uint16) opt.Val[float64] {
	if v == 0 {
		return opt.Val[float64]{}
	}
	return opt.Make(float64(v) * 0.01)
}

// accPosCovGeodetic fills the horizontal/vertical accuracy fallback from
// PosCovGeodetic (1-sigma RSS of variances). Fill is used so a direct
// HAccuracy/VAccuracy reading always wins.
func accPosCovGeodetic(ne *gpsprot.NavEpochMsg, m *sbfbin.PosCovGeodetic) {
	if l, ok := rssLength(m.Cov_latlat, m.Cov_lonlon); ok {
		ne.Acc.Hor.Fill(opt.Make(l))
	}
	if l, ok := rssLength(m.Cov_hgthgt); ok {
		ne.Acc.Vert.Fill(opt.Make(l))
	}
}

// accPosCovCartesian fills the 3D position accuracy from PosCovCartesian. In 2D
// mode all Cov_* fields are voided together, so the block is skipped.
func accPosCovCartesian(ne *gpsprot.NavEpochMsg, m *sbfbin.PosCovCartesian) {
	if m.Mode.Is2D() {
		return
	}
	if l, ok := rssLength(m.Cov_xx, m.Cov_yy, m.Cov_zz); ok {
		ne.Acc.Pos.Fill(opt.Make(l))
	}
}

// accVelCovCartesian fills the 3D speed accuracy from VelCovCartesian.
func accVelCovCartesian(ne *gpsprot.NavEpochMsg, m *sbfbin.VelCovCartesian) {
	if m.Mode.Is2D() {
		return
	}
	if s, ok := rssSpeed(m.Cov_VxVx, m.Cov_VyVy, m.Cov_VzVz); ok {
		ne.Acc.Speed.Fill(opt.Make(s))
	}
}

// accVelCovGeodetic fills the 2D ground-speed accuracy from VelCovGeodetic.
func accVelCovGeodetic(ne *gpsprot.NavEpochMsg, m *sbfbin.VelCovGeodetic) {
	if s, ok := rssSpeed(m.Cov_VnVn, m.Cov_VeVe); ok {
		ne.Acc.GroundSpeed.Fill(opt.Make(s))
	}
}

// rssVariance returns sqrt of the sum of the given variances, reporting false
// if any is at the do-not-use sentinel.
func rssVariance(vars ...float32) (float64, bool) {
	var sum float64
	for _, v := range vars {
		if v == sbfbin.CovDNU {
			return 0, false
		}
		sum += float64(v)
	}
	return math.Sqrt(sum), true
}

func rssLength(vars ...float32) (gpsprot.Length, bool) {
	if v, ok := rssVariance(vars...); ok {
		return gpsprot.Meters(v), true
	}
	return 0, false
}

func rssSpeed(vars ...float32) (gpsprot.Speed, bool) {
	if v, ok := rssVariance(vars...); ok {
		return gpsprot.MetersPerSecondFromFloat(v), true
	}
	return 0, false
}
