package nov

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// PosGeo converts a Pos into a PosGeoMsg and accumulates
// accuracy into the NavEpochMsg.
func PosGeo[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.Pos[S, P], nativeMsgID string) *gpsprot.PosGeoMsg {
	ne.Acc.Hor.Set(gpsprot.Meters(math.Sqrt(
		float64(b.LatSigma)*float64(b.LatSigma) +
			float64(b.LonSigma)*float64(b.LonSigma))))
	ne.Acc.Vert.Set(gpsprot.Meters(float64(b.HgtSigma)))
	return &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(b.Lat), gpsprot.DegreesFromFloat(b.Lon)},
		Height:      opt.Make(gpsprot.Meters(float64(b.Hgt) + float64(b.Undulation))),
		HeightMSL:   opt.Make(gpsprot.Meters(b.Hgt)),
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

// VelGeoVel converts a Vel into a VelGeoMsg. BESTVEL does not carry
// per-component sigmas, so accuracy is not populated.
func VelGeoVel[P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.Vel[P], nativeMsgID string) *gpsprot.VelGeoMsg {
	return &gpsprot.VelGeoMsg{
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(b.HorSpd)),
		Course:      opt.Make(gpsprot.DegreesFromFloat(b.TrkGnd)),
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

// PosECEFXYZ converts XYZ position fields into a PosECEFMsg and accumulates
// accuracy into the NavEpochMsg.
func PosECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.PosECEFMsg {
	ne.Acc.Pos.Set(gpsprot.Meters(math.Sqrt(
		float64(b.PXSigma)*float64(b.PXSigma) +
			float64(b.PYSigma)*float64(b.PYSigma) +
			float64(b.PZSigma)*float64(b.PZSigma))))
	return &gpsprot.PosECEFMsg{
		Pos:         gpsprot.Point3D{gpsprot.Meters(b.PX), gpsprot.Meters(b.PY), gpsprot.Meters(b.PZ)},
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

// VelECEFXYZ converts XYZ velocity fields into a VelECEFMsg and accumulates
// accuracy into the NavEpochMsg.
func VelECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.VelECEFMsg {
	ne.Acc.Speed.Set(gpsprot.MetersPerSecondFromFloat(math.Sqrt(
		float64(b.VXSigma)*float64(b.VXSigma) +
			float64(b.VYSigma)*float64(b.VYSigma) +
			float64(b.VZSigma)*float64(b.VZSigma))))
	return &gpsprot.VelECEFMsg{
		Vel:         [3]gpsprot.Speed{gpsprot.MetersPerSecondFromFloat(b.VX), gpsprot.MetersPerSecondFromFloat(b.VY), gpsprot.MetersPerSecondFromFloat(b.VZ)},
		NativeMsgID: nativeMsgID,
		Priority:    gpsprot.PriVendorLow,
	}
}

func posGeoBestPos(h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
	m *novmsg.Pos[novmsg.SolStatus, novmsg.PosType], tag gpsprot.Tag, tRead time.Time) (bool, error) {
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs,
		m.GalBDS3Sig, m.GPSGLOBDS2Sig)
	if m.PSolStatus != novmsg.SolComputed {
		return false, nil
	}
	posG := PosGeo(ne, m, "BESTPOS")
	posG.Tag = tag
	h.PosGeo(posG, tRead)
	return true, nil
}

func sinoPosGeoBestPos(h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
	m *novmsg.Pos[novmsg.SolStatus, novmsg.SinoPosType], tag gpsprot.Tag, tRead time.Time) (bool, error) {
	sinoQuality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs)
	if m.PSolStatus != novmsg.SolComputed {
		return false, nil
	}
	posG := PosGeo(ne, m, "BESTPOS")
	posG.Tag = tag
	h.PosGeo(posG, tRead)
	return true, nil
}

func posVelECEFBestXYZ(h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
	m *novmsg.XYZ[novmsg.SolStatus, novmsg.PosType], tag gpsprot.Tag, tRead time.Time) (bool, error) {
	quality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs,
		m.GalBDS3Sig, m.GPSGLOBDS2Sig)
	var posE *gpsprot.PosECEFMsg
	var velE *gpsprot.VelECEFMsg
	if m.PSolStatus == novmsg.SolComputed {
		posE = PosECEFXYZ(ne, m, "BESTXYZ")
	}
	if m.VSolStatus == novmsg.SolComputed {
		velE = VelECEFXYZ(ne, m, "BESTXYZ")
	}
	if posE != nil {
		posE.Tag = tag
		h.PosECEF(posE, tRead)
	}
	if velE != nil {
		velE.Tag = tag
		h.VelECEF(velE, tRead)
	}
	return posE != nil || velE != nil, nil
}

func sinoPosVelECEFBestXYZ(h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
	m *novmsg.XYZ[novmsg.SolStatus, novmsg.SinoPosType], tag gpsprot.Tag, tRead time.Time) (bool, error) {
	sinoQuality(ne, m.PSolStatus, m.PosType,
		m.DiffAge, m.StnID, m.NumSVs, m.NumSolnSVs)
	var posE *gpsprot.PosECEFMsg
	var velE *gpsprot.VelECEFMsg
	if m.PSolStatus == novmsg.SolComputed {
		posE = PosECEFXYZ(ne, m, "BESTXYZ")
	}
	if m.VSolStatus == novmsg.SolComputed {
		velE = VelECEFXYZ(ne, m, "BESTXYZ")
	}
	if posE != nil {
		posE.Tag = tag
		h.PosECEF(posE, tRead)
	}
	if velE != nil {
		velE.Tag = tag
		h.VelECEF(velE, tRead)
	}
	return posE != nil || velE != nil, nil
}

