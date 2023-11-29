package configcmd

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
	{"ttyS0", []string{"--flash"}, flagVars{flash: true}},
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
	{"ttyS0", []string{"--flash", "--reset"}, flagVars{flash: true, reset: true}},
	{"ttyS0", []string{"--gnss", "GPS,GLO,GAL,BDS"}, flagVars{
		primaryGNSS: gpsprot.GPS,
		enabledGNSS: gpsprot.MajorGNSSSet}},
	{"ttyS0", []string{"--gnss", "beidou,gps,glonass,galileo"}, flagVars{
		primaryGNSS: gpsprot.BDS,
		enabledGNSS: gpsprot.MajorGNSSSet}},
	{"", []string{"--socket", "/tmp/socket"}, flagVars{socketPath: "/tmp/socket"}},
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
	{"--reset"},
	{"--survey", "--disable-time-mode"},
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
