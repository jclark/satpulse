package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestGPSConfig(t *testing.T) {
	// Baseline target that represents the common structure
	var baseline gpsprot.ConfigTarget
	baseline.Props.SetPPS(defaultPPSWidth)
	baseline.Opts.PVTMsg = gpsprot.PVTMsgTimingPTP
	baseline.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgNone)

	tests := []struct {
		name          string
		config        string
		speed         int
		cf            cfgFeatures
		modifyTarget  func(*gpsprot.ConfigTarget)
		expectedError string
	}{
		{
			name: "empty gps config with time pulse",
			config: `[gps]
config = true`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Default behavior: survey mode with SetStatic=true and default survey params
				target.Opts.Survey.MinDur = 2000 * time.Second   // default surveyTime
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default surveyAcc
				target.Opts.SetStatic = true
			},
		},
		{
			name: "time pulse with position",
			config: `[gps]
config = true`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg | cfgPosition,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.PVTMsg |= gpsprot.PVTMsgPos
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
			},
		},
		{
			name: "mobile=true disables time mode",
			config: `[gps]
config = true
mobile = true
surveyTime = 3000
surveyAcc = 15`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 3000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(15)
			},
		},
		{
			name: "mobile=false (default) + no fixed position = survey mode",
			config: `[gps]
config = true
surveyTime = 2500
surveyAcc = 10
resurvey = true`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2500 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(10)
				target.Opts.Survey.Flags = gpsprot.SurveyAgain
				target.Opts.SetStatic = true
			},
		},
		{
			name: "fixed position mode",
			config: `[gps]
config = true
surveyTime = 1000
surveyAcc = 5
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
fixedPosAcc = 3`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:       true,
					PosType:      gpsprot.PosTypeECEF,
					FixedPosECEF: [3]gpsprot.Length{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
					FixedPosAcc:  gpsprot.Meters(3),
				})
				target.Opts.Survey.MinDur = 1000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(5)
			},
		},
		{
			name: "fixed position LLH mode",
			config: `[gps]
config = true
surveyTime = 1000
surveyAcc = 5
fixedPosLLH = [51.5007, -0.1246, 11]
fixedPosAcc = 0.05`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:      true,
					PosType:     gpsprot.PosTypeLLH,
					FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(51.5007), gpsprot.DegreesFromFloat(-0.1246)},
					Height:      gpsprot.Meters(11),
					FixedPosAcc: gpsprot.Meters(0.05),
				})
				target.Opts.Survey.MinDur = 1000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(5)
			},
		},
		{
			name: "fixed position LLH zero",
			config: `[gps]
config = true
fixedPosLLH = [0, 0, 0]`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:      true,
					PosType:     gpsprot.PosTypeLLH,
					FixedPosLLH: [2]gpsprot.Angle{0, 0},
					Height:      0,
					FixedPosAcc: gpsprot.Meters(20),
				})
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
			},
		},
		{
			name: "mobile with position enables velocity",
			config: `[gps]
config = true
mobile = true`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg | cfgPosition,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.PVTMsg |= gpsprot.PVTMsgPos | gpsprot.PVTMsgVel
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
			},
		},
		{
			name: "mobile=true overrides fixed position",
			config: `[gps]
config = true
mobile = true
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 2000 * time.Second   // default
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default
			},
		},
		{
			name: "mobile=true overrides fixed position LLH",
			config: `[gps]
config = true
mobile = true
fixedPosLLH = [51.5007, -0.1246, 11]`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
			},
		},
		{
			name: "fixed position ECEF and LLH conflict",
			config: `[gps]
config = true
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
fixedPosLLH = [51.5007, -0.1246, 11]`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "survey accuracy too small",
			config: `[gps]
config = true
surveyAcc = 0.0001`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error", // any error
		},
		{
			name: "fixed position accuracy too small",
			config: `[gps]
config = true
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
fixedPosAcc = 0.0001`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error", // any error
		},
		{
			name: "fixed position LLH latitude too large",
			config: `[gps]
config = true
fixedPosLLH = [91, 0, 0]`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "fixed position LLH longitude too small",
			config: `[gps]
config = true
fixedPosLLH = [0, -181, 0]`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "fixed position LLH height too large",
			config: `[gps]
config = true
fixedPosLLH = [0, 0, 10001]`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "fixed position with resurvey and survey params",
			config: `[gps]
config = true
mobile = false
resurvey = true
surveyTime = 3000
surveyAcc = 10
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
fixedPosAcc = 5`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:       true,
					PosType:      gpsprot.PosTypeECEF,
					FixedPosECEF: [3]gpsprot.Length{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
					FixedPosAcc:  gpsprot.Meters(5),
				})
				target.Opts.Survey.MinDur = 3000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(10)
				target.Opts.Survey.Flags = gpsprot.SurveyAgain
				// Note: unlike old test, survey params are still set in new implementation
			},
		},
		{
			name: "min elevation",
			config: `[gps]
config = true
minElevation = 5.0`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
				target.Props.SetMinElevation(gpsprot.DegreesFromFloat(5.0))
			},
		},
		{
			name: "min elevation out of range",
			config: `[gps]
config = true
minElevation = 100`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "rtcm base ID",
			config: `[gps]
config = true
rtcmBaseID = 100`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
				target.Props.SetRTCMBaseID(100)
			},
		},
		{
			name: "rtcm base ID zero",
			config: `[gps]
config = true
rtcmBaseID = 0`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
				target.Props.SetRTCMBaseID(0)
			},
		},
		{
			name: "rtcm base ID max",
			config: `[gps]
config = true
rtcmBaseID = 4095`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
				target.Props.SetRTCMBaseID(4095)
			},
		},
		{
			name: "rtcm base ID out of range",
			config: `[gps]
config = true
rtcmBaseID = 4096`,
			speed:         9600,
			cf:            cfgTimePulse | cfgTimePulseMsg,
			expectedError: "error",
		},
		{
			name: "rtcm base ID not set",
			config: `[gps]
config = true`,
			speed: 9600,
			cf:    cfgTimePulse | cfgTimePulseMsg,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 2000 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(20)
				target.Opts.SetStatic = true
			},
		},
		{
			name: "mobile=true without time pulse",
			config: `[gps]
config = true
mobile = true`,
			speed: 9600,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Create fresh Props without PPS when time pulse is disabled
				target.Props = gpsprot.ConfigProps{}
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 2000 * time.Second   // default
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default
				target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC
			},
		},
		{
			name: "survey mode without time pulse",
			config: `[gps]
config = true
surveyTime = 1800
surveyAcc = 8`,
			speed: 9600,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Create fresh Props without PPS when time pulse is disabled
				target.Props = gpsprot.ConfigProps{}
				target.Opts.Survey.MinDur = 1800 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(8)
				target.Opts.SetStatic = true
				target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC
			},
		},
		{
			name: "time pulse without time pulse messages",
			config: `[gps]
config = true
surveyTime = 1800
surveyAcc = 8`,
			speed: 9600,
			cf:    cfgTimePulse,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Opts.Survey.MinDur = 1800 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(8)
				target.Opts.SetStatic = true
				target.Opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC
			},
		},
		{
			name:   "no config without time pulse",
			config: `[gps]`,
			speed:  9600,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// When config=false, target should be empty except for NMEAMsg
				*target = gpsprot.ConfigTarget{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.config)
			cfg, err := readConfig(r)
			if err != nil {
				t.Fatal(err)
			}

			target, err := cfg.GPS.target(tt.speed, tt.cf)
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			// Create expected target by copying baseline and applying modifications
			expected := baseline
			if tt.modifyTarget != nil {
				tt.modifyTarget(&expected)
			}

			if *target != expected {
				t.Errorf("target mismatch:\ngot:  %+v\nexpected: %+v", *target, expected)
			}
		})
	}
}

func TestSatellitesInfo(t *testing.T) {
	cfgStr := `[gps]
	satellitesOutput = true`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.SatellitesOutput == nil || *cfg.GPS.SatellitesOutput != true {
		t.Errorf("SatellitesOutputs: got %v, want true", cfg.GPS.SatellitesOutput)
	}
	cfgStr = `[gps]`
	r = strings.NewReader(cfgStr)
	cfg, err = readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.SatellitesOutput != nil {
		t.Errorf("SatellitesOutputs: got %v, want nil", cfg.GPS.SatellitesOutput)
	}
	opt, _ := cfg.GPS.satsMsg(38400, true)
	if opt.Get() != gpsprot.SatsMsgSat|gpsprot.SatsMsgSignal {
		t.Errorf("setSatellitesMsg: got %v, want SatsMsgSat|SatsMsgSignal", opt.Get())
	}
}

func TestRTCMOutput(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		cf         cfgFeatures
		wantOutput bool
		outputSet  bool
		wantPrefer bool
		preferSet  bool
		wantSet    bool
		wantMsg    gpsprot.RTCMMsgFlags
	}{
		{
			name: "true uses MSM4 auto by default",
			config: `[gps]
rtcmOutput = true`,
			outputSet:  true,
			wantOutput: true,
			wantSet:    true,
			wantMsg:    gpsprot.RTCMMsgAuto,
		},
		{
			name: "auto detects MSM7-to-MSM4 conversion",
			config: `[gps]
rtcmOutput = true`,
			cf:         cfgRTCMMSM7To4,
			outputSet:  true,
			wantOutput: true,
			wantSet:    true,
			wantMsg:    gpsprot.RTCMMsgAutoMSM7,
		},
		{
			name: "explicit prefer true",
			config: `[gps]
rtcmOutput = true
rtcmPreferMSM7 = true`,
			outputSet:  true,
			wantOutput: true,
			preferSet:  true,
			wantPrefer: true,
			wantSet:    true,
			wantMsg:    gpsprot.RTCMMsgAutoMSM7,
		},
		{
			name: "explicit prefer false overrides conversion",
			config: `[gps]
rtcmOutput = true
rtcmPreferMSM7 = false`,
			cf:         cfgRTCMMSM7To4,
			outputSet:  true,
			wantOutput: true,
			preferSet:  true,
			wantSet:    true,
			wantMsg:    gpsprot.RTCMMsgAuto,
		},
		{
			name: "false disables RTCM",
			config: `[gps]
rtcmOutput = false
rtcmPreferMSM7 = true`,
			outputSet:  true,
			preferSet:  true,
			wantPrefer: true,
			wantSet:    true,
			wantMsg:    gpsprot.RTCMMsgNone,
		},
		{
			name: "unset output leaves RTCM unset",
			config: `[gps]
rtcmPreferMSM7 = true`,
			preferSet:  true,
			wantPrefer: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := readConfig(strings.NewReader(tt.config))
			if err != nil {
				t.Fatal(err)
			}
			if (cfg.GPS.RTCMOutput != nil) != tt.outputSet {
				t.Fatalf("RTCMOutput set = %t, want %t", cfg.GPS.RTCMOutput != nil, tt.outputSet)
			}
			if tt.outputSet && *cfg.GPS.RTCMOutput != tt.wantOutput {
				t.Errorf("RTCMOutput: got %v, want %v", *cfg.GPS.RTCMOutput, tt.wantOutput)
			}
			if (cfg.GPS.RTCMPreferMSM7 != nil) != tt.preferSet {
				t.Fatalf("RTCMPreferMSM7 set = %t, want %t", cfg.GPS.RTCMPreferMSM7 != nil, tt.preferSet)
			}
			if tt.preferSet && *cfg.GPS.RTCMPreferMSM7 != tt.wantPrefer {
				t.Errorf("RTCMPreferMSM7: got %v, want %v", *cfg.GPS.RTCMPreferMSM7, tt.wantPrefer)
			}
			opt := cfg.GPS.rtcmMsg(tt.cf)
			if opt.IsSet() != tt.wantSet {
				t.Fatalf("rtcmMsg set = %t, want %t", opt.IsSet(), tt.wantSet)
			}
			if tt.wantSet && opt.Get() != tt.wantMsg {
				t.Errorf("rtcmMsg: got %v, want %v", opt.Get(), tt.wantMsg)
			}
		})
	}
}

func TestRTCMOutputAutoMSM7FromConfigTarget(t *testing.T) {
	tests := []struct {
		name   string
		config string
		set    bool
		want   gpsprot.RTCMMsgFlags
	}{
		{
			name: "caster mountpoint conversion",
			config: `[gps]
config = true
rtcmOutput = true

[[ntrip.mountpoint]]
name = "MSM4"
msm7to4 = true`,
			set:  true,
			want: gpsprot.RTCMMsgAutoMSM7,
		},
		{
			name: "stream push conversion",
			config: `[gps]
config = true
rtcmOutput = true

[[stream.push]]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "MSM4"
ntrip.password = "secret"
ntrip.msm7to4 = true`,
			set:  true,
			want: gpsprot.RTCMMsgAutoMSM7,
		},
		{
			name: "explicit false keeps old default",
			config: `[gps]
config = true
rtcmOutput = true
rtcmPreferMSM7 = false

[[ntrip.mountpoint]]
name = "MSM4"
msm7to4 = true`,
			set:  true,
			want: gpsprot.RTCMMsgAuto,
		},
		{
			name: "no conversion keeps old default",
			config: `[gps]
config = true
rtcmOutput = true

[[ntrip.mountpoint]]
name = "MSM7"`,
			set:  true,
			want: gpsprot.RTCMMsgAuto,
		},
		{
			name: "prefer true ignored when output false",
			config: `[gps]
config = true
rtcmOutput = false
rtcmPreferMSM7 = true

[[ntrip.mountpoint]]
name = "MSM4"
msm7to4 = true`,
			set:  true,
			want: gpsprot.RTCMMsgNone,
		},
		{
			name: "prefer true ignored when output unset",
			config: `[gps]
config = true
rtcmPreferMSM7 = true

[[ntrip.mountpoint]]
name = "MSM4"
msm7to4 = true`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := readConfig(strings.NewReader(tt.config))
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.Stream.Validate(); err != nil {
				t.Fatalf("Stream.Validate: %v", err)
			}
			target, err := cfg.GPS.target(38400, configFeatures(cfg, false))
			if err != nil {
				t.Fatal(err)
			}
			if target.Opts.RTCMMsg.IsSet() != tt.set {
				t.Fatalf("RTCMMsg set = %t, want %t", target.Opts.RTCMMsg.IsSet(), tt.set)
			}
			if !tt.set {
				return
			}
			if got := target.Opts.RTCMMsg.Get(); got != tt.want {
				t.Errorf("RTCMMsg = %v, want %v", got, tt.want)
			}
		})
	}
}
