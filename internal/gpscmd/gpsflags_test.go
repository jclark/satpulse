package gpscmd

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/time/app/daemon"
)

type validFlagsTestCase struct {
	dev  string
	args []string
	vars flagVars
}

var validFlagsTestCases = []validFlagsTestCase{
	{"ttyS0", []string{}, flagVars{showReceiver: true}},
	{"ttyS0", []string{"--reset"}, flagVars{configOpts: gpsprot.ConfigOptions{Reset: gpsprot.ResetCold}}},
	{"ttyS0", []string{"--nmea"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC),
		RTCMMsg: opt.Make(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  opt.Make(gpsprot.RawMsgNone),
		SatsMsg: opt.Make(gpsprot.SatsMsgNone),
	}}},
	{"ttyS0", []string{"--nmea", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC),
		RTCMMsg: opt.Make(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  opt.Make(gpsprot.RawMsgNone),
		SatsMsg: opt.Make(gpsprot.SatsMsgNone),
		Save:    gpsprot.SaveMinimal,
	}}},
	{"ttyS0", []string{"--pps", "0.1"}, flagVars{pps: opt.Make(100 * time.Millisecond)}},
	{"ttyS0", []string{"--pps", "0.1", "--save"}, flagVars{pps: opt.Make(100 * time.Millisecond), configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveMinimal}}},
	{"ttyS0", []string{"--pps", "0"}, flagVars{pps: opt.Make(time.Duration(0))}},
	{"ttyS0", []string{"-p", "0.1"}, flagVars{pps: opt.Make(100 * time.Millisecond)}},
	{"ttyS0", []string{"--mobile"}, flagVars{mode: opt.Make(gpsprot.Mode{Static: false})}},
	{"ttyS0", []string{"--survey"}, flagVars{mode: opt.Make(gpsprot.Mode{Static: true}), configOpts: gpsprot.ConfigOptions{Survey: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: defaultSurveyTime * time.Second, AccLimit: gpsprot.Meters(defaultSurveyAcc)}}}},
	{"ttyS0", []string{"--survey", "--survey-time", "300", "--survey-acc", "5.5"}, flagVars{mode: opt.Make(gpsprot.Mode{Static: true}), configOpts: gpsprot.ConfigOptions{Survey: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: 300 * time.Second, AccLimit: gpsprot.Meters(5.5)}}}},
	{"ttyS0", []string{"--speed", "9600"}, flagVars{configOpts: gpsprot.ConfigOptions{BaudRate: 9600}}},
	{"ttyS0", []string{"--device-speed", "9600"}, flagVars{localSpeed: 9600, showReceiver: true}},
	{"ttyS0", []string{"--save-all", "--reset"}, flagVars{configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveAll, Reset: gpsprot.ResetCold}}},
	{"ttyS0", []string{"--gnss", "GPS,GLO,GAL,BDS"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL, gpsprot.BDS),
	}},
	{"ttyS0", []string{"--gnss", "beidou,gps,glonass,galileo"}, flagVars{
		enabledSignals: gpsprot.BandAll.SignalSet(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL, gpsprot.BDS),
	}},
	{"", []string{"--socket", "/tmp/socket"}, flagVars{socketPath: "/tmp/socket", showReceiver: true}},
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
		NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC),
		RTCMMsg: opt.Make(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  opt.Make(gpsprot.RawMsgNone),
		SatsMsg: opt.Make(gpsprot.SatsMsgNone),
	}}},
	{"ttyS0", []string{"--reset", "--save-all"}, flagVars{configOpts: gpsprot.ConfigOptions{Reset: gpsprot.ResetCold, Save: gpsprot.SaveAll}}},
	// Test --raw-out flag
	{"ttyS0", []string{"--raw-out", "obs"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgObs)}}},
	{"ttyS0", []string{"--raw-out", "nav"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "obs,nav"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "nav,obs"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)}}},
	{"ttyS0", []string{"--raw-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgFlags(0))}}},
	{"ttyS0", []string{"--raw-out", "obs", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgObs), Save: gpsprot.SaveMinimal}}},
	// Test --pvt-out flag
	{"ttyS0", []string{"--pvt-out", "daemon"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: daemon.PVTMsgFlags | gpsprot.PVTMsgOff}}},
	{"ttyS0", []string{"--pvt-out", "pos"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos}}},
	{"ttyS0", []string{"--pvt-out", "vel"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgVel}}},
	{"ttyS0", []string{"--pvt-out", "time"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTime}}},
	{"ttyS0", []string{"--pvt-out", "tp"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTimePulse}}},
	{"ttyS0", []string{"--pvt-out", "leap"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgLeapSecond}}},
	{"ttyS0", []string{"--pvt-out", "pos,time"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime}}},
	{"ttyS0", []string{"--pvt-out", "pos,vel"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgVel}}},
	{"ttyS0", []string{"--pvt-out", "tp,tai,leap"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI | gpsprot.PVTMsgLeapSecond}}},
	{"ttyS0", []string{"--pvt-out", "survey"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgSurvey}}},
	{"ttyS0", []string{"--pvt-out", "qual"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgQuality}}},
	{"ttyS0", []string{"--pvt-out", "pos,qual"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgQuality}}},
	{"ttyS0", []string{"--pvt-out", "epoch"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgEpoch}}},
	{"ttyS0", []string{"--pvt-out", "pos,epoch"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos | gpsprot.PVTMsgEpoch}}},
	{"ttyS0", []string{"--pvt-out", "off"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgOff}}},
	{"ttyS0", []string{"--pvt-out", "pos", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{PVTMsg: gpsprot.PVTMsgPos, Save: gpsprot.SaveMinimal}}},
	// Test combining --raw-out and --pvt-out
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos"}, flagVars{configOpts: gpsprot.ConfigOptions{RawMsg: opt.Make(gpsprot.RawMsgObs), PVTMsg: gpsprot.PVTMsgPos}}},
	// Test --rtcm-out flag
	{"ttyS0", []string{"--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM7"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM7)}}},
	{"ttyS0", []string{"--rtcm-out", "ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM4,ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "MSM7,ARP"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP)}}},
	{"ttyS0", []string{"--rtcm-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgFlags(0))}}},
	{"ttyS0", []string{"--rtcm-out", "MSM4", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4), Save: gpsprot.SaveMinimal}}},
	// Test case-insensitive RTCM flags
	{"ttyS0", []string{"--rtcm-out", "msm4"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4)}}},
	{"ttyS0", []string{"--rtcm-out", "Msm4,Arp"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)}}},
	// Test auto flag functionality
	{"ttyS0", []string{"--rtcm-out", "auto"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	{"ttyS0", []string{"--rtcm-out", "auto,msm7"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	{"ttyS0", []string{"--rtcm-out", "msm7,auto"}, flagVars{configOpts: gpsprot.ConfigOptions{RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM7 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgLax)}}},
	// Test combining multiple output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{
		RawMsg:  opt.Make(gpsprot.RawMsgObs),
		PVTMsg:  gpsprot.PVTMsgPos,
		RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4),
	}}},
	// Test --nmea-out flag
	{"ttyS0", []string{"--nmea-out", "RMC"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC)}}},
	{"ttyS0", []string{"--nmea-out", "GGA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgGGA)}}},
	{"ttyS0", []string{"--nmea-out", "GSA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgGSA)}}},
	{"ttyS0", []string{"--nmea-out", "GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "ZDA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgZDA)}}},
	{"ttyS0", []string{"--nmea-out", "GLL"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgGLL)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GSA,GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "RMC,GGA,GSA,GSV"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV)}}},
	{"ttyS0", []string{"--nmea-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgFlags(0))}}},
	{"ttyS0", []string{"--nmea-out", "RMC", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC), Save: gpsprot.SaveMinimal}}},
	// Test case-insensitive NMEA flags
	{"ttyS0", []string{"--nmea-out", "rmc"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC)}}},
	{"ttyS0", []string{"--nmea-out", "Rmc,Gga"}, flagVars{configOpts: gpsprot.ConfigOptions{NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)}}},
	// Test --sats-out flag
	{"ttyS0", []string{"--sats-out", "sat"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat)}}},
	{"ttyS0", []string{"--sats-out", "sig"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSignal)}}},
	{"ttyS0", []string{"--sats-out", "sat,sig"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)}}},
	{"ttyS0", []string{"--sats-out", "sig,sat"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)}}},
	{"ttyS0", []string{"--sats-out", "none"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgFlags(0))}}},
	{"ttyS0", []string{"--sats-out", "sat", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat), Save: gpsprot.SaveMinimal}}},
	// Test case-insensitive sats flags
	{"ttyS0", []string{"--sats-out", "SAT"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat)}}},
	{"ttyS0", []string{"--sats-out", "Sig"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSignal)}}},
	{"ttyS0", []string{"--sats-out", "Sat,Sig"}, flagVars{configOpts: gpsprot.ConfigOptions{SatsMsg: opt.Make(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)}}},
	// Test combining all output flags
	{"ttyS0", []string{"--raw-out", "obs", "--pvt-out", "pos", "--rtcm-out", "MSM4", "--nmea-out", "RMC", "--sats-out", "sat"}, flagVars{configOpts: gpsprot.ConfigOptions{
		RawMsg:  opt.Make(gpsprot.RawMsgObs),
		PVTMsg:  gpsprot.PVTMsgPos,
		RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4),
		NMEAMsg: opt.Make(gpsprot.NMEAMsgRMC),
		SatsMsg: opt.Make(gpsprot.SatsMsgSat),
	}}},
	// Test --binary flag
	{"ttyS0", []string{"--binary"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
	}}},
	{"ttyS0", []string{"--binary", "--save"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
		Save:    gpsprot.SaveMinimal,
	}}},
	// Test --binary with explicit --pvt-out (should use the explicit value)
	{"ttyS0", []string{"--binary", "--pvt-out", "tp,leap"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgNone),
		PVTMsg:  gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgLeapSecond,
	}}},
	// Test --binary with --rtcm-out (should not set default pvt)
	{"ttyS0", []string{"--binary", "--rtcm-out", "MSM4"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgNone),
		RTCMMsg: opt.Make(gpsprot.RTCMMsgMSM4),
	}}},
	// Test --nmea with explicit --nmea-out
	{"ttyS0", []string{"--nmea", "--nmea-out", "GGA,GSA"}, flagVars{configOpts: gpsprot.ConfigOptions{
		NMEAMsg: opt.Make(gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA),
		RTCMMsg: opt.Make(gpsprot.RTCMMsgNone),
		PVTMsg:  gpsprot.PVTMsgOff,
		RawMsg:  opt.Make(gpsprot.RawMsgNone),
		SatsMsg: opt.Make(gpsprot.SatsMsgNone),
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
		mode: opt.Make(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
			FixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
		}),
	}},
	// Test --fixed-pos-ecef with custom accuracy (Eiffel Tower)
	{"ttyS0", []string{"--fixed-pos-ecef", "4200935.82,168323.10,4780213.04", "--fixed-pos-acc", "5.0"}, flagVars{
		mode: opt.Make(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(4200935.82), gpsprot.Meters(168323.10), gpsprot.Meters(4780213.04)},
			FixedPosAcc:  gpsprot.Meters(5.0),
		}),
	}},
	// Test --fixed-pos-ecef with save (Statue of Liberty)
	{"ttyS0", []string{"--fixed-pos-ecef", "1331340.65,-4656583.35,4136313.40", "--save"}, flagVars{
		mode: opt.Make(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(1331340.65), gpsprot.Meters(-4656583.35), gpsprot.Meters(4136313.40)},
			FixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
		}),
		configOpts: gpsprot.ConfigOptions{Save: gpsprot.SaveMinimal},
	}},
	// Test --fixed-pos-ecef with spaces around commas (Mount Everest)
	{"ttyS0", []string{"--fixed-pos-ecef", "302769.89, 5636025.47, 2979493.09"}, flagVars{
		mode: opt.Make(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(302769.89), gpsprot.Meters(5636025.47), gpsprot.Meters(2979493.09)},
			FixedPosAcc:  gpsprot.Meters(defaultFixedPosAcc),
		}),
	}},
	// Test --fixed-pos-ecef with minimum accuracy
	{"ttyS0", []string{"--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0.001"}, flagVars{
		mode: opt.Make(gpsprot.Mode{
			Static:       true,
			PosType:      gpsprot.PosTypeECEF,
			FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(3978578.17), gpsprot.Meters(-8652.15), gpsprot.Meters(4968410.94)},
			FixedPosAcc:  gpsprot.Meters(0.001),
		}),
	}},
	// Test --ant-cable-delay flag
	{"ttyS0", []string{"--ant-cable-delay", "100"}, flagVars{antCableDelay: opt.Make(100 * time.Nanosecond)}},
	{"ttyS0", []string{"--ant-cable-delay", "0"}, flagVars{antCableDelay: opt.Make(time.Duration(0))}},
	{"ttyS0", []string{"--ant-cable-delay", "1000000"}, flagVars{antCableDelay: opt.Make(1000000 * time.Nanosecond)}},
	{"ttyS0", []string{"--ant-cable-delay", "-50"}, flagVars{antCableDelay: opt.Make(-50 * time.Nanosecond)}},
	// Test --show-config flag
	{"ttyS0", []string{"--show-config"}, flagVars{configGet: showProps}},
	{"ttyS0", []string{"-c"}, flagVars{configGet: showProps}},
	{"", []string{"--socket", "/tmp/socket", "--show-config"}, flagVars{socketPath: "/tmp/socket", configGet: showProps}},
	{"", []string{"--socket", "/tmp/socket", "-c"}, flagVars{socketPath: "/tmp/socket", configGet: showProps}},
	// Test --msg-file with --tag flag
	{"ttyS0", []string{"--msg-file", "test.toml"}, flagVars{msgFilePath: "test.toml", msgTags: []string{""}, capture: opt.Make(2 * time.Second)}},
	{"ttyS0", []string{"--msg-file", "test.toml", "--tag", "setup"}, flagVars{msgFilePath: "test.toml", msgTags: []string{"setup"}, capture: opt.Make(2 * time.Second)}},
	{"ttyS0", []string{"--msg-file", "test.toml", "-t", "setup"}, flagVars{msgFilePath: "test.toml", msgTags: []string{"setup"}, capture: opt.Make(2 * time.Second)}},
	{"ttyS0", []string{"--msg-file", "test.toml", "--tag", "setup,ppp"}, flagVars{msgFilePath: "test.toml", msgTags: []string{"setup", "ppp"}, capture: opt.Make(2 * time.Second)}},
	{"ttyS0", []string{"--msg-file", "test.toml", "--tag", "foo,,bar"}, flagVars{msgFilePath: "test.toml", msgTags: []string{"foo", "", "bar"}, capture: opt.Make(2 * time.Second)}},
	{"ttyS0", []string{"--msg-file", "test.toml", "--tag", ""}, flagVars{msgFilePath: "test.toml", msgTags: []string{""}, capture: opt.Make(2 * time.Second)}},
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
			} else if !reflect.DeepEqual(*vars, expect) {
				t.Errorf("parseFlags returned %+v, expected %+v", *vars, expect)
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
	{"--serial-device", "ttyS0", "--speed", "9600", "--pps", "1.5"},
	{"--serial-device", "ttyS0", "--speed", "9600", "--pps"},
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
	{"--serial-device", "ttyS0", "--sats-out", ""},                             // empty sats-out value
	{"--serial-device", "ttyS0", "--sats-out", "invalid"},                      // invalid sats-out flag
	{"--serial-device", "ttyS0", "--sats-out", "sat,invalid"},                  // partially invalid sats-out flags
	// Test invalid combinations with --nmea and --binary
	{"--serial-device", "ttyS0", "--nmea", "--binary"},            // can't use both --nmea and --binary
	{"--serial-device", "ttyS0", "--binary", "--nmea"},            // can't use both --binary and --nmea
	{"--serial-device", "ttyS0", "--nmea", "--rtcm-out", "MSM4"},  // can't use --nmea with --rtcm-out
	{"--serial-device", "ttyS0", "--nmea", "--pvt-out", "pos"},    // can't use --nmea with --pvt-out
	{"--serial-device", "ttyS0", "--nmea", "--raw-out", "obs"},    // can't use --nmea with --raw-out
	{"--serial-device", "ttyS0", "--nmea", "--sats-out", "sat"},   // can't use --nmea with --sats-out
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
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", ""},                        // empty value
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,2"},                     // too few coordinates
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,2,3,4"},                 // too many coordinates
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "abc,def,ghi"},             // invalid numbers
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "1,abc,3"},                 // partially invalid numbers
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "7000000,0,0"},             // coordinates too far from Earth
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "0,0,0"},                   // center of Earth (invalid)
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "8000000,8000000,8000000"}, // far outside valid range
	// Test invalid --fixed-pos-acc values
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0"},      // accuracy too small
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--fixed-pos-acc", "0.0005"}, // accuracy below minimum
	// Test mutual exclusion with --mobile and --survey
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--mobile"}, // can't use with mobile
	{"--serial-device", "ttyS0", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94", "--survey"}, // can't use with survey
	{"--serial-device", "ttyS0", "--mobile", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"}, // can't use mobile with fixed-pos
	{"--serial-device", "ttyS0", "--survey", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"}, // can't use survey with fixed-pos
	// Test invalid --ant-cable-delay values
	{"--serial-device", "ttyS0", "--ant-cable-delay", "abc"}, // invalid number
	{"--serial-device", "ttyS0", "--ant-cable-delay", "1.5"}, // floating point not allowed
	{"--serial-device", "ttyS0", "--ant-cable-delay", ""},    // empty value
	// Test invalid --show-config combinations
	{"--serial-device", "ttyS0", "--show-config", "--factory-reset"},                  // can't use show-config with factory-reset
	{"--serial-device", "ttyS0", "--show-config", "--reset"},                          // can't use show-config with reset
	{"--serial-device", "ttyS0", "--show-config", "--reload"},                         // can't use show-config with reload
	{"--serial-device", "ttyS0", "--factory-reset", "--show-config"},                  // can't use factory-reset with show-config
	{"--serial-device", "ttyS0", "--reset", "--show-config"},                          // can't use reset with show-config
	{"--serial-device", "ttyS0", "--reload", "--show-config"},                         // can't use reload with show-config
	{"--serial-device", "ttyS0", "-c", "--factory-reset"},                             // can't use short form with factory-reset
	{"--serial-device", "ttyS0", "-c", "--reset"},                                     // can't use short form with reset
	{"--serial-device", "ttyS0", "-c", "--reload"},                                    // can't use short form with reload
	{"--serial-device", "ttyS0", "--factory-reset", "-c"},                             // can't use factory-reset with short form
	{"--serial-device", "ttyS0", "--reset", "-c"},                                     // can't use reset with short form
	{"--serial-device", "ttyS0", "--reload", "-c"},                                    // can't use reload with short form
	{"--serial-device", "ttyS0", "--show-config", "--factory-reset", "--gnss", "GPS"}, // multiple incompatible options
	// Test --msg-file mutual exclusivity with config flags
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--gnss", "GPS"},                                      // can't use with --gnss
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--pps", "0.1"},                                       // can't use with --pps
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--save"},                                             // can't use with --save
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--save-all"},                                         // can't use with --save-all
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--reset"},                                            // can't use with --reset
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--reload"},                                           // can't use with --reload
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--factory-reset"},                                    // can't use with --factory-reset
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--show-config"},                                      // can't use with --show-config
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--nmea"},                                             // can't use with --nmea
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--binary"},                                           // can't use with --binary
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--mobile"},                                           // can't use with --mobile
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--survey"},                                           // can't use with --survey
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--time-gnss", "GPS"},                                 // can't use with --time-gnss
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--pvt-out", "pos"},                                   // can't use with --pvt-out
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--raw-out", "obs"},                                   // can't use with --raw-out
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--rtcm-out", "MSM4"},                                 // can't use with --rtcm-out
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--nmea-out", "RMC"},                                  // can't use with --nmea-out
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--sats-out", "sat"},                                  // can't use with --sats-out
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--ant-cable-delay", "100"},                           // can't use with --ant-cable-delay
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--speed", "9600"},                                    // can't use with --speed
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--fixed-pos-ecef", "3978578.17,-8652.15,4968410.94"}, // can't use with --fixed-pos-ecef
	// Test --msg-file cannot be combined with --test-log
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--test-log", "test.log", "--packet-log", "pkt.log"}, // can't use with --test-log
	// Test --tag requires --msg-file
	{"--serial-device", "ttyS0", "--tag", "setup"}, // --tag without --msg-file
	// Test --msg-file cannot be combined with --show-receiver
	{"--serial-device", "ttyS0", "--msg-file", "test.toml", "--show-receiver"}, // can't use with --show-receiver
	// Test --config-file mutual exclusivity with --serial-device and --device-speed
	{"--config-file", "test.toml", "--serial-device", "ttyS0"},  // can't use with --serial-device
	{"--config-file", "test.toml", "--device-speed", "9600"},    // can't use with --device-speed
	{"-f", "test.toml", "-d", "ttyS0"},                         // short forms
	{"-f", "test.toml", "-s", "9600"},                           // short forms
	{"--config-file", "/nonexistent/path/satpulse.toml"},        // file does not exist
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "satpulse-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestParseFlagsConfigFile(t *testing.T) {
	t.Run("device and speed", func(t *testing.T) {
		path := writeTestConfig(t, "[serial]\ndevice = \"/dev/ttyUSB0\"\nspeed = 9600\n")
		vars, _, err := parseFlags("config", []string{"-f", path})
		if err != nil {
			t.Fatalf("parseFlags returned error: %v", err)
		}
		if vars.serialDevice != "/dev/ttyUSB0" {
			t.Errorf("serialDevice = %q, want %q", vars.serialDevice, "/dev/ttyUSB0")
		}
		if vars.localSpeed != 9600 {
			t.Errorf("localSpeed = %d, want %d", vars.localSpeed, 9600)
		}
	})
	t.Run("ignores other keys", func(t *testing.T) {
		path := writeTestConfig(t, "[serial]\ndevice = \"/dev/ttyACM0\"\nspeed = 38400\n[phc]\ninterface = \"enp1s0\"\n[gps]\nconfig = true\n")
		vars, _, err := parseFlags("config", []string{"-f", path})
		if err != nil {
			t.Fatalf("parseFlags returned error: %v", err)
		}
		if vars.serialDevice != "/dev/ttyACM0" {
			t.Errorf("serialDevice = %q, want %q", vars.serialDevice, "/dev/ttyACM0")
		}
		if vars.localSpeed != 38400 {
			t.Errorf("localSpeed = %d, want %d", vars.localSpeed, 38400)
		}
	})
	t.Run("missing speed", func(t *testing.T) {
		path := writeTestConfig(t, "[serial]\ndevice = \"/dev/ttyUSB0\"\n")
		vars, _, err := parseFlags("config", []string{"-f", path})
		if err == nil {
			t.Errorf("parseFlags returned nil error")
		}
		if vars != nil {
			t.Errorf("parseFlags returned non-nil vars")
		}
	})
	t.Run("missing device", func(t *testing.T) {
		path := writeTestConfig(t, "[serial]\nspeed = 9600\n")
		vars, _, err := parseFlags("config", []string{"-f", path})
		if err == nil {
			t.Errorf("parseFlags returned nil error")
		}
		if vars != nil {
			t.Errorf("parseFlags returned non-nil vars")
		}
	})
	t.Run("empty file", func(t *testing.T) {
		path := writeTestConfig(t, "")
		vars, _, err := parseFlags("config", []string{"-f", path})
		if err == nil {
			t.Errorf("parseFlags returned nil error")
		}
		if vars != nil {
			t.Errorf("parseFlags returned non-nil vars")
		}
	})
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
