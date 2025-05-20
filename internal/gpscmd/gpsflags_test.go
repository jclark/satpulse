package gpscmd

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

type validFlagsTestCase struct {
	dev  string
	args []string
	vars flagVars
}

var validFlagsTestCases = []validFlagsTestCase{
	{"ttyS0", []string{}, flagVars{}},
	{"ttyS0", []string{"--save"}, flagVars{save: true}},
	{"ttyS0", []string{"--reset"}, flagVars{reset: true}},
	{"ttyS0", []string{"--nmea"}, flagVars{nmea: true}},
	{"ttyS0", []string{"--pps"}, flagVars{pps: true}},
	{"ttyS0", []string{"--disable-time-mode"}, flagVars{disableTimeMode: true}},
	{"ttyS0", []string{"--survey"}, flagVars{survey: true}},
	{"ttyS0", []string{"--survey", "--survey-time", "300", "--survey-acc", "5.5"}, flagVars{
		survey:     true,
		surveyTime: 300,
		surveyAcc:  5.5,
	}},
	{"ttyS0", []string{"-p"}, flagVars{pps: true}},
	{"ttyS0", []string{"--speed", "9600"}, flagVars{remoteSpeed: 9600}},
	{"ttyS0", []string{"--device-speed", "9600"}, flagVars{localSpeed: 9600}},
	{"ttyS0", []string{"--save", "--reset"}, flagVars{save: true, reset: true}},
	{"ttyS0", []string{"--gnss", "GPS,GLO,GAL,BDS"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL, gpsprot.BDS),
	}},
	{"ttyS0", []string{"--gnss", "beidou,gps,glonass,galileo"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL, gpsprot.BDS),
	}},
	{"", []string{"--socket", "/tmp/socket"}, flagVars{socketPath: "/tmp/socket"}},
	{"ttyS0", []string{"-g", "GPS", "--band", "L1"}, flagVars{
		enabledSignals: gpsprot.BandL1.SignalSet(gpsprot.GPS),
	}},
	{"ttyS0", []string{"--gnss", "GPS", "--band", "L2"}, flagVars{
		enabledSignals: gpsprot.BandL2.SignalSet(gpsprot.GPS),
	}},
	{"ttyS0", []string{"--gnss", "GPS", "--band", "L5"}, flagVars{
		enabledSignals: gpsprot.BandL5.SignalSet(gpsprot.GPS),
	}},
	{"ttyS0", []string{"--gnss", "GAL", "--band", "E5"}, flagVars{
		enabledSignals: (gpsprot.BandL5 | gpsprot.BandE5b).SignalSet(gpsprot.GAL),
	}},
	{"ttyS0", []string{"--gnss", "GAL", "--band", "L1,E6"}, flagVars{
		enabledSignals: (gpsprot.BandL1|gpsprot.BandE6).SignalSet(gpsprot.GAL),
	}},
	{"ttyS0", []string{"--gnss", "GPS,GAL", "--band", "L1,L2"}, flagVars{
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandL2).SignalSet(gpsprot.GPS, gpsprot.GAL),
	}},
	{"ttyS0", []string{"--gnss", "GPS", "-b", "L1,L2,L5"}, flagVars{
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandL2 | gpsprot.BandL5).SignalSet(gpsprot.GPS),
	}},
	{"ttyS0", []string{"-g", "GPS,GAL", "--band", "l1,e6"}, flagVars{
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandE6).SignalSet(gpsprot.GPS, gpsprot.GAL),
	}},
	{"ttyS0", []string{"-g", "GPS,BDS", "--band", " L1, L2 "}, flagVars{
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandL2).SignalSet(gpsprot.GPS, gpsprot.BDS),
	}},
	{"ttyS0", []string{"-g", "GPS,GAL,BDS", "--band", "L1,L2,L5,E5,E6"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GAL, gpsprot.BDS),
	}},
}

func TestParseFlagsValid(t *testing.T) {
	for _, tc := range validFlagsTestCases {
		expect := tc.vars
		if expect.surveyAcc == 0 {
			expect.surveyAcc = defaultSurveyAcc
		}
		if expect.surveyTime == 0 {
			expect.surveyTime = defaultSurveyTime
		}
		args := append([]string{}, tc.args...)
		if tc.dev != "" {
			expect.serialDevice = tc.dev

			args = append(args, "--serial-device", tc.dev)
		}
		t.Run(strings.Join(tc.args, ","), func(t *testing.T) {
			vars, _, err := parseFlags("config", args)
			if err != nil {
				t.Errorf("parseFlags returned error: %v", err)
			} else if vars == nil {
				t.Errorf("parseFlags returned nil vars")
			} else if *vars != expect {
				t.Errorf("parseFlags returned %v, expected %v", *vars, expect)
			}
		})
	}
}

var invalidTestCases = [][]string{
	{"--socket", "/tmp/socket", "--serial-device", "ttyS0"},
	{"--serial-device", "ttyS0", "--speed", "0"},
	{},
	{"--serial-device", "ttyS0", "--speed", "9600", "--gnss", "GPS", "--band", "L3"},
	{"--reset"},
	{"--survey", "--disable-time-mode"},
	{"--serial-device", "ttyS0", "--gnss", "SBAS"},                // only augmentation signals
	{"--serial-device", "ttyS0", "--gnss", "GLO", "--band", "L5"}, // no signals in GNSS+band
	{"--serial-device", "ttyS0", "--gnss", "SBAS,QZSS"},           // non-major GNSS
	{"--serial-device", "ttyS0", "--gnss", "GAL", "--band", "E6"}, // only augmentation signals
}

func TestParseFlagsInvalid(t *testing.T) {
	for _, args := range invalidTestCases {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			vars, _, err := parseFlags("config", args)
			if err == nil {
				t.Errorf("parseFlags returned nil error")
			} else if vars != nil {
				t.Errorf("parseFlags returned non-nil vars")
			}
		})
	}
}
