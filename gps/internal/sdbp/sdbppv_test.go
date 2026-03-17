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
				Valid: 1, FixSats: 12,
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
				NumSVUsed: opt.Make[uint16](12),
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
	m := &sdbpbin.DatLLA3{Valid: 1, GroundSpeed: 0.5, Heading: 90.0, SpeedAcc: 10, HeadingAcc: 500}
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
		Valid: 1, FixSats: 10,
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
		Valid: 1,
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
		Valid: 1,
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

func TestDopDatDOP(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatDOP{
		Valid: 1, FixSats: 12,
		GDOP: 150, PDOP: 120, HDOP: 80, VDOP: 90, TDOP: 110,
	}
	dopDatDOP(ne, m)
	want := gpsprot.NavEpochMsg{
		NumSVUsed: opt.Make[uint16](12),
		DOP: gpsprot.DOP{
			Geom: opt.Make(1.5),
			Pos:  opt.Make(1.2),
			Hor:  opt.Make(0.8),
			Vert: opt.Make(0.9),
			Time: opt.Make(1.1),
		},
	}
	if !reflect.DeepEqual(ne.DOP, want.DOP) {
		t.Errorf("DOP:\n  got  %+v\n  want %+v", ne.DOP, want.DOP)
	}
	if ne.NumSVUsed != want.NumSVUsed {
		t.Errorf("NumSVUsed = %v, want %v", ne.NumSVUsed, want.NumSVUsed)
	}
}

func TestDopDatDOPNilEpoch(t *testing.T) {
	m := &sdbpbin.DatDOP{Valid: 1, FixSats: 12, PDOP: 120}
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
