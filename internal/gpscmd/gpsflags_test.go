package gpscmd

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
)
type validFlagsTestCase struct {
	dev  string
	args []string
	vars flagVars
}
var validFlagsTestCases = []validFlagsTestCase{
	{"ttyS0", []string{}, flagVars{}},
	{"ttyS0", []string{"--reset"}, flagVars{reset: gpsprot.ResetCold}},
	{"ttyS0", []string{"--nmea"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		satsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
	}},
	{"ttyS0", []string{"--nmea", "--save"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		satsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
		save:    gpsprot.SaveMinimal,
	}},
	{"ttyS0", []string{"--pps"}, flagVars{pps: true}},
	{"ttyS0", []string{"--pps", "--save"}, flagVars{pps: true, save: gpsprot.SaveMinimal}},
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
	{"ttyS0", []string{"--save-all", "--reset"}, flagVars{save: gpsprot.SaveAll, reset: gpsprot.ResetCold}},
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
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandE6).SignalSet(gpsprot.GAL),
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
	{"ttyS0", []string{"--save-all"}, flagVars{save: gpsprot.SaveAll}},
	{"ttyS0", []string{"--factory-reset"}, flagVars{reset: gpsprot.ResetFactory}},
	// Need configuration changes with --save
	{"ttyS0", []string{"--reset", "--save", "--nmea"}, flagVars{
		reset:   gpsprot.ResetCold,
		save:    gpsprot.SaveMinimal,
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		satsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
	}},
	{"ttyS0", []string{"--reset", "--save-all"}, flagVars{reset: gpsprot.ResetCold, save: gpsprot.SaveAll}},
	// Test --raw-out flag
	{"ttyS0", []string{"--raw-out", "obs"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs)}},
	{"ttyS0", []string{"--raw-out", "nav"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgNavData)}},
	{"ttyS0", []string{"--raw-out", "obs,nav"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}},
	{"ttyS0", []string{"--raw-out", "nav,obs"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}},
	{"ttyS0", []string{"--raw-out", "none"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgFlags(0))}},
	{"ttyS0", []string{"--raw-out", "obs", "--save"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs), save: gpsprot.SaveMinimal}},
	// Test --pvt-out flag
	{"ttyS0", []string{"--pvt-out", "daemon"}, flagVars{pvtMsg: gpsevent.PVTMsgFlags}},
	{"ttyS0", []string{"--pvt-out", "pos"}, flagVars{pvtMsg: gpsprot.PVTMsgPos}},
	{"ttyS0", []string{"--pvt-out", "time"}, flagVars{pvtMsg: gpsprot.PVTMsgTime}},
	{"ttyS0", []string{"--pvt-out", "tp"}, flagVars{pvtMsg: gpsprot.PVTMsgTimePulse}},
	{"ttyS0", []string{"--pvt-out", "leap"}, flagVars{pvtMsg: gpsprot.PVTMsgLeapSecond}},
	{"ttyS0", []string{"--pvt-out", "tai"}, flagVars{pvtMsg: gpsprot.PVTMsgTAI}},
	{"ttyS0", []string{"--pvt-out", "pos,time"}, flagVars{pvtMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime}},
	{"ttyS0", []string{"--pvt-out", "tp,tai,leap"}, flagVars{pvtMsg: gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond}},
	{"ttyS0", []string{"--pvt-out", "off"}, flagVars{pvtMsg: gpsprot.PVTMsgOff}},
	{"ttyS0", []string{"--pvt-out", "pos", "--save"}, flagVars{pvtMsg: gpsprot.PVTMsgPos, save: gpsprot.SaveMinimal}},
	// Test combining --raw-out and --pvt-out
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos"}, flagVars{rawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs), pvtMsg: gpsprot.PVTMsgPos}},
	// Test --rtcm-out flag
	{"ttyS0", []string{"--rtcm-out", "MSM4"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4)}},
	{"ttyS0", []string{"--rtcm-out", "MSM7"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7)}},
	{"ttyS0", []string{"--rtcm-out", "ARP"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgARP)}},
	{"ttyS0", []string{"--rtcm-out", "MSM4,MSM7"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7)}},
	{"ttyS0", []string{"--rtcm-out", "MSM4,ARP"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}},
	{"ttyS0", []string{"--rtcm-out", "MSM7,ARP"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP)}},
	{"ttyS0", []string{"--rtcm-out", "MSM4,MSM7,ARP"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP)}},
	{"ttyS0", []string{"--rtcm-out", "none"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgFlags(0))}},
	{"ttyS0", []string{"--rtcm-out", "MSM4", "--save"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4), save: gpsprot.SaveMinimal}},
	// Test case-insensitive RTCM flags
	{"ttyS0", []string{"--rtcm-out", "msm4"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4)}},
	{"ttyS0", []string{"--rtcm-out", "Msm4,Arp"}, flagVars{rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}},
	// Test combining multiple output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4"}, flagVars{
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgObs),
		pvtMsg:  gpsprot.PVTMsgPos,
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
	}},
	// Test --nmea-out flag
	{"ttyS0", []string{"--nmea-out", "RMC"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC)}},
	{"ttyS0", []string{"--nmea-out", "GGA"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGGA)}},
	{"ttyS0", []string{"--nmea-out", "GSA"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGSA)}},
	{"ttyS0", []string{"--nmea-out", "GSV"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGSV)}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}},
	{"ttyS0", []string{"--nmea-out", "RMC,GSA,GSV"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA,GSA,GSV"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}},
	{"ttyS0", []string{"--nmea-out", "none"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgFlags(0))}},
	{"ttyS0", []string{"--nmea-out", "RMC", "--save"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC), save: gpsprot.SaveMinimal}},
	// Test case-insensitive NMEA flags
	{"ttyS0", []string{"--nmea-out", "rmc"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC)}},
	{"ttyS0", []string{"--nmea-out", "Rmc,Gga"}, flagVars{nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}},
	// Test combining all output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4", "--nmea-out", "RMC"}, flagVars{
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgObs),
		pvtMsg:  gpsprot.PVTMsgPos,
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
	}},
	// Test --binary flag
	{"ttyS0", []string{"--binary"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		pvtMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
	}},
	{"ttyS0", []string{"--binary", "--save"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		pvtMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
		save:    gpsprot.SaveMinimal,
	}},
	// Test --binary with explicit --pvt-out (should use the explicit value)
	{"ttyS0", []string{"--binary", "--pvt-out", "tp,leap"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		pvtMsg:  gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgLeapSecond,
	}},
	// Test --binary with --rtcm-out (should not set default pvt)
	{"ttyS0", []string{"--binary", "--rtcm-out", "MSM4"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
	}},
	// Test --nmea with explicit --nmea-out
	{"ttyS0", []string{"--nmea", "--nmea-out", "GGA,GSA"}, flagVars{
		nmeaMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA),
		rtcmMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		rawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		satsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
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
	{"--serial-device", "ttyS0", "--gnss", "SBAS"},                             // only augmentation signals
	{"--serial-device", "ttyS0", "--gnss", "GLO", "--band", "L5"},              // no signals in GNSS+band
	{"--serial-device", "ttyS0", "--gnss", "SBAS,QZSS"},                        // non-major GNSS
	{"--serial-device", "ttyS0", "--gnss", "GAL", "--band", "E6"},              // only augmentation signals
	{"--serial-device", "ttyS0", "--save", "--save-all"},                       // can't use both save and save-all
	{"--serial-device", "ttyS0", "--factory-reset", "--save"},                  // can't use factory-reset with save
	{"--serial-device", "ttyS0", "--factory-reset", "--reset"},                 // can't use factory-reset with reset
	{"--serial-device", "ttyS0", "--factory-reset", "--nmea"},                  // can't use factory-reset with config changes
	{"--serial-device", "ttyS0", "--factory-reset", "--gnss", "GPS"},           // can't use factory-reset with config changes
	{"--serial-device", "ttyS0", "--reset", "--gnss", "GPS"},                   // reset without save would lose config changes
	{"--serial-device", "ttyS0", "--save"},                                     // no config changes to save
	{"--serial-device", "ttyS0", "--reset", "--save"},                          // no config changes to save with --save
	{"--serial-device", "ttyS0", "--factory-reset", "--save"},                  // incompatible options
	{"--serial-device", "ttyS0", "--factory-reset", "--save-all"},              // incompatible options
	{"--serial-device", "ttyS0", "--factory-reset", "--save", "--gnss", "GPS"}, // multiple incompatible options
	{"--serial-device", "ttyS0", "--raw-out", ""},                              // empty raw-out value
	{"--serial-device", "ttyS0", "--raw-out", "invalid"},                       // invalid raw-out flag
	{"--serial-device", "ttyS0", "--raw-out", "obs,invalid"},                   // partially invalid raw-out flags
	{"--serial-device", "ttyS0", "--pvt-out", ""},                              // empty pvt-out value
	{"--serial-device", "ttyS0", "--pvt-out", "invalid"},                       // invalid pvt-out flag
	{"--serial-device", "ttyS0", "--pvt-out", "pos,invalid"},                   // partially invalid pvt-out flags
	{"--serial-device", "ttyS0", "--rtcm-out", ""},                             // empty rtcm-out value
	{"--serial-device", "ttyS0", "--rtcm-out", "invalid"},                      // invalid rtcm-out flag
	{"--serial-device", "ttyS0", "--rtcm-out", "msm4,invalid"},                 // partially invalid rtcm-out flags
	{"--serial-device", "ttyS0", "--rtcm-out", "other"},                        // trying to use hidden "other" flag
	{"--serial-device", "ttyS0", "--nmea-out", ""},                             // empty nmea-out value
	{"--serial-device", "ttyS0", "--nmea-out", "invalid"},                      // invalid nmea-out flag
	{"--serial-device", "ttyS0", "--nmea-out", "RMC,invalid"},                  // partially invalid nmea-out flags
	{"--serial-device", "ttyS0", "--nmea-out", "other"},                        // trying to use hidden "other" flag
	// Test invalid combinations with --nmea and --binary
	{"--serial-device", "ttyS0", "--nmea", "--binary"},                         // can't use both --nmea and --binary
	{"--serial-device", "ttyS0", "--binary", "--nmea"},                         // can't use both --binary and --nmea
	{"--serial-device", "ttyS0", "--nmea", "--rtcm-out", "MSM4"},              // can't use --nmea with --rtcm-out
	{"--serial-device", "ttyS0", "--nmea", "--pvt-out", "pos"},                // can't use --nmea with --pvt-out
	{"--serial-device", "ttyS0", "--nmea", "--raw-out", "obs"},                // can't use --nmea with --raw-out
	{"--serial-device", "ttyS0", "--binary", "--nmea-out", "RMC"},             // can't use --binary with --nmea-out
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
