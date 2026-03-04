package nov

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

func TestQualityOEM7(t *testing.T) {
	tests := []struct {
		name      string
		posType   uint32
		wantLevel gpsprot.FixLevel
		wantDim   gpsprot.SolutionDim
		wantCorr  gpsprot.CorrKind
		wantAux   gpsprot.AuxSrc
	}{
		{"FIXEDPOS", 1, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0, 0},
		{"FLOATCONV", 4, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0},
		{"WIDELANE", 5, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrPartialDualFreq.Expand(), 0},
		{"NARROWLANE", 6, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0},
		{"PROPAGATED", 19, gpsprot.FixLevelNone, 0, 0, 0},
		{"RTK_DIRECT_INS", 51, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS},
		{"INS_SBAS", 52, gpsprot.FixLevelCodeCorrected, gpsprot.SolutionDim3D, gpsprot.CorrSBAS.Expand(), gpsprot.AuxSrcINS},
		{"EXT_CONSTRAINED", 67, gpsprot.FixLevelNotMeasured, 0, 0, gpsprot.AuxSrcINS},
		{"OPERATIONAL", 70, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0},
		{"WARNING", 71, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0},
		{"OUT_OF_BOUNDS", 72, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0},
		{"INS_PPP_CONVERGING", 73, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverging.Expand(), gpsprot.AuxSrcINS},
		{"INS_PPP", 74, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), gpsprot.AuxSrcINS},
		{"PPP_BASIC_CONVERGING", 77, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverging.Expand(), 0},
		{"PPP_BASIC", 78, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0},
		{"INS_PPP_BASIC_CONVERGING", 79, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverging.Expand(), gpsprot.AuxSrcINS},
		{"INS_PPP_BASIC", 80, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), gpsprot.AuxSrcINS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			quality(&ne, novmsg.SolComputed, novmsg.PosType(tt.posType),
				0, novmsg.StationID{}, 0, 0, 0, 0)
			if ne.FixLevel != tt.wantLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.wantLevel)
			}
			if ne.SolutionDim != tt.wantDim {
				t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tt.wantDim)
			}
			if ne.Correction != tt.wantCorr {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.wantCorr)
			}
			if ne.AuxSrc != tt.wantAux {
				t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, tt.wantAux)
			}
		})
	}
}

func TestQualitySinoGNSS(t *testing.T) {
	tests := []struct {
		name      string
		posType   uint32
		wantLevel gpsprot.FixLevel
		wantDim   gpsprot.SolutionDim
		wantCorr  gpsprot.CorrKind
	}{
		{"SINGLE_SMOOTH", 9, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0},
		{"FIX_DERIVATION", 35, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand()},
		{"SUPER_WIDE_LANE", 51, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrPartialDualFreq.Expand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			sinoQuality(&ne, novmsg.SolComputed, novmsg.SinoPosType(tt.posType),
				0, novmsg.StationID{}, 0, 0, 0, 0)
			if ne.FixLevel != tt.wantLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.wantLevel)
			}
			if ne.SolutionDim != tt.wantDim {
				t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tt.wantDim)
			}
			if ne.Correction != tt.wantCorr {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.wantCorr)
			}
		})
	}
}

func TestQualityFields(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	m := &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus:    novmsg.SolComputed,
		PosType:       novmsg.PosNarrowInt,
		Lat:           47.0,
		Lon:           8.0,
		Hgt:           400.0,
		LatSigma:      0.01,
		LonSigma:      0.01,
		HgtSigma:      0.02,
		DiffAge:       1.5,
		StnID:         novmsg.StationID{'1', '2', '3', 0},
		NumSVs:        20,
		NumSolnSVs:    15,
		GPSGLOBDS2Sig: 0x01, // GPS L1CA
		GalBDS3Sig:    0x01, // GAL E1
	}}
	posGeoBestPos(&gpsprot.DefaultHandler{}, &ne, &m.Pos, TagBinary, time.Time{})
	if ne.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Errorf("FixLevel = %v, want CarrierFixed", ne.FixLevel)
	}
	if ne.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("SolutionDim = %v, want 3D", ne.SolutionDim)
	}
	if ne.Correction != gpsprot.CorrFullDualFreq.Expand() {
		t.Errorf("Correction = %v, want FullDualFreq expanded", ne.Correction)
	}
	if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 15 {
		t.Errorf("NumSVUsed = %v, want 15", ne.NumSVUsed)
	}
	if !ne.NumSVTracked.IsSet() || ne.NumSVTracked.Get() != 20 {
		t.Errorf("NumSVTracked = %v, want 20", ne.NumSVTracked)
	}
	if !ne.DiffAge.IsSet() || ne.DiffAge.Get() != gpsprot.Seconds(1.5) {
		t.Errorf("DiffAge = %v, want 1.5s", ne.DiffAge)
	}
	if !ne.RTCMRefBaseID.IsSet() || ne.RTCMRefBaseID.Get() != 123 {
		t.Errorf("RTCMRefBaseID = %v, want 123", ne.RTCMRefBaseID)
	}
	wantSig := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1)
	if ne.SignalsUsed != wantSig {
		t.Errorf("SignalsUsed = %v, want %v", ne.SignalsUsed, wantSig)
	}
}

