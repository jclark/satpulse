package ntrip

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// majorSignals returns a SignalSet covering one signal per major GNSS,
// some L1 and some L2/L5 so carrier comes out as 2.
func majorSignals() gpsprot.SignalSet {
	return gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C,
		gpsprot.SigGLOL1,
		gpsprot.SigGALE1,
		gpsprot.SigBDSB1I,
	)
}

func TestStreamRecordBuilder(t *testing.T) {
	tests := []struct {
		name             string
		shared           SharedStreamConfig
		props            *gpsprot.ConfigProps
		info             *gpsprot.ReceiverInfo
		version          string
		msm              int
		hasAuth          bool
		configCapturePos *gpsprot.PosGeoMsg
		sc               StreamConfig
		mountName        string
		expect           string
	}{
		{
			name:      "minimal anonymous, no signals",
			shared:    SharedStreamConfig{},
			props:     nil,
			info:      nil,
			version:   "",
			msm:       0,
			hasAuth:   false,
			sc:        StreamConfig{},
			mountName: "BKK",
			expect:    "BKK;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;0;",
		},
		{
			name:    "auth flag emits B",
			shared:  SharedStreamConfig{},
			hasAuth: true,
			expect:  "BKK;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;satpulse;none;B;N;0;",
		},
		{
			name:    "shared overrides apply",
			shared:  SharedStreamConfig{Country: "THA", Network: "MyNet", Lat: opt.Make(gpsprot.DegreesFromFloat(13.76)), Lon: opt.Make(gpsprot.DegreesFromFloat(100.5)), Generator: "Custom", Bitrate: 9600},
			expect:  "BKK;RTCM 3.3;;0;;MyNet;THA;13.76;100.50;0;0;Custom;none;N;N;9600;",
		},
		{
			name: "synthesize MSM4 from major signals",
			shared: SharedStreamConfig{},
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetSignalsEnabled(majorSignals())
				return cp
			}(),
			msm:    4,
			expect: "BKK;RTCM 3.3;1005(1),1074(1),1084(1),1094(1),1124(1),1230(1);2;GPS+GLO+GAL+BDS;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;0;",
		},
		{
			name: "synthesize MSM7",
			shared: SharedStreamConfig{},
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetSignalsEnabled(majorSignals())
				return cp
			}(),
			msm:    7,
			expect: "BKK;RTCM 3.3;1005(1),1077(1),1087(1),1097(1),1127(1),1230(1);2;GPS+GLO+GAL+BDS;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;0;",
		},
		{
			name: "L1-only carrier=1",
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1))
				return cp
			}(),
			msm:    4,
			expect: "BKK;RTCM 3.3;1005(1),1074(1),1094(1);1;GPS+GAL;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;0;",
		},
		{
			name:    "generator from receiver hardware",
			shared:  SharedStreamConfig{},
			info:    &gpsprot.ReceiverInfo{Hardware: "ZED-F9T"},
			version: "satpulse 0.2.0",
			expect:  "BKK;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;ZED-F9T;none;N;N;0;",
		},
		{
			name:    "generator falls through to versionInfo",
			info:    &gpsprot.ReceiverInfo{},
			version: "satpulse 0.2.0",
			expect:  "BKK;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;satpulse 0.2.0;none;N;N;0;",
		},
		{
			name: "lat/lon from ECEF mode",
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetMode(gpsprot.Mode{
					PosType:      gpsprot.PosTypeECEF,
					FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(-2693884), gpsprot.Meters(-4293780), gpsprot.Meters(3857399)},
				})
				return cp
			}(),
			expect: "BKK;RTCM 3.3;;0;;Misc;XXX;37.46;-122.10;0;0;satpulse;none;N;N;0;",
		},
		{
			name: "lat/lon from LLH mode",
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetMode(gpsprot.Mode{
					PosType:     gpsprot.PosTypeLLH,
					FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.76), gpsprot.DegreesFromFloat(100.50)},
				})
				return cp
			}(),
			expect: "BKK;RTCM 3.3;;0;;Misc;XXX;13.76;100.50;0;0;satpulse;none;N;N;0;",
		},
		{
			name: "lat/lon from captured PosGeo when no fixed pos",
			configCapturePos: &gpsprot.PosGeoMsg{
				LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.76), gpsprot.DegreesFromFloat(100.50)},
			},
			expect: "BKK;RTCM 3.3;;0;;Misc;XXX;13.76;100.50;0;0;satpulse;none;N;N;0;",
		},
		{
			name:   "Mode.FixedPos wins over captured PosGeo",
			shared: SharedStreamConfig{},
			props: func() *gpsprot.ConfigProps {
				cp := new(gpsprot.ConfigProps)
				cp.SetMode(gpsprot.Mode{
					PosType:     gpsprot.PosTypeLLH,
					FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.76), gpsprot.DegreesFromFloat(100.50)},
				})
				return cp
			}(),
			configCapturePos: &gpsprot.PosGeoMsg{
				LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(40.0), gpsprot.DegreesFromFloat(-74.0)},
			},
			expect: "BKK;RTCM 3.3;;0;;Misc;XXX;13.76;100.50;0;0;satpulse;none;N;N;0;",
		},
		{
			name:   "shared override wins over captured PosGeo",
			shared: SharedStreamConfig{Lat: opt.Make(gpsprot.DegreesFromFloat(1.5)), Lon: opt.Make(gpsprot.DegreesFromFloat(2.5))},
			configCapturePos: &gpsprot.PosGeoMsg{
				LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(40.0), gpsprot.DegreesFromFloat(-74.0)},
			},
			expect: "BKK;RTCM 3.3;;0;;Misc;XXX;1.50;2.50;0;0;satpulse;none;N;N;0;",
		},
		{
			name:      "stream description overrides identifier",
			sc:        StreamConfig{Description: "Bangkok"},
			mountName: "BKK",
			expect:    "Bangkok;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;0;",
		},
		{
			name:      "stream bitrate overrides shared",
			shared:    SharedStreamConfig{Bitrate: 9600},
			sc:        StreamConfig{Bitrate: 4800},
			mountName: "BKK",
			expect:    "Bangkok;RTCM 3.3;;0;;Misc;XXX;0.00;0.00;0;0;satpulse;none;N;N;4800;",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			build := StreamRecordBuilder(&tc.shared, tc.props, tc.info, tc.version, tc.msm, tc.hasAuth, tc.configCapturePos)
			name := tc.mountName
			if name == "" {
				name = "BKK"
			}
			got := build(&tc.sc, name)
			expect := tc.expect
			// Some test cases above use a hardcoded "Bangkok" identifier,
			// independent of mountName.
			if strings.HasPrefix(expect, "Bangkok") && tc.sc.Description == "" {
				expect = strings.Replace(expect, "Bangkok", name, 1)
			}
			if got != expect {
				t.Errorf("got  %q\nwant %q", got, expect)
			}
		})
	}
}
