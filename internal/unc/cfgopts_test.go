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
			name: "enable RECTIMEB for PVTMsgTimePulse",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTimePulse
			},
			expectedCmds: []string{
				"RECTIMEB 1", // RECTIMEB is used for TimePulse
			},
		},
		{
			name: "enable multiple PVT messages",
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse
			},
			expectedCmds: []string{
				"RECTIMEB 1",
			},
		},
		{
			name: "disable messages with PVTMsgOff",
			currentState: []string{
				"RECTIMEB 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgOff
			},
			expectedCmds: []string{
				"UNLOG RECTIMEB",
			},
		},
		{
			name: "keep needed message with PVTMsgOff",
			currentState: []string{
				"RECTIMEB 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgOff
			},
			expectedCmds: []string{
				"RECTIMEB 1",
			},
		},
		{
			name: "enable UTCB messages for all GNSS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Has GPS, GLO, GAL, BDS, QZSS
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgLeapSecond
			},
			expectedCmds: []string{
				"GPSUTCB 1",
				"BD3UTCB 1",
				"GALUTCB 1",
			},
		},
		{
			name: "enable UTCB messages for GPS only",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS
				"MASK GAL",
				"MASK BDS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgLeapSecond
			},
			expectedCmds: []string{
				"GPSUTCB 1",
			},
		},
		{
			name: "enable UTCB messages for GPS and BDS",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS
				"MASK GAL",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgLeapSecond
			},
			expectedCmds: []string{
				"GPSUTCB 1",
				"BD3UTCB 1",
			},
		},
		{
			name: "disable UTCB messages with PVTMsgOff",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Has GPS, GLO, GAL, BDS, QZSS
				"RECTIMEB 1",
				"GPSUTCB 1",
				"BD3UTCB 1",
				"GALUTCB 1",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgOff
			},
			expectedCmds: []string{
				"UNLOG RECTIMEB",
				"UNLOG GPSUTCB",
				"UNLOG BD3UTCB",
				"UNLOG GALUTCB",
			},
		},
		{
			name: "no UTCB messages when no GNSS enabled",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
				"MASK GPS",
				"MASK GAL",
				"MASK BDS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgLeapSecond
			},
			expectedCmds: []string{
				// No UTCB messages
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
			name: "enable RTCM MSM4 messages with all GNSS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Has GPS, GLO, GAL, BDS, QZSS
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			},
			expectedCmds: []string{
				"RTCM1074 1", // GPS MSM4
				"RTCM1084 1", // GLO MSM4
				"RTCM1094 1", // GAL MSM4
				"RTCM1114 1", // QZSS MSM4
				"RTCM1124 1", // BDS MSM4
				"RTCM1230 1", // GLO bias (added when GLO enabled)
			},
		},
		{
			name: "enable RTCM MSM4 with GPS only",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS (no GLO)
				"MASK GAL",
				"MASK BDS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			},
			expectedCmds: []string{
				"RTCM1074 1", // Only GPS MSM4
			},
		},
		{
			name: "enable RTCM MSM4 with GPS and GLO",
			currentState: []string{
				"CONFIG SIGNALGROUP 1", // Has GPS, GLO, GAL, BDS, QZSS
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			},
			expectedCmds: []string{
				"RTCM1074 1", // GPS MSM4
				"RTCM1084 1", // GLO MSM4
				"RTCM1230 1", // GLO bias
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
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Has GPS, GLO, GAL, BDS, QZSS
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)
			},
			expectedCmds: []string{
				"RTCM1074 1", // GPS MSM4
				"RTCM1084 1", // GLO MSM4
				"RTCM1094 1", // GAL MSM4
				"RTCM1114 1", // QZSS MSM4
				"RTCM1124 1", // BDS MSM4
				"RTCM1230 1", // GLO bias
				"RTCM1005 1", // ARP
			},
		},
		{
			name: "enable RTCM MSM7 messages with GPS and BDS",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS (no GLO)
				"MASK GAL",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM7)
			},
			expectedCmds: []string{
				"RTCM1077 1", // GPS MSM7
				"RTCM1127 1", // BDS MSM7
			},
		},
		{
			name: "no RTCM messages when no GNSS enabled",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
				"MASK GPS",
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			},
			expectedCmds: []string{
				// No RTCM messages
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
			name: "enable raw navigation data for all GNSS",
			currentState: []string{
				"CONFIG SIGNALGROUP 2", // Has GPS, BDS, GLO, GAL, QZSS, NAVIC
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
				// Note: No NAVICEPHB as Unicore doesn't support it
			},
		},
		{
			name: "enable raw navigation data for GPS only",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS (no GLO)
				"MASK GAL",
				"MASK BDS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RawMsg.Set(gpsprot.RawMsgNavData)
			},
			expectedCmds: []string{
				"GPSEPHB 1", // Only GPS ephemeris
			},
		},
		{
			name: "enable raw navigation data for GPS and BDS",
			currentState: []string{
				"CONFIG SIGNALGROUP 8", // Has GPS, GAL, BDS
				"MASK GAL",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RawMsg.Set(gpsprot.RawMsgNavData)
			},
			expectedCmds: []string{
				"GPSEPHB 1",
				"BDSEPHB 1",
			},
		},
		{
			name: "no raw navigation data when no GNSS enabled",
			currentState: []string{
				"CONFIG SIGNALGROUP 1",
				"MASK GPS",
				"MASK GLO",
				"MASK GAL",
				"MASK BDS",
				"MASK QZSS",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.RawMsg.Set(gpsprot.RawMsgNavData)
			},
			expectedCmds: []string{
				// No ephemeris messages
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
				"SATSINFOB 1",
				"GPGGA 1",
			},
		},
		{
			name: "enable messages with save",
			currentState: []string{
				"CONFIG SIGNALGROUP 2",
			},
			targetOpts: func(opts *gpsprot.ConfigOptions) {
				opts.PVTMsg = gpsprot.PVTMsgTime
				opts.RTCMMsg.Set(gpsprot.RTCMMsgARP)
				opts.Save = gpsprot.SaveMinimal
			},
			expectedCmds: []string{
				"RECTIMEB 1",
				"RTCM1005 1",
				"SAVECONFIG",
			},
		},
	}

	testNativeConfigProps(t, tests)
}