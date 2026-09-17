package septentrio

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

func TestModeFixLevel(t *testing.T) {
	tests := []struct {
		mode sbfbin.Mode
		fix  gpsprot.FixLevel
		dim  gpsprot.SolutionDim
	}{
		{sbfbin.ModeNoPVT, gpsprot.FixLevelNone, 0},
		{sbfbin.ModeStandalone, gpsprot.FixLevelCode, gpsprot.SolutionDim3D},
		{sbfbin.ModeDifferential, gpsprot.FixLevelCode, gpsprot.SolutionDim3D},
		{sbfbin.ModeFixedLocation, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly},
		{sbfbin.ModeRTKFixed, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D},
		{sbfbin.ModeRTKFloat, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D},
		{sbfbin.ModeSBAS, gpsprot.FixLevelCode, gpsprot.SolutionDim3D},
		{sbfbin.ModeMovingBaseRTKFixed, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D},
		{sbfbin.ModeMovingBaseRTKFloat, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D},
		{sbfbin.ModePPP, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D},
		{sbfbin.ModeStandalone | 0x80, gpsprot.FixLevelCode, gpsprot.SolutionDim2D},
	}
	for _, tc := range tests {
		fix, dim := modeFixLevel(tc.mode)
		if fix != tc.fix || dim != tc.dim {
			t.Errorf("modeFixLevel(%#x) = (%v,%v), want (%v,%v)", uint8(tc.mode), fix, dim, tc.fix, tc.dim)
		}
	}
}

func TestModeCorrection(t *testing.T) {
	physical := modeCorrection(pvtCommon{WACorrInfo: sbfbin.WACorrInfo(sbfbin.WACorrBasePhysical << 5)})
	if physical&gpsprot.CorrOSR == 0 || physical&gpsprot.CorrUsed == 0 {
		t.Errorf("physical base: want OSR|Used, got %v", physical)
	}
	ssr := modeCorrection(pvtCommon{WACorrInfo: sbfbin.WACorrInfo(sbfbin.WACorrBaseSSR << 5)})
	if ssr&gpsprot.CorrSSR == 0 {
		t.Errorf("SSR base: want SSR, got %v", ssr)
	}
	sbas := modeCorrection(pvtCommon{Mode: sbfbin.ModeSBAS})
	if sbas&gpsprot.CorrSBAS == 0 {
		t.Errorf("SBAS mode: want CorrSBAS, got %v", sbas)
	}
	ppp := modeCorrection(pvtCommon{Mode: sbfbin.ModePPP})
	if ppp&gpsprot.CorrPPP == 0 {
		t.Errorf("PPP mode: want CorrPPP, got %v", ppp)
	}
	none := modeCorrection(pvtCommon{})
	if none != 0 {
		t.Errorf("no corrections: want 0, got %v", none)
	}
}

// TestDopDOP checks the x0.01 scaling from a real DOP block.
func TestDopDOP(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "DOP")
	var ne gpsprot.NavEpochMsg
	dopDOP(&ne, b.Params.(*sbfbin.DOP))
	approx(t, "PDOP", ne.DOP.Pos.Get(), 1.36, 1e-9)
	approx(t, "HDOP", ne.DOP.Hor.Get(), 0.81, 1e-9)
	approx(t, "VDOP", ne.DOP.Vert.Get(), 1.09, 1e-9)
	approx(t, "TDOP", ne.DOP.Time.Get(), 0.89, 1e-9)
	if !ne.NumSVUsed.IsSet() {
		t.Error("NumSVUsed fallback from DOP.NrSV unset")
	}
}

func TestDopSBFZeroIsUnavailable(t *testing.T) {
	v := dopSBF(0)
	if v.IsSet() {
		t.Error("DOP 0 must be treated as not available")
	}
}

// TestQualityAccPrimaryWinsOverCov verifies HAccuracy (direct) beats the
// PosCovGeodetic RSS fallback for Acc.Hor/Vert.
func TestQualityAccPrimaryWinsOverCov(t *testing.T) {
	pvt := findBlock(t, "pvt-cov.jsonl", "PVTGeodetic").Params.(*sbfbin.PVTGeodetic)
	cov := findBlock(t, "pvt-cov.jsonl", "PosCovGeodetic").Params.(*sbfbin.PosCovGeodetic)
	var ne gpsprot.NavEpochMsg
	// Cov arrives first, then the direct reading must overwrite it.
	accPosCovGeodetic(&ne, cov)
	qualityPVT(&ne, pvtGeodeticCommon(pvt), opt.Val[uint16]{}, false)
	approx(t, "Acc.Hor", ne.Acc.Hor.Get().Meters(), float64(pvt.HAccuracy)*0.01, 1e-9)
	approx(t, "Acc.Vert", ne.Acc.Vert.Get().Meters(), float64(pvt.VAccuracy)*0.01, 1e-9)
	if ne.GNSSUsed.IsZero() {
		t.Error("GNSSUsed empty")
	}
	if !ne.GNSSUsed.Contains(gpsprot.GPS) {
		t.Error("GNSSUsed should contain GPS (SignalInfo bit 0)")
	}
}

