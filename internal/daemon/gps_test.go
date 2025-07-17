package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestGPSConfig(t *testing.T) {
	// Baseline target that represents the common structure
	var baseline gpsprot.ConfigTarget
	baseline.Props.SetPPS(defaultPPSWidth)
	baseline.Opts.PVTMsg = gpsevent.TimePulsePVTMsgFlags
	baseline.Opts.NMEAMsg = gpsprot.MakeOption(gpsprot.NMEAMsgNone)

	tests := []struct {
		name                 string
		config               string
		speed                int
		wantSatellitesOutput bool
		tpFlags              gpsTimePulseFlags
		modifyTarget         func(*gpsprot.ConfigTarget)
		expectedWidth        time.Duration
		expectedError        string
	}{
		{
			name: "empty gps config with time pulse",
			config: `[gps]
config = true`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Default behavior: survey mode with SetStatic=true and default survey params
				target.Opts.Survey.MinDur = 2000 * time.Second   // default surveyTime
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default surveyAcc
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
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
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
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
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
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
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
			name: "mobile=true overrides fixed position",
			config: `[gps]
config = true
mobile = true
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 2000 * time.Second   // default
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default
			},
		},
		{
			name: "survey accuracy too small",
			config: `[gps]
config = true
surveyAcc = 0.0001`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
			expectedError:        "error", // any error
		},
		{
			name: "fixed position accuracy too small",
			config: `[gps]
config = true
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
fixedPosAcc = 0.0001`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
			expectedError:        "error", // any error
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
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              gpsTimePulseEnable,
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
			name: "mobile=true without time pulse",
			config: `[gps]
config = true
mobile = true`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              0, // no time pulse
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Create fresh Props without PPS when time pulse is disabled
				target.Props = gpsprot.ConfigProps{}
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.Survey.MinDur = 2000 * time.Second   // default
				target.Opts.Survey.AccLimit = gpsprot.Meters(20) // default
				target.Opts.PVTMsg = gpsevent.NoTimePulsePVTMsgFlags
			},
		},
		{
			name: "survey mode without time pulse",
			config: `[gps]
config = true
surveyTime = 1800
surveyAcc = 8`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              0, // no time pulse
			modifyTarget: func(target *gpsprot.ConfigTarget) {
				// Create fresh Props without PPS when time pulse is disabled
				target.Props = gpsprot.ConfigProps{}
				target.Opts.Survey.MinDur = 1800 * time.Second
				target.Opts.Survey.AccLimit = gpsprot.Meters(8)
				target.Opts.SetStatic = true
				target.Opts.PVTMsg = gpsevent.NoTimePulsePVTMsgFlags
			},
		},
		{
			name:                 "no config without time pulse",
			config:               `[gps]`,
			speed:                9600,
			wantSatellitesOutput: false,
			tpFlags:              0, // no time pulse
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

			target, width, err := cfg.GPS.target(tt.speed, tt.wantSatellitesOutput, tt.tpFlags)
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
			if width != tt.expectedWidth {
				t.Errorf("width: got %v, expected %v", width, tt.expectedWidth)
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
	opt := cfg.GPS.satsMsg(38400, true)
	if opt.Get() != gpsprot.SatsMsgSat {
		t.Errorf("setSatellitesMsg: got %v, want SatellitesMsgSV", opt.Get())
	}
}

func TestRTCMOutput(t *testing.T) {
	cfgStr := `[gps]
	rtcmOutput = true`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.RTCMOutput == nil || *cfg.GPS.RTCMOutput != true {
		t.Errorf("RTCMOutput: got %v, want true", cfg.GPS.RTCMOutput)
	}
	opt := cfg.GPS.rtcmMsg()
	if opt.Get() != gpsprot.RTCMMsgAuto {
		t.Errorf("rtcmMsg: got %v, want RTCMMsgAuto", opt.Get())
	}

	cfgStr = `[gps]
	rtcmOutput = false`
	r = strings.NewReader(cfgStr)
	cfg, err = readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.RTCMOutput == nil || *cfg.GPS.RTCMOutput != false {
		t.Errorf("RTCMOutput: got %v, want false", cfg.GPS.RTCMOutput)
	}
	opt = cfg.GPS.rtcmMsg()
	if opt.Get() != gpsprot.RTCMMsgNone {
		t.Errorf("rtcmMsg: got %v, want RTCMMsgNone", opt.Get())
	}

	cfgStr = `[gps]`
	r = strings.NewReader(cfgStr)
	cfg, err = readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.RTCMOutput != nil {
		t.Errorf("RTCMOutput: got %v, want nil", cfg.GPS.RTCMOutput)
	}
	opt = cfg.GPS.rtcmMsg()
	if opt.IsSet() {
		t.Errorf("rtcmMsg: got set option, want unset option")
	}
}
