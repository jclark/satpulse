package gpscmd

import (
	"strings"
	"testing"
	"time"

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
	{"ttyS0", []string{"--reset"}, flagVars{configOpts: gpsprot.ConfigOptions{Reset: gpsprot.ResetCold}}},
	{"ttyS0", []string{"--nmea"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		SatsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
	}}},
	{"ttyS0", []string{"--nmea", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		SatsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
		Save:    gpsprot.SaveMinimal,
	}}},
	{"ttyS0", []string{"--pps"}, flagVars{pps: true}},
	{"ttyS0", []string{"--pps", "--save"}, flagVars{pps: true, configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveMinimal}}},
	{"ttyS0", []string{"--mobile"}, flagVars{mobile: true}},
	{"ttyS0", []string{"--survey"}, flagVars{configOpts: gpsprot.ConfigOptions{Survey: gpsprot.Survey{When: gpsprot.TimeModeAny, MinDur: defaultSurveyTime * time.Second, AccLimit: gpsprot.Meters(defaultSurveyAcc)}}}},
	{"ttyS0", []string{"--survey", "--survey-time", "300", "--survey-acc", "5.5"}, flagVars{configOpts: gpsprot.ConfigOptions{Survey: gpsprot.Survey{When: gpsprot.TimeModeAny, MinDur: 300 * time.Second, AccLimit: gpsprot.Meters(5.5)}}}},
	{"ttyS0", []string{"-p"}, flagVars{pps: true}},
	{"ttyS0", []string{"--speed", "9600"}, flagVars{configOpts: gpsprot.ConfigOptions{BaudRate: 9600}}},
	{"ttyS0", []string{"--device-speed", "9600"}, flagVars{localSpeed: 9600}},
	{"ttyS0", []string{"--save-all", "--reset"}, flagVars{configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveAll, Reset: gpsprot.ResetCold}}},
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
	{"ttyS0", []string{"--gnss", "GPS,GAL", "--band", "L1,L2,E5b"}, flagVars{
		enabledSignals: (gpsprot.BandL1 | gpsprot.BandL2 | gpsprot.BandE5b).SignalSet(gpsprot.GPS, gpsprot.GAL),
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
	{"ttyS0", []string{"--save-all"}, flagVars{configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveAll}}},
	{"ttyS0", []string{"--factory-reset"}, flagVars{configOpts: gpsprot.ConfigOptions{Reset: gpsprot.ResetFactory}}},
	// Need configuration changes with --save
	{"ttyS0", []string{"--reset", "--save", "--nmea"}, flagVars{configOpts: gpsprot.ConfigOptions{
		Reset:   gpsprot.ResetCold,
		Save:    gpsprot.SaveMinimal,
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		SatsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
	}}},
	{"ttyS0", []string{"--reset", "--save-all"}, flagVars{configOpts: gpsprot.ConfigOptions{Reset: gpsprot.ResetCold, Save: gpsprot.SaveAll}}},
	// Test --raw-out flag
	{"ttyS0", []string{"--raw-out", "obs"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs)}}},
	{"ttyS0", []string{"--raw-out", "nav"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "obs,nav"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "nav,obs"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgFlags(0))}}},
	{"ttyS0", []string{"--raw-out", "obs", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs), Save: gpsprot.SaveMinimal}}},
	// Test --pvt-out flag
	{"ttyS0", []string{"--pvt-out", "daemon"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsevent.TimePulsePVTMsgFlags | gpsprot.PVTMsgOff}}},
	{"ttyS0", []string{"--pvt-out", "pos"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos}}},
	{"ttyS0", []string{"--pvt-out", "vel"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgVel}}},
	{"ttyS0", []string{"--pvt-out", "time"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTime}}},
	{"ttyS0", []string{"--pvt-out", "tp"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTimePulse}}},
	{"ttyS0", []string{"--pvt-out", "leap"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgLeapSecond}}},
	{"ttyS0", []string{"--pvt-out", "pos,time"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime}}},
	{"ttyS0", []string{"--pvt-out", "pos,vel"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgVel}}},
	{"ttyS0", []string{"--pvt-out", "tp,tai,leap"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond}}},
	{"ttyS0", []string{"--pvt-out", "survey"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgSurvey}}},
	{"ttyS0", []string{"--pvt-out", "off"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgOff}}},
	{"ttyS0", []string{"--pvt-out", "pos", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos, Save: gpsprot.SaveMinimal}}},
	// Test combining --raw-out and --pvt-out
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: gpsprot.MakeOption(gpsprot.RawMsgObs), PVTMsg: gpsprot.PVTMsgPos}}},
	// Test --rtcm-out flag
	{"ttyS0", []string{"--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM7"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7)}}},
	{"ttyS0", []string{"--rtcm-out", "ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM4,ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM7,ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgFlags(0))}}},
	{"ttyS0", []string{"--rtcm-out", "MSM4", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4), Save: gpsprot.SaveMinimal}}},
	// Test case-insensitive RTCM flags
	{"ttyS0", []string{"--rtcm-out", "msm4"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4)}}},
	{"ttyS0", []string{"--rtcm-out", "Msm4,Arp"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}}},
	// Test auto flag functionality
	{"ttyS0", []string{"--rtcm-out", "auto"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	{"ttyS0", []string{"--rtcm-out", "auto,msm7"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	{"ttyS0", []string{"--rtcm-out", "msm7,auto"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	// Test combining multiple output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgObs),
		PVTMsg:  gpsprot.PVTMsgPos,
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
	}}},
	// Test --nmea-out flag
	{"ttyS0", []string{"--nmea-out", "RMC"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC)}}},
	{"ttyS0", []string{"--nmea-out", "GGA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGGA)}}},
	{"ttyS0", []string{"--nmea-out", "GSA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGSA)}}},
	{"ttyS0", []string{"--nmea-out", "GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "ZDA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgZDA)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GSA,GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA,GSA,GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgFlags(0))}}},
	{"ttyS0", []string{"--nmea-out", "RMC", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC), Save: gpsprot.SaveMinimal}}},
	// Test case-insensitive NMEA flags
	{"ttyS0", []string{"--nmea-out", "rmc"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC)}}},
	{"ttyS0", []string{"--nmea-out", "Rmc,Gga"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}}},
	// Test combining all output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4", "--nmea-out", "RMC"}, flagVars{configOpts: gpsprot.ConfigOptions{
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgObs),
		PVTMsg:  gpsprot.PVTMsgPos,
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgRMC),
	}}},
	// Test --binary flag
	{"ttyS0", []string{"--binary"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
	}}},
	{"ttyS0", []string{"--binary", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
		Save:    gpsprot.SaveMinimal,
	}}},
	// Test --binary with explicit --pvt-out (should use the explicit value)
	{"ttyS0", []string{"--binary", "--pvt-out", "tp,leap"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgLeapSecond,
	}}},
	// Test --binary with --rtcm-out (should not set default pvt)
	{"ttyS0", []string{"--binary", "--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgNone),
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgMSM4),
	}}},
	// Test --nmea with explicit --nmea-out
	{"ttyS0", []string{"--nmea", "--nmea-out", "GGA,GSA"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: gpsprot.MakeOption(gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA),
		RTCMMsg: gpsprot.MakeOption(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  gpsprot.MakeOption(gpsprot.RawMsgNone),
		SatsMsg: gpsprot.MakeOption(gpsprot.SatsMsgNone),
	}}},
	// Test --time-gnss flag
	{"ttyS0", []string{"--time-gnss", "GPS"}, flagVars{timeGNSS: gpsprot.GPS}},
	{"ttyS0", []string{"--time-gnss", "GAL"}, flagVars{timeGNSS: gpsprot.GAL}},
	{"ttyS0", []string{"--time-gnss", "BDS"}, flagVars{timeGNSS: gpsprot.BDS}},
	{"ttyS0", []string{"--time-gnss", "GLO"}, flagVars{timeGNSS: gpsprot.GLO}},
	// Test valid combinations of --gnss and --time-gnss
	{"ttyS0", []string{"--gnss", "GPS,GAL", "--time-gnss", "GPS"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GAL),
		timeGNSS:       gpsprot.GPS,
	}},
	{"ttyS0", []string{"--gnss", "GPS,GAL,BDS", "--time-gnss", "GAL"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GAL, gpsprot.BDS),
		timeGNSS:       gpsprot.GAL,
	}},
	{"ttyS0", []string{"--gnss", "BDS,GLO", "--time-gnss", "BDS"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.BDS, gpsprot.GLO),
		timeGNSS:       gpsprot.BDS,
	}},
	// Test --fixed-pos-ecef flag with Big Ben coordinates
	{"ttyS0", []string{"--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"}, flagVars{
		fixedPosECEF: gpsprot.Point3D{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
		fixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
	}},
	// Test --fixed-pos-ecef with custom accuracy (Eiffel Tower)
	{"ttyS0", []string{"--fixed-pos-ecef", "4200935.82,168323.10,4780213.04", "--fixed-pos-acc", "5.0"}, flagVars{
		fixedPosECEF: gpsprot.Point3D{gpsprot.Meters(4200935.82), gpsprot.Meters(168323.10), gpsprot.Meters(4780213.04)},
		fixedPosAcc:  gpsprot.Meters(5.0),
	}},
	// Test --fixed-pos-ecef with save (Statue of Liberty)
	{"ttyS0", []string{"--fixed-pos-ecef", "1331340.65,-4656583.35,4136313.40", "--save"}, flagVars{
		fixedPosECEF: gpsprot.Point3D{gpsprot.Meters(1331340.65), gpsprot.Meters(-4656583.35), gpsprot.Meters(4136313.40)},
		fixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
		configOpts:   gpsprot.ConfigOptions{Save: gpsprot.SaveMinimal},
	}},
	// Test --fixed-pos-ecef with spaces around commas (Mount Everest)
	{"ttyS0", []string{"--fixed-pos-ecef", "302769.89, 5636025.47, 2979493.09"}, flagVars{
		fixedPosECEF: gpsprot.Point3D{gpsprot.Meters(302769.89), gpsprot.Meters(5636025.47), gpsprot.Meters(2979493.09)},
		fixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
	}},
	// Test --fixed-pos-ecef with minimum accuracy
	{"ttyS0", []string{"--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0.001"}, flagVars{
		fixedPosECEF: gpsprot.Point3D{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
		fixedPosAcc:  gpsprot.Meters(0.001),
	}},
}

func TestParseFlagsValid(t *testing.T) {
	for _, tc := range validFlagsTestCases {
		expect := tc.vars
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
	{"--serial-device", "ttyS0", "--speed", "9600", "--survey", "--mobile"},
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
	{"--serial-device", "ttyS0", "--rtcm-out", "MSM4,MSM7"},                    // MSM4 and MSM7 cannot be used together
	{"--serial-device", "ttyS0", "--rtcm-out", "MSM4,MSM7,ARP"},                // MSM4 and MSM7 cannot be used together
	{"--serial-device", "ttyS0", "--factory-reset", "--save", "--gnss", "GPS"}, // multiple incompatible options
	{"--serial-device", "ttyS0", "--raw-out", ""},                              // empty raw-out value
	{"--serial-device", "ttyS0", "--raw-out", "invalid"},                       // invalid raw-out flag
	{"--serial-device", "ttyS0", "--raw-out", "obs,invalid"},                   // partially invalid raw-out flags
	{"--serial-device", "ttyS0", "--pvt-out", ""},                              // empty pvt-out value
	{"--serial-device", "ttyS0", "--pvt-out", "invalid"},                       // invalid pvt-out flag
	{"--serial-device", "ttyS0", "--pvt-out", "time,after"},                    // after requires tp
	{"--serial-device", "ttyS0", "--pvt-out", "pos,tai"},                       // tai requires after or time
	{"--serial-device", "ttyS0", "--pvt-out", "time,ecef"},                     // ecef requires pos or vel
	{"--serial-device", "ttyS0", "--pvt-out", "after,tai"},                     // after requires tp
	{"--serial-device", "ttyS0", "--pvt-out", "ecef,tai"},                      // ecef requires pos or vel, tai requires after or time
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
	{"--serial-device", "ttyS0", "--nmea", "--binary"},            // can't use both --nmea and --binary
	{"--serial-device", "ttyS0", "--binary", "--nmea"},            // can't use both --binary and --nmea
	{"--serial-device", "ttyS0", "--nmea", "--rtcm-out", "MSM4"},  // can't use --nmea with --rtcm-out
	{"--serial-device", "ttyS0", "--nmea", "--pvt-out", "pos"},    // can't use --nmea with --pvt-out
	{"--serial-device", "ttyS0", "--nmea", "--raw-out", "obs"},    // can't use --nmea with --raw-out
	{"--serial-device", "ttyS0", "--binary", "--nmea-out", "RMC"}, // can't use --binary with --nmea-out
	// Test invalid --time-gnss options
	{"--serial-device", "ttyS0", "--time-gnss", "SBAS"},    // not a major GNSS constellation
	{"--serial-device", "ttyS0", "--time-gnss", "QZSS"},    // not a major GNSS constellation
	{"--serial-device", "ttyS0", "--time-gnss", "NAVIC"},   // not a major GNSS constellation
	{"--serial-device", "ttyS0", "--time-gnss", "invalid"}, // invalid GNSS constellation
	// Test invalid combinations of --gnss and --time-gnss
	{"--serial-device", "ttyS0", "--gnss", "GPS,GAL", "--time-gnss", "BDS"}, // BDS not in enabled GNSS
	{"--serial-device", "ttyS0", "--gnss", "GPS", "--time-gnss", "GLO"},     // GLO not in enabled GNSS
	{"--serial-device", "ttyS0", "--gnss", "GAL,BDS", "--time-gnss", "GPS"}, // GPS not in enabled GNSS
	{"--serial-device", "ttyS0", "--gnss", "BDS", "--time-gnss", "GAL"},     // GAL not in enabled GNSS
	// Test invalid --fixed-pos-ecef values
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", ""},                           // empty value
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,2"},                        // too few coordinates
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,2,3,4"},                    // too many coordinates
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "abc,def,ghi"},                // invalid numbers
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,abc,3"},                    // partially invalid numbers
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "7000000,0,0"},                // coordinates too far from Earth
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "0,0,0"},                      // center of Earth (invalid)
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "8000000,8000000,8000000"},    // far outside valid range
	// Test invalid --fixed-pos-acc values
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0"}, // accuracy too small
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0.0005"}, // accuracy below minimum
	// Test mutual exclusion with --mobile and --survey
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--mobile"},   // can't use with mobile
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--survey"},   // can't use with survey
	{"--serial-device", "ttyS0", "--mobile", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"},   // can't use mobile with fixed-pos
	{"--serial-device", "ttyS0", "--survey", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"},   // can't use survey with fixed-pos
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
