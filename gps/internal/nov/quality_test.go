package nov

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

func TestPosTypeQuality(t *testing.T) {
	tests := []struct {
		pt     uint32
		name   string
		fl     gpsprot.FixLevel
		fd     gpsprot.SolutionDim
		ck     gpsprot.CorrKind
		aux    gpsprot.AuxSrc
		wantOK bool
	}{
		{0, "NONE", gpsprot.FixLevelNone, 0, 0, 0, true},
		{2, "FIXEDHEIGHT", gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, 0, true},
		{8, "DOPPLER_VELOCITY", gpsprot.FixLevelDoppler, 0, 0, 0, true},
		{16, "SINGLE", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0, true},
		{17, "PSRDIFF", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true},
		{18, "SBAS", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrSBAS.Expand(), 0, true},
		{32, "L1_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true},
		{33, "IONOFREE_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{34, "NARROW_FLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{48, "L1_INT", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), 0, true},
		{49, "WIDE_INT", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrPartialDualFreq.Expand(), 0, true},
		{50, "NARROW_INT", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrFullDualFreq.Expand(), 0, true},
		{53, "INS_PSRSP", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, gpsprot.AuxSrcINS, true},
		{54, "INS_PSRDIFF", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true},
		{55, "INS_RTKFLOAT", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true},
		{56, "INS_RTKFIXED", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), gpsprot.AuxSrcINS, true},
		{68, "PPP_CONVERGING", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverging.Expand(), 0, true},
		{69, "PPP", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrPPPConverged.Expand(), 0, true},
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
				t.Errorf("SolutionDim = %v, want %v", fd, tt.fd)
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
		name     string
		gpsGlo   novmsg.HexByte
		galBds3  novmsg.HexByte
		wantGNSS gpsprot.GNSSSet
		wantBand gpsprot.Band
	}{
		{"GPS_L1", 0x01, 0x00,
			gpsprot.GNSSSetOf(gpsprot.GPS), gpsprot.BandL1},
		{"GPS_L2", 0x02, 0x00,
			gpsprot.GNSSSetOf(gpsprot.GPS), gpsprot.BandL2},
		{"GPS_L5", 0x04, 0x00,
			gpsprot.GNSSSetOf(gpsprot.GPS), gpsprot.BandL5},
		{"GLO_L1_L2", 0x30, 0x00,
			gpsprot.GNSSSetOf(gpsprot.GLO), gpsprot.BandL1 | gpsprot.BandL2},
		{"GLO_L3", 0x40, 0x00,
			gpsprot.GNSSSetOf(gpsprot.GLO), gpsprot.BandE5b},
		{"GAL_E1", 0x00, 0x01,
			gpsprot.GNSSSetOf(gpsprot.GAL), gpsprot.BandL1},
		{"GAL_E5a_E5b", 0x00, 0x06,
			gpsprot.GNSSSetOf(gpsprot.GAL), gpsprot.BandL5 | gpsprot.BandE5b},
		{"GAL_ALTBOC", 0x00, 0x08,
			gpsprot.GNSSSetOf(gpsprot.GAL), gpsprot.BandL5 | gpsprot.BandE5b},
		{"BDS_B1", 0x00, 0x10,
			gpsprot.GNSSSetOf(gpsprot.BDS), gpsprot.BandL1},
		{"BDS_B2", 0x00, 0x20,
			gpsprot.GNSSSetOf(gpsprot.BDS), gpsprot.BandL5 | gpsprot.BandE5b},
		{"BDS_B3", 0x00, 0x40,
			gpsprot.GNSSSetOf(gpsprot.BDS), gpsprot.BandE6},
		{"GAL_E6", 0x00, 0x80,
			gpsprot.GNSSSetOf(gpsprot.GAL), gpsprot.BandE6},
		{"all", 0x77, 0xFF,
			gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL, gpsprot.BDS),
			gpsprot.BandL1 | gpsprot.BandL2 | gpsprot.BandL5 | gpsprot.BandE5b | gpsprot.BandE6},
		{"empty", 0x00, 0x00, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, b := SignalsUsed(tt.gpsGlo, tt.galBds3)
			if gs != tt.wantGNSS {
				t.Errorf("GNSS = %v, want %v", gs, tt.wantGNSS)
			}
			if b != tt.wantBand {
				t.Errorf("Band = %v, want %v", b, tt.wantBand)
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
