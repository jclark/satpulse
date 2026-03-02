package qtmmsg

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestParsePeriodicMsg(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    PeriodicMsg
	}{
		{
			name:    "PVT",
			payload: "PQTMPVT,1,31075000,20221225,083737.000,,3,09,18,31.12738291,117.26372910,34.212,5.267,3.212,2.928,0.238,4.346,34.12,2.16,4.38",
			want: &PVT{
				TOW:     31075000,
				Date:    "20221225",
				Time:    "083737.000",
				FixType: 3,
				NumSV:   9,
				LeapS:   opt.Make[uint8](18),
				Lat:     opt.Make(31.12738291),
				Lon:     opt.Make(117.26372910),
				Alt:     opt.Make(34.212),
				Sep:     opt.Make(5.267),
				VelN:    opt.Make(3.212),
				VelE:    opt.Make(2.928),
				VelD:    opt.Make(0.238),
				Spd:     opt.Make(4.346),
				Heading: opt.Make(34.12),
				HDOP:    opt.Make(2.16),
				PDOP:    opt.Make(4.38),
			},
		},
		{
			name:    "VEL",
			payload: "PQTMVEL,1,154512.100,1.251,2.452,1.245,2.752,3.021,180.512,0.124,0.254,0.250",
			want: &VEL{
				Time:       "154512.100",
				VelN:       opt.Make(1.251),
				VelE:       opt.Make(2.452),
				VelD:       opt.Make(1.245),
				GrdSpd:     opt.Make(2.752),
				Spd:        opt.Make(3.021),
				COG:        opt.Make(180.512),
				GrdSpdAcc:  opt.Make(0.124),
				SpdAcc:     opt.Make(0.254),
				HeadingAcc: opt.Make(0.250),
			},
		},
		{
			name:    "EPE",
			payload: "PQTMEPE,2,1.000,1.000,1.000,1.414,1.732",
			want: &EPE{
				EPENorth: opt.Make(1.0),
				EPEEast:  opt.Make(1.0),
				EPEDown:  opt.Make(1.0),
				EPE2D:    opt.Make(1.414),
				EPE3D:    opt.Make(1.732),
			},
		},
		{
			name:    "DOP",
			payload: "PQTMDOP,1,570643000,1.01,0.88,0.49,0.73,0.50,0.36,0.35",
			want: &DOP{
				TOW:    570643000,
				GDOP:   opt.Make(1.01),
				PDOP:   opt.Make(0.88),
				TDOP:   opt.Make(0.49),
				VDOP:   opt.Make(0.73),
				HDOP:   opt.Make(0.50),
				NDOP:   opt.Make(0.36),
				EDOP:   opt.Make(0.35),
			},
		},
		{
			name:    "SVINStatus",
			payload: "PQTMSVINSTATUS,1,1000,1,01,20,100,-2484434.3645,4875976.9741,3266161.3412,1.2415",
			want: &SVINStatus{
				TOW:     1000,
				Valid:   1,
				Res0:    "01",
				Obs:     20,
				CfgDur:  100,
				MeanX:   opt.Make(-2484434.3645),
				MeanY:   opt.Make(4875976.9741),
				MeanZ:   opt.Make(3266161.3412),
				MeanAcc: opt.Make(1.2415),
			},
		},
		{
			name:    "NAV",
			payload: "PQTMNAV,1,1,1,190423.000,20241224,212681000,2346,18,,,12,,31.45874521,117.41532415,45.1254,-6.1245,,,1.2451,2.1254,5.1242,,,290,1.0,,78,56,,,,,,,1.2101,1.2148,0.4578,1.1547,,,45.124,,",
			want: &NAV{
				TimeStatus: 1,
				TimeRef:    1,
				UTC:        "190423.000",
				Date:       "20241224",
				TOW:        opt.Make[uint32](212681000),
				WN:         opt.Make[uint16](2346),
				LeapSec:    opt.Make[uint8](18),
				SolType:    opt.Make[uint8](12),
				Lat:        opt.Make(31.45874521),
				Lon:        opt.Make(117.41532415),
				Alt:        opt.Make(45.1254),
				Sep:        opt.Make(-6.1245),
				LatStd:     opt.Make(1.2451),
				LonStd:     opt.Make(2.1254),
				AltStd:     opt.Make(5.1242),
				DiffID:     opt.Make[uint16](290),
				DiffAge:    opt.Make(1.0),
				SatView:    78,
				SatUsed:    56,
				HVel:       opt.Make(1.2101),
				VVel:       opt.Make(1.2148),
				HVelStd:    opt.Make(0.4578),
				VVelStd:    opt.Make(1.1547),
				COG:        opt.Make(45.124),
			},
		},
		{
			name:    "EOE",
			payload: "PQTMEOE,1,190423.000,20241224,2346,212681000",
			want: &EOE{
				UTC:    "190423.000",
				Date:   "20241224",
				WN:     2346,
				TOW:    212681000,
			},
		},
		{
			name:    "GeofenceStatus",
			payload: "PQTMGEOFENCESTATUS,1,124521.000,1,2,2,2",
			want: &GeofenceStatus{
				Time:   "124521.000",
				State0: 1,
				State1: 2,
				State2: 2,
				State3: 2,
			},
		},
		{
			name:    "TXT",
			payload: "PQTMTXT,1,01,01,01,0x105f0cf810417c00",
			want: &TXT{
				TotalNum: 1,
				Num:      1,
				TextID:   1,
				Text:     "0x105f0cf810417c00",
			},
		},
		{
			name:    "PL",
			payload: "PQTMPL,1,55045200,5.00,1,1,2879,2718,4766,5344,4323,10902,,,",
			want: &PL{
				TOW:  opt.Make[uint32](55045200),
				PUL:  5.00,
				PosN: opt.Make[uint32](2879),
				PosE: opt.Make[uint32](2718),
				PosD: opt.Make[uint32](4766),
				VelN: opt.Make[uint32](5344),
				VelE: opt.Make[uint32](4323),
				VelD: opt.Make[uint32](10902),
			},
		},
		{
			name:    "ODO",
			payload: "PQTMODO,1,120635.000,1,112.3",
			want: &ODO{
				Time:  "120635.000",
				State: 1,
				Dist:  opt.Make(112.3),
			},
		},
		{
			name:    "AntennaStatus",
			payload: "PQTMANTENNASTATUS,1,1,1",
			want: &AntennaStatus{
				Status:   1,
				PowerInd: 1,
			},
		},
		{
			name:    "unrecognized PQTM",
			payload: "PQTMFOO,1,2,3",
		},
		{
			name:    "non-PQTM",
			payload: "GPGGA,1,2,3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParsePeriodicMsg(tt.payload)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(msg, tt.want) {
				t.Errorf("msg:\n got %+v\nwant %+v", msg, tt.want)
			}
		})
	}
}

func TestParsePeriodicMsgErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"malformed", "PQTMPVT,abc"},
		{"version mismatch", "PQTMPVT,99,31075000,20221225,083737.000,,3,09,18,31.12738291,117.26372910,34.212,5.267,3.212,2.928,0.238,4.346,34.12,2.16,4.38"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParsePeriodicMsg(tt.payload)
			if err == nil {
				t.Error("expected error")
			}
			if msg != nil {
				t.Error("expected nil msg on error")
			}
		})
	}
}

func TestPeriodicMsgVer(t *testing.T) {
	tests := []struct {
		name    string
		wantVer uint8
		wantOK  bool
	}{
		{"PVT", 1, true},
		{"EPE", 2, true},
		{"NAV", 1, true},
		{"UNKNOWN", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, ok := PeriodicMsgVer(tt.name)
			if ver != tt.wantVer || ok != tt.wantOK {
				t.Errorf("PeriodicMsgVer(%q) = (%d, %v), want (%d, %v)", tt.name, ver, ok, tt.wantVer, tt.wantOK)
			}
		})
	}
}

func TestPeriodicMsgID(t *testing.T) {
	tests := []struct {
		msg     PeriodicMsg
		name    string
		version uint8
	}{
		{&PVT{}, "PVT", 1},
		{&VEL{}, "VEL", 1},
		{&EPE{}, "EPE", 2},
		{&DOP{}, "DOP", 1},
		{&SVINStatus{}, "SVINSTATUS", 1},
		{&NAV{}, "NAV", 1},
		{&EOE{}, "EOE", 1},
		{&GeofenceStatus{}, "GEOFENCESTATUS", 1},
		{&TXT{}, "TXT", 1},
		{&PL{}, "PL", 1},
		{&ODO{}, "ODO", 1},
		{&AntennaStatus{}, "ANTENNASTATUS", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ver := tt.msg.ID()
			if name != tt.name || ver != tt.version {
				t.Errorf("ID() = (%q, %d), want (%q, %d)", name, ver, tt.name, tt.version)
			}
		})
	}
}