// TestQualityAccCovFallback uses the covariance RSS when HAccuracy is DNU.
func TestQualityAccCovFallback(t *testing.T) {
	cov := findBlock(t, "pvt-cov.jsonl", "PosCovGeodetic").Params.(*sbfbin.PosCovGeodetic)
	var g sbfbin.PVTGeodetic
	g.Mode = sbfbin.ModeStandalone
	g.HAccuracy, g.VAccuracy = sbfbin.PVTAccuracyDNU, sbfbin.PVTAccuracyDNU
	g.MeanCorrAge = sbfbin.PVTMeanCorrAgeDNU
	g.NrSV = sbfbin.PVTNrSVDNU
	var ne gpsprot.NavEpochMsg
	qualityPVT(&ne, pvtGeodeticCommon(&g), opt.Val[uint16]{}, false)
	if ne.Acc.Hor.IsSet() {
		t.Error("Acc.Hor should be unset before cov fallback")
	}
	accPosCovGeodetic(&ne, cov)
	if !ne.Acc.Hor.IsSet() || !ne.Acc.Vert.IsSet() {
		t.Error("cov fallback did not fill Acc.Hor/Vert")
	}
}

// TestRTCMRefBaseIDPrimary prefers the observed BaseStation ID.
func TestRTCMRefBaseIDPrimary(t *testing.T) {
	var g sbfbin.PVTGeodetic
	g.Mode = sbfbin.ModeRTKFixed
	g.WACorrInfo = sbfbin.WACorrInfo(sbfbin.WACorrBasePhysical << 5)
	g.ReferenceID = 99
	var ne gpsprot.NavEpochMsg
	qualityPVT(&ne, pvtGeodeticCommon(&g), opt.Make(uint16(33)), true)
	if got := ne.RTCMRefBaseID.Get(); got != 33 {
		t.Errorf("RTCMRefBaseID = %d, want 33 (BaseStation primary)", got)
	}
}

// TestRTCMRefBaseIDPrimaryRequiresCurrentRTCM rejects a cached BaseStation
// ID when either the current PVT or the BaseStation source is not RTCM-like.
func TestRTCMRefBaseIDPrimaryRequiresCurrentRTCM(t *testing.T) {
	var g sbfbin.PVTGeodetic
	g.Mode = sbfbin.ModeStandalone
	g.WACorrInfo = sbfbin.WACorrInfo(sbfbin.WACorrBaseNone << 5)
	g.ReferenceID = sbfbin.PVTReferenceIDDNU
	var ne gpsprot.NavEpochMsg
	qualityPVT(&ne, pvtGeodeticCommon(&g), opt.Make(uint16(33)), true)
	if ne.RTCMRefBaseID.IsSet() {
		t.Error("standalone mode must not reuse cached RTCMRefBaseID")
	}
	g.Mode = sbfbin.ModeRTKFixed
	g.WACorrInfo = sbfbin.WACorrInfo(sbfbin.WACorrBasePhysical << 5)
	var ne2 gpsprot.NavEpochMsg
	qualityPVT(&ne2, pvtGeodeticCommon(&g), opt.Make(uint16(33)), false)
	if ne2.RTCMRefBaseID.IsSet() {
		t.Error("non-RTCM BaseStation source must not become RTCMRefBaseID")
	}
}

// TestRTCMRefBaseIDFallback uses an in-range DGNSS/RTK ReferenceID only when a
// physical/virtual base applies.
func TestRTCMRefBaseIDFallback(t *testing.T) {
	var g sbfbin.PVTGeodetic
	g.Mode = sbfbin.ModeRTKFixed
	g.WACorrInfo = sbfbin.WACorrInfo(sbfbin.WACorrBasePhysical << 5)
	g.ReferenceID = 77
	var ne gpsprot.NavEpochMsg
	qualityPVT(&ne, pvtGeodeticCommon(&g), opt.Val[uint16]{}, false)
	if got := ne.RTCMRefBaseID.Get(); got != 77 {
		t.Errorf("RTCMRefBaseID = %d, want 77 (ReferenceID fallback)", got)
	}
	g.Mode = sbfbin.ModeDifferential
	g.ReferenceID = 78
	var ne2 gpsprot.NavEpochMsg
	qualityPVT(&ne2, pvtGeodeticCommon(&g), opt.Val[uint16]{}, false)
	if got := ne2.RTCMRefBaseID.Get(); got != 78 {
		t.Errorf("RTCMRefBaseID = %d, want 78 (DGNSS ReferenceID fallback)", got)
	}
	g.Mode = sbfbin.ModeStandalone
	g.ReferenceID = 123
	var ne3 gpsprot.NavEpochMsg
	qualityPVT(&ne3, pvtGeodeticCommon(&g), opt.Val[uint16]{}, false)
	if ne3.RTCMRefBaseID.IsSet() {
		t.Error("standalone mode ReferenceID must not become RTCMRefBaseID")
	}
	g.Mode = sbfbin.ModeDifferential
	g.ReferenceID = 4096
	var ne4 gpsprot.NavEpochMsg
	qualityPVT(&ne4, pvtGeodeticCommon(&g), opt.Val[uint16]{}, false)
	if ne4.RTCMRefBaseID.IsSet() {
		t.Error("out-of-range ReferenceID must not become RTCMRefBaseID")
	}
}
