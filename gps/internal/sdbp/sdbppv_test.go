package sdbp

import (
	"math"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

func TestPosGeoDatLLA3(t *testing.T) {
	tests := []struct {
		name    string
		input   *sdbpbin.DatLLA3
		want    *gpsprot.PosGeoMsg
		wantNE  gpsprot.NavEpochMsg // expected fields set on NavEpochMsg
	}{
		{
			name: "valid",
			input: &sdbpbin.DatLLA3{
				Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen, FixSats: 12,
				Lat: 13.731845, Lon: 100.644758,
				AltMSL: 6.5, GeoidSep: -28.5,
				HAcc: 1500, VAcc: 2500,
			},
			want: &gpsprot.PosGeoMsg{
				LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.731845), gpsprot.DegreesFromFloat(100.644758)},
				Height:      opt.Make(gpsprot.Meters(6.5 + -28.5)),
				HeightMSL:   opt.Make(gpsprot.Meters(6.5)),
				NativeMsgID: "DAT-LLA3",
			},
			wantNE: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				SolutionDim: gpsprot.SolutionDim3D,
				NumSVUsed:   opt.Make[uint16](12),
				Acc: gpsprot.Accuracy{
					Hor:  opt.Make(gpsprot.Length(1500) * gpsprot.Millimeter),
					Vert: opt.Make(gpsprot.Length(2500) * gpsprot.Millimeter),
				},
			},
		},
		{
			name:  "invalid",
			input: &sdbpbin.DatLLA3{Valid: 0},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ne := &gpsprot.NavEpochMsg{}
			got := posGeoDatLLA3(ne, tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("posGeoDatLLA3:\n  got  %+v\n  want %+v", got, tc.want)
			}
			if tc.want != nil {
				if ne.FixLevel != tc.wantNE.FixLevel {
					t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tc.wantNE.FixLevel)
				}
				if ne.SolutionDim != tc.wantNE.SolutionDim {
					t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tc.wantNE.SolutionDim)
				}
				if ne.NumSVUsed != tc.wantNE.NumSVUsed {
					t.Errorf("NumSVUsed = %v, want %v", ne.NumSVUsed, tc.wantNE.NumSVUsed)
				}
				if ne.Acc.Hor != tc.wantNE.Acc.Hor {
					t.Errorf("Acc.Hor = %v, want %v", ne.Acc.Hor, tc.wantNE.Acc.Hor)
				}
				if ne.Acc.Vert != tc.wantNE.Acc.Vert {
					t.Errorf("Acc.Vert = %v, want %v", ne.Acc.Vert, tc.wantNE.Acc.Vert)
				}
			}
		})
	}
}

func TestVelGeoDatLLA3(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatLLA3{Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen, GroundSpeed: 0.5, Heading: 90.0, SpeedAcc: 10, HeadingAcc: 500}
	got := velGeoDatLLA3(ne, m)
	want := &gpsprot.VelGeoMsg{
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(0.5)),
		Course:      opt.Make(gpsprot.DegreesFromFloat(90.0)),
		NativeMsgID: "DAT-LLA3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoDatLLA3:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestPosECEFDatECEF2(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatECEF2{
		Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen, FixSats: 10,
		X: -1106227.0, Y: 5650780.0, Z: 1502567.0,
		XAcc: 2000, YAcc: 2000, ZAcc: 3000,
	}
	got := posECEFDatECEF2(ne, m)
	want := &gpsprot.PosECEFMsg{
		Pos:         gpsprot.Point3D{gpsprot.Meters(-1106227.0), gpsprot.Meters(5650780.0), gpsprot.Meters(1502567.0)},
		NativeMsgID: "DAT-ECEF2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posECEFDatECEF2:\n  got  %+v\n  want %+v", got, want)
	}
	if ne.NumSVUsed != opt.Make[uint16](10) {
		t.Errorf("NumSVUsed = %v, want 10", ne.NumSVUsed)
	}
	if ne.Acc.Pos != opt.Make(gpsprot.Length(3000)*gpsprot.Millimeter) {
		t.Errorf("Acc.Pos = %v, want 3000mm", ne.Acc.Pos)
	}
}

func TestVelECEFDatECEF2(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatECEF2{
		Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen,
		VX: 0.01, VY: -0.02, VZ: 0.03,
		VXAcc: 5, VYAcc: 5, VZAcc: 5,
	}
	got := velECEFDatECEF2(ne, m)
	want := &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(0.01),
			gpsprot.MetersPerSecondFromFloat(-0.02),
			gpsprot.MetersPerSecondFromFloat(0.03),
		},
		NativeMsgID: "DAT-ECEF2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velECEFDatECEF2:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestVelGeoDatNED3(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatNED3{
		Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen,
		VN: 3.0, VE: 4.0, VD: 0.0,
		VNAcc: 5, VEAcc: 5, VDAcc: 5,
	}
	speed2d := math.Sqrt(9 + 16) // 5.0
	speed3d := speed2d            // VD=0
	got := velGeoDatNED3(ne, m)
	want := &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(3.0),
			gpsprot.MetersPerSecondFromFloat(4.0),
			gpsprot.MetersPerSecondFromFloat(0.0),
		}),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(speed2d)),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(speed3d)),
		NativeMsgID: "DAT-NED3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoDatNED3:\n  got  %+v\n  want %+v", got, want)
	}
}