// quality populates solution quality fields on the NavEpochMsg from BESTPOS/BESTXYZ
// fields. Called regardless of PSolStatus: sets FixLevelNone when not SolComputed.
func quality(ne *gpsprot.NavEpochMsg, solStatus novmsg.SolStatus, posType novmsg.PosType,
	diffAge float32, stnID novmsg.StationID, numSVs, numSolnSVs uint8,
	galBds3, gpsGloBds2 novmsg.HexByte) {
	ne.NumSVUsed.Set(uint16(numSolnSVs))
	ne.NumSVTracked.Set(uint16(numSVs))
	ne.GNSSUsed, ne.BandsUsed = SignalsUsed(gpsGloBds2, galBds3)
	if diffAge > 0 {
		ne.DiffAge.Set(gpsprot.Seconds(float64(diffAge)))
	}
	if v, ok := StationIDValue(stnID); ok {
		ne.RTCMRefBaseID.Set(v)
	}
	ne.Correction |= stnIDPPPService(stnID)
	if solStatus != novmsg.SolComputed {
		ne.FixLevel = gpsprot.FixLevelNone
		return
	}
	// OEM7/ByNav-specific pos type values first.
	switch posType {
	case novmsg.PosFixedPos:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDimTimeOnly
		return
	case novmsg.PosFloatConv:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrOSR.Expand()
		return
	case novmsg.PosWideLane:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPartialDualFreq.Expand()
		return
	case novmsg.PosNarrowLane:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrFullDualFreq.Expand()
		return
	case novmsg.PosPropagated:
		ne.FixLevel = gpsprot.FixLevelNone
		return
	case novmsg.PosRTKDirectINS:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrOSR.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosINSSBAS:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrSBAS.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosExtConstrained:
		ne.FixLevel = gpsprot.FixLevelNotMeasured
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosOperational, novmsg.PosWarning, novmsg.PosOutOfBounds:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverged.Expand()
		return
	case novmsg.PosINSPPPConverging:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverging.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosINSPPP:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverged.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosPPPBasicConverging:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverging.Expand()
		return
	case novmsg.PosPPPBasic:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverged.Expand()
		return
	case novmsg.PosINSPPPBasicConverging:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverging.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	case novmsg.PosINSPPPBasic:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPPPConverged.Expand()
		ne.AuxSrc |= gpsprot.AuxSrcINS
		return
	}
	// Shared pos type values.
	fl, fd, ck, aux, ok := PosTypeQuality(uint32(posType))
	if ok {
		ne.FixLevel = fl
		ne.SolutionDim = fd
		ne.Correction = ck
		ne.AuxSrc |= aux
	}
}

// sinoQuality handles SinoGNSS-specific PosType values, then falls through
// to the shared PosTypeQuality for values common across vendors.
func sinoQuality(ne *gpsprot.NavEpochMsg, solStatus novmsg.SolStatus, posType novmsg.SinoPosType,
	diffAge float32, stnID novmsg.StationID, numSVs, numSolnSVs uint8) {
	ne.NumSVUsed.Set(uint16(numSolnSVs))
	ne.NumSVTracked.Set(uint16(numSVs))
	if diffAge > 0 {
		ne.DiffAge.Set(gpsprot.Seconds(float64(diffAge)))
	}
	if v, ok := StationIDValue(stnID); ok {
		ne.RTCMRefBaseID.Set(v)
	}
	if solStatus != novmsg.SolComputed {
		ne.FixLevel = gpsprot.FixLevelNone
		return
	}
	switch posType {
	case novmsg.SinoPosSingleSmooth:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.SolutionDim = gpsprot.SolutionDim3D
		return
	case novmsg.SinoPosFIXDerivation:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrOSR.Expand()
		return
	case novmsg.SinoPosSuperWideLane:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.SolutionDim = gpsprot.SolutionDim3D
		ne.Correction = gpsprot.CorrPartialDualFreq.Expand()
		return
	}
	fl, fd, ck, aux, ok := PosTypeQuality(uint32(posType))
	if ok {
		ne.FixLevel = fl
		ne.SolutionDim = fd
		ne.Correction = ck
		ne.AuxSrc |= aux
	}
}

// stnIDPPPService checks for non-numeric NovAtel PPP service codes
// (e.g. "TSTR", "TSTL", "TSX", "OCXH") in the StationID field and returns
// CorrPPP expanded when found.
func stnIDPPPService(s novmsg.StationID) gpsprot.CorrKind {
	n := len(s)
	for n > 0 && s[n-1] == 0 {
		n--
	}
	if n == 0 {
		return 0
	}
	for i := 0; i < n; i++ {
		if !ascii.IsDigit(s[i]) {
			return gpsprot.CorrPPP.Expand()
		}
	}
	return 0
}

// psrDOP populates DOP fields on the NavEpochMsg from a PSRDOP message.
// PSRDOP does not provide VDOP; DOP.Vert is left unset.
func psrDOP(ne *gpsprot.NavEpochMsg, m *novmsg.PsrDop) {
	ne.DOP.Geom.Set(float64(m.GDOP))
	ne.DOP.Pos.Set(float64(m.PDOP))
	ne.DOP.Hor.Set(float64(m.HDOP))
	ne.DOP.Time.Set(float64(m.TDOP))
}