func TestQualityNotComputed(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	quality(&ne, novmsg.InsufficientObs, novmsg.PosType(novmsg.PosSingle),
		0, novmsg.StationID{}, 5, 0, 0, 0)
	if ne.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %v, want None", ne.FixLevel)
	}
	if ne.SolutionDim != 0 {
		t.Errorf("SolutionDim = %v, want 0", ne.SolutionDim)
	}
	if ne.Correction != 0 {
		t.Errorf("Correction = %v, want 0", ne.Correction)
	}
}

func TestPsrDOP(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	m := &novmsg.PsrDop{PsrDopInitChunk: novmsg.PsrDopInitChunk{
		GDOP: 2.5,
		PDOP: 2.1,
		HDOP: 1.1,
		TDOP: 1.3,
	}}
	psrDOP(&ne, m)
	if !ne.DOP.Geom.IsSet() || ne.DOP.Geom.Get() != float64(float32(2.5)) {
		t.Errorf("DOP.Geom = %v, want 2.5", ne.DOP.Geom)
	}
	if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != float64(float32(2.1)) {
		t.Errorf("DOP.Pos = %v, want 2.1", ne.DOP.Pos)
	}
	if !ne.DOP.Hor.IsSet() || ne.DOP.Hor.Get() != float64(float32(1.1)) {
		t.Errorf("DOP.Hor = %v, want 1.1", ne.DOP.Hor)
	}
	if !ne.DOP.Time.IsSet() || ne.DOP.Time.Get() != float64(float32(1.3)) {
		t.Errorf("DOP.Time = %v, want 1.3", ne.DOP.Time)
	}
	if ne.DOP.Vert.IsSet() {
		t.Error("DOP.Vert should not be set (PSRDOP does not provide VDOP)")
	}
}

func TestStnIDPPPService(t *testing.T) {
	tests := []struct {
		name     string
		stnID    novmsg.StationID
		wantCorr gpsprot.CorrKind
	}{
		{"numeric", novmsg.StationID{'1', '2', '3', 0}, 0},
		{"empty", novmsg.StationID{}, 0},
		{"TSTR", novmsg.StationID{'T', 'S', 'T', 'R'}, gpsprot.CorrPPP.Expand()},
		{"TSTL", novmsg.StationID{'T', 'S', 'T', 'L'}, gpsprot.CorrPPP.Expand()},
		{"TSX", novmsg.StationID{'T', 'S', 'X', 0}, gpsprot.CorrPPP.Expand()},
		{"OCXH", novmsg.StationID{'O', 'C', 'X', 'H'}, gpsprot.CorrPPP.Expand()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stnIDPPPService(tt.stnID)
			if got != tt.wantCorr {
				t.Errorf("stnIDPPPService(%v) = %v, want %v", tt.stnID, got, tt.wantCorr)
			}
		})
	}
}

func TestStnIDPPPServiceInQuality(t *testing.T) {
	// Verify that non-numeric StationID enriches Correction with CorrPPP
	// when quality is called with a PPP pos type.
	var ne gpsprot.NavEpochMsg
	quality(&ne, novmsg.SolComputed, novmsg.PosType(novmsg.PosPPP),
		0, novmsg.StationID{'T', 'S', 'T', 'R'}, 10, 8, 0, 0)
	if ne.Correction&gpsprot.CorrPPP.Expand() != gpsprot.CorrPPP.Expand() {
		t.Errorf("Correction = %v, want CorrPPP bits set", ne.Correction)
	}
}