// TestDopDatDOP uses a real captured DAT-DOP packet in timing-only mode.
func TestDopDatDOP(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatDOP{
		Valid: sdbpbin.DatValidPosTiming, FixSats: 29,
		GDOP: 19, PDOP: 9999, HDOP: 9999, VDOP: 9999, TDOP: 19,
	}
	dopDatDOP(ne, m)
	if ne.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelCode)
	}
	if ne.SolutionDim != gpsprot.SolutionDimTimeOnly {
		t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, gpsprot.SolutionDimTimeOnly)
	}
	if ne.NumSVUsed != opt.Make[uint16](29) {
		t.Errorf("NumSVUsed = %v, want 29", ne.NumSVUsed)
	}
	if ne.DOP.Time != opt.Make(0.19) {
		t.Errorf("DOP.Time = %v, want 0.19", ne.DOP.Time)
	}
}

func TestDopDatDOPNilEpoch(t *testing.T) {
	m := &sdbpbin.DatDOP{Valid: sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen, FixSats: 12, PDOP: 120}
	dopDatDOP(nil, m) // should not panic
}

func TestSurveyDatTSURV(t *testing.T) {
	tests := []struct {
		name  string
		input *sdbpbin.DatTSURV
		want  *gpsprot.SurveyMsg
	}{
		{
			name: "in-progress",
			input: &sdbpbin.DatTSURV{
				Status: 1, ObsTime: 300, ObsCount: 1500,
				AvgX: -114470005, AvgY: 609034510, AvgZ: 150417139,
				AvgVariance: 50000,
			},
			want: &gpsprot.SurveyMsg{
				Position: gpsprot.Point3D{
					gpsprot.Length(-114470005) * gpsprot.Centimeter,
					gpsprot.Length(609034510) * gpsprot.Centimeter,
					gpsprot.Length(150417139) * gpsprot.Centimeter,
				},
				Accuracy:   gpsprot.Length(math.Sqrt(5.0) * float64(gpsprot.Meter)),
				ObsTime:    300 * gpsprot.Second,
				ObsCount:   1500,
				Valid:      false,
				InProgress: true,
			},
		},
		{
			name:  "complete",
			input: &sdbpbin.DatTSURV{Status: 2},
			want: &gpsprot.SurveyMsg{
				Valid:      true,
				InProgress: false,
			},
		},
		{
			name:  "idle",
			input: &sdbpbin.DatTSURV{Status: 0},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := surveyDatTSURV(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("surveyDatTSURV:\n  got  %+v\n  want %+v", got, tc.want)
			}
		})
	}
}

func TestQualityFromValid(t *testing.T) {
	tests := []struct {
		name       string
		valid      sdbpbin.DatValid
		fixLevel   gpsprot.FixLevel
		solDim     gpsprot.SolutionDim
		correction gpsprot.CorrKind
		auxSrc     gpsprot.AuxSrc
	}{
		{"unavailable", sdbpbin.DatValidPosUnavailable | sdbpbin.DatValidFixOpen, gpsprot.FixLevelNone, 0, 0, 0},
		{"DR/unavail", sdbpbin.DatValidPosDR, gpsprot.FixLevelNone, 0, 0, gpsprot.AuxSrcDR},
		{"DR/DR", sdbpbin.DatValidPosDR | sdbpbin.DatValidFixDR, gpsprot.FixLevelNone, 0, 0, gpsprot.AuxSrcDR},
		{"2D/open", sdbpbin.DatValidPos2D | sdbpbin.DatValidFixOpen, gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, 0},
		{"2D/diff", sdbpbin.DatValidPos2D | sdbpbin.DatValidFixDifferential, gpsprot.FixLevelCode, gpsprot.SolutionDim2D, gpsprot.CorrUsed, 0},
		{"2D/auth", sdbpbin.DatValidPos2D | sdbpbin.DatValidFixAuthorized, gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, 0},
		{"3D/open", sdbpbin.DatValidPos3D | sdbpbin.DatValidFixOpen, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0},
		{"3D/diff", sdbpbin.DatValidPos3D | sdbpbin.DatValidFixDifferential, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrUsed, 0},
		{"3D/auth", sdbpbin.DatValidPos3D | sdbpbin.DatValidFixAuthorized, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0},
		{"3D/RTKFixed", sdbpbin.DatValidPos3D | sdbpbin.DatValidFixRTKFixed, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR | gpsprot.CorrUsed, 0},
		{"3D/RTKFloat", sdbpbin.DatValidPos3D | sdbpbin.DatValidFixRTKFloat, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR | gpsprot.CorrUsed, 0},
		{"timing/unavail", sdbpbin.DatValidPosTiming, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0, 0},
		{"timing/open", sdbpbin.DatValidPosTiming | sdbpbin.DatValidFixOpen, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			qualityFromValid(&ne, tt.valid)
			if ne.FixLevel != tt.fixLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.fixLevel)
			}
			if ne.SolutionDim != tt.solDim {
				t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tt.solDim)
			}
			if ne.Correction != tt.correction {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.correction)
			}
			if ne.AuxSrc != tt.auxSrc {
				t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, tt.auxSrc)
			}
		})
	}
}
