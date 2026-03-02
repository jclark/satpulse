package nov

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

func TestPosTypeQuality(t *testing.T) {
	tests := []struct {
		pt      uint32
		name    string
		fl      gpsprot.FixLevel
		fd      gpsprot.FixDim
		ck      gpsprot.CorrKind
		aux     gpsprot.AuxSrc
		wantOK  bool
	}{
		{0, "NONE", gpsprot.FixLevelNone, 0, 0, 0, true},
		{2, "FIXEDHEIGHT", gpsprot.FixLevelCode, gpsprot.FixDim2D, 0, 0, true},
		{8, "DOPPLER_VELOCITY", 0, gpsprot.FixDimVelocityOnly, 0, 0, true},
		{16, "SINGLE", gpsprot.FixLevelCode, gpsprot.FixDim3D, 0, 0, true},
		{17, "PSRDIFF", gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true},
		{18, "SBAS", gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrSBAS.Expand(), 0, true},
		{32, "L1_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true},
		{33, "IONOFREE_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{34, "NARROW_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{48, "L1_INT", gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), 0, true},
		{49, "WIDE_INT", gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrPartialDualFreq.Expand(), 0, true},
		{50, "NARROW_INT", gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{53, "INS_PSRSP", gpsprot.FixLevelCode, gpsprot.FixDim3D, 0, gpsprot.AuxSrcINS, true},
		{54, "INS_PSRDIFF", gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true},
		{55, "INS_RTKFLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true},
		{56, "INS_RTKFIXED", gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, gpsprot.CorrBaseStation.Expand(), gpsprot.AuxSrcINS, true},
		{68, "PPP_CONVERGING", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrPPPConverging.Expand(), 0, true},
		{69, "PPP", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, gpsprot.CorrPPPConverged.Expand(), 0, true},
		// Vendor-specific values should return false
		{1, "FIXEDPOS", 0, 0, 0, 0, false},
		{4, "FLOATCONV", 0, 0, 0, 0, false},
		{52, "INS", 0, 0, 0, 0, false},
		{70, "PPP_AR/OPERATIONAL", 0, 0, 0, 0, false},
		{71, "PPP_RTK/WARNING", 0, 0, 0, 0, false},
		{99, "unknown", 0, 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fl, fd, ck, aux, ok := PosTypeQuality(tt.pt)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if fl != tt.fl {
				t.Errorf("FixLevel = %v, want %v", fl, tt.fl)
			}
			if fd != tt.fd {
				t.Errorf("FixDim = %v, want %v", fd, tt.fd)
			}
			if ck != tt.ck {
				t.Errorf("CorrKind = %v, want %v", ck, tt.ck)
			}
			if aux != tt.aux {
				t.Errorf("AuxSrc = %v, want %v", aux, tt.aux)
			}
		})
	}
}

func TestSignalsUsed(t *testing.T) {
	tests := []struct {
		name        string
		gpsGloBds2  novmsg.HexByte
		galBds3     novmsg.HexByte
		wantSignals []gpsprot.Signal
	}{
		{"GPS_L1CA", 0x01, 0x00, []gpsprot.Signal{gpsprot.SigGPSL1CA}},
		{"GPS_L2P", 0x02, 0x00, []gpsprot.Signal{gpsprot.SigGPSL2P}},
		{"GPS_L5", 0x08, 0x00, []gpsprot.Signal{gpsprot.SigGPSL5}},
		{"GLO_L1_L2", 0x30, 0x00, []gpsprot.Signal{gpsprot.SigGLOL1, gpsprot.SigGLOL2}},
		{"BDS_B1I_B2I", 0xC0, 0x00, []gpsprot.Signal{gpsprot.SigBDSB1I, gpsprot.SigBDSB2I}},
		{"GAL_E1", 0x00, 0x01, []gpsprot.Signal{gpsprot.SigGALE1}},
		{"GAL_E5a_E5b", 0x00, 0x06, []gpsprot.Signal{gpsprot.SigGALE5a, gpsprot.SigGALE5b}},
		{"BDS3_B1C_B2a", 0x00, 0x30, []gpsprot.Signal{gpsprot.SigBDSB1C, gpsprot.SigBDSB2a}},
		{"BDS3_B3I_B2b", 0x00, 0xC0, []gpsprot.Signal{gpsprot.SigBDSB3I, gpsprot.SigBDSB2b}},
		{"all", 0xFF, 0xF7, []gpsprot.Signal{
			gpsprot.SigGPSL1CA, gpsprot.SigGPSL2P, gpsprot.SigGPSL2C, gpsprot.SigGPSL5,
			gpsprot.SigGLOL1, gpsprot.SigGLOL2,
			gpsprot.SigBDSB1I, gpsprot.SigBDSB2I,
			gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
			gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB3I, gpsprot.SigBDSB2b,
		}},
		{"empty", 0x00, 0x00, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := SignalsUsed(tt.gpsGloBds2, tt.galBds3)
			want := gpsprot.SignalSetOf(tt.wantSignals...)
			if ss != want {
				t.Errorf("SignalsUsed(%02x, %02x) = %v, want %v", tt.gpsGloBds2, tt.galBds3, ss, want)
			}
		})
	}
}

func TestStationIDValue(t *testing.T) {
	tests := []struct {
		name   string
		s      novmsg.StationID
		wantV  uint16
		wantOK bool
	}{
		{"zero", novmsg.StationID{'0', 0, 0, 0}, 0, true},
		{"1234", novmsg.StationID{'1', '2', '3', '4'}, 1234, true},
		{"4095", novmsg.StationID{'4', '0', '9', '5'}, 4095, true},
		{"4096_too_large", novmsg.StationID{'4', '0', '9', '6'}, 0, false},
		{"9901_too_large", novmsg.StationID{'9', '9', '0', '1'}, 0, false},
		{"empty", novmsg.StationID{0, 0, 0, 0}, 0, false},
		{"non_numeric", novmsg.StationID{'T', 'S', 'T', 'R'}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := StationIDValue(tt.s)
			if ok != tt.wantOK || v != tt.wantV {
				t.Errorf("StationIDValue(%q) = (%d, %v), want (%d, %v)", tt.s, v, ok, tt.wantV, tt.wantOK)
			}
		})
	}
}

func TestStationIDUint(t *testing.T) {
	tests := []struct {
		name   string
		s      novmsg.StationID
		wantV  uint16
		wantOK bool
	}{
		{"zero", novmsg.StationID{'0', 0, 0, 0}, 0, true},
		{"4096", novmsg.StationID{'4', '0', '9', '6'}, 4096, true},
		{"9901", novmsg.StationID{'9', '9', '0', '1'}, 9901, true},
		{"non_numeric", novmsg.StationID{'T', 'S', 'T', 'R'}, 0, false},
		{"empty", novmsg.StationID{0, 0, 0, 0}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := StationIDUint(tt.s)
			if ok != tt.wantOK || v != tt.wantV {
				t.Errorf("StationIDUint(%q) = (%d, %v), want (%d, %v)", tt.s, v, ok, tt.wantV, tt.wantOK)
			}
		})
	}
}
