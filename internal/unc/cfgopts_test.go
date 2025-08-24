package unc

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestPVTMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable RECTIMEB for PVTMsgTime",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime
			},
			expectedCmds: []string{
				"RECTIMEB 1",
			},
		},
		{
			name: "enable PPSSTATUS for PVTMsgTimePulse",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTimePulse
			},
			expectedCmds: []string{
				"RECTIMEB 1", // RECTIMEB is also needed when PVTMsgTimePulse is on
				"PPSSTATUS 1",
			},
		},
		{
			name: "enable multiple PVT messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse
			},
			expectedCmds: []string{
				"RECTIMEB 1",
				"PPSSTATUS 1",
			},
		},
		{
			name: "disable messages with PVTMsgOff",
			currentState: []string{
				"RECTIMEB 1",
				"PPSSTATUS 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgOff
			},
			expectedCmds: []string{
				"UNLOG RECTIMEB",
				"UNLOG PPSSTATUS",
			},
		},
		{
			name: "keep needed message with PVTMsgOff",
			currentState: []string{
				"RECTIMEB 1",
				"PPSSTATUS 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgOff
			},
			expectedCmds: []string{
				"RECTIMEB 1",
				"UNLOG PPSSTATUS",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestSatsMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable SATSINFOB for SatsMsgSat",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.SatsMsg.Set(gpsprot.SatsMsgSat)
			},
			expectedCmds: []string{
				"SATSINFOB 1",
			},
		},
		{
			name: "enable SATSINFOB for SatsMsgSignal",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.SatsMsg.Set(gpsprot.SatsMsgSignal)
			},
			expectedCmds: []string{
				"SATSINFOB 1",
			},
		},
		{
			name: "enable SATSINFOB for SatsMsgAny",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.SatsMsg.Set(gpsprot.SatsMsgAny)
			},
			expectedCmds: []string{
				"SATSINFOB 1",
			},
		},
		{
			name: "disable SATSINFOB",
			currentState: []string{
				"SATSINFOB 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.SatsMsg.Set(0)
			},
			expectedCmds: []string{
				"UNLOG SATSINFOB",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestNMEAMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable RMC and GGA",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
			},
			expectedCmds: []string{
				"GPRMC 1",
				"GPGGA 1",
			},
		},
		{
			name: "enable all standard NMEA messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA | gpsprot.NMEAMsgGSA | gpsprot.NMEAMsgGSV | gpsprot.NMEAMsgZDA | gpsprot.NMEAMsgVTG)
			},
			expectedCmds: []string{
				"GPRMC 1",
				"GPGGA 1",
				"GPGSA 1",
				"GPGSV 1",
				"GPZDA 1",
				"GPVTG 1",
			},
		},
		{
			name: "disable NMEA messages",
			currentState: []string{
				"GPRMC 1",
				"GPGGA 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.NMEAMsg.Set(0)
			},
			expectedCmds: []string{
				"UNLOG GPRMC",
				"UNLOG GPGGA",
				"UNLOG GPGSA",
				"UNLOG GPGSV",
				"UNLOG GPZDA",
				"UNLOG GPVTG",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestRTCMMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable RTCM MSM4 messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			},
			expectedCmds: []string{
				"RTCM1074 1",
				"RTCM1084 1",
				"RTCM1124 1",
			},
		},
		{
			name: "enable RTCM ARP message",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgARP)
			},
			expectedCmds: []string{
				"RTCM1005 1",
			},
		},
		{
			name: "enable RTCM MSM4 and ARP",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)
			},
			expectedCmds: []string{
				"RTCM1074 1",
				"RTCM1084 1",
				"RTCM1124 1",
				"RTCM1005 1",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestRawMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable raw observation messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RawMsg.Set(gpsprot.RawMsgObs)
			},
			expectedCmds: []string{
				"OBSVMB 1",
			},
		},
		{
			name: "enable raw navigation data for enabled GNSS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Signal group 2 includes GPS, BDS, GLO, GAL, QZSS
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RawMsg.Set(gpsprot.RawMsgNavData)
			},
			expectedCmds: []string{
				"GPSEPHB 1",
				"BDSEPHB 1",
				"GLOEPHB 1",
				"GALEPHB 1",
				"QZSSEPHB 1",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestSaveReset(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "save configuration",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.Save = gpsprot.SaveMinimal
			},
			expectedCmds: []string{
				"SAVECONFIG",
			},
		},
		{
			name: "save all configuration",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.Save = gpsprot.SaveAll
			},
			expectedCmds: []string{
				"SAVECONFIG",
			},
		},
		{
			name: "reload configuration",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.Reset = gpsprot.ResetReload
			},
			expectedCmds: []string{
				"RESET",
			},
		},
		{
			name: "cold reset",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.Reset = gpsprot.ResetCold
			},
			expectedCmds: []string{
				"RESET ALL",
			},
		},
		{
			name: "factory reset",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.Reset = gpsprot.ResetFactory
			},
			expectedCmds: []string{
				"FRESET",
			},
		},
	}

	testNativeConfigProps(t, tests)
}

func TestCombinedMessages(t *testing.T) {
	tests := []nativeConfigPropsTestCase{
		{
			name: "enable PVT and Sats messages together",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime
				opts.SatsMsg.Set(gpsprot.SatsMsgSat)
			},
			expectedCmds: []string{
				"RECTIMEB 1",
				"SATSINFOB 1",
			},
		},
		{
			name: "enable PVT, Sats and NMEA messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse
				opts.SatsMsg.Set(gpsprot.SatsMsgSat)
				opts.NMEAMsg.Set(gpsprot.NMEAMsgGGA)
			},
			expectedCmds: []string{
				"RECTIMEB 1",
				"PPSSTATUS 1",
				"SATSINFOB 1",
				"GPGGA 1",
			},
		},
	}

	testNativeConfigProps(t, tests)
}