package unc

import "github.com/jclark/satpulse/internal/gpsprot"

// generateOptsCommands generates native commands from ConfigOptions
func generateOptsCommands(opts *gpsprot.ConfigOptions) []nativeCommand {
	strs := append(generateMsgCommands(opts), generateSaveResetCommands(opts)...)
	cmds := make([]nativeCommand, len(strs))
	for i, cmd := range strs {
		cmds[i] = nativeCommand{cmd: cmd}
	}
	return cmds
}

// generateMsgCommands generates all message enable/disable commands from ConfigOptions
func generateMsgCommands(opts *gpsprot.ConfigOptions) []string {
	var cmds []string
	if opts.PVTMsg.IsSet() {
		cmds = append(cmds, generatePVTMsgCommands(opts.PVTMsg.Get())...)
	}
	if opts.SatsMsg.IsSet() {
		cmds = append(cmds, generateSatsMsgCommands(opts.SatsMsg.Get())...)
	}
	if opts.NMEAMsg.IsSet() {
		cmds = append(cmds, generateNMEAMsgCommands(opts.NMEAMsg.Get())...)
	}
	if opts.RTCMMsg.IsSet() {
		cmds = append(cmds, generateRTCMMsgCommands(opts.RTCMMsg.Get())...)
	}
	if opts.RawMsg.IsSet() {
		cmds = append(cmds, generateRawMsgCommands(opts.RawMsg.Get())...)
	}
	return cmds
}

// generatePVTMsgCommands maps PVT flags to Unicore message commands
func generatePVTMsgCommands(flags gpsprot.PVTMsgFlags) []string {
	var cmds []string
	off := flags&gpsprot.PVTMsgOff != 0
	// RECTIMEB is needed for both Time and TimePulse
	if flags&(gpsprot.PVTMsgTime|gpsprot.PVTMsgTimePulse) != 0 {
		cmds = append(cmds, "RECTIMEB 1")
	} else if off {
		cmds = append(cmds, "UNLOG RECTIMEB")
	}
	// PPSSTATUS for TimePulse
	if flags&gpsprot.PVTMsgTimePulse != 0 {
		cmds = append(cmds, "PPSSTATUS 1")
	} else if off {
		cmds = append(cmds, "UNLOG PPSSTATUS")
	}
	// Future: Add leap second messages when LeapSecondMsg is implemented
	// if flags&gpsprot.PVTMsgLeapSecond != 0 {
	//     cmds = append(cmds, "GPSUTCB 1", "BD3UTCB 1", "GALUTCB 1")
	// }
	return cmds
}

// generateSatsMsgCommands maps Sats flags to Unicore message commands
func generateSatsMsgCommands(flags gpsprot.SatsMsgFlags) []string {
	var cmds []string
	// SATSINFOB provides both satellite and signal info
	if flags&gpsprot.SatsMsgAny != 0 {
		cmds = append(cmds, "SATSINFOB 1")
	} else {
		cmds = append(cmds, "UNLOG SATSINFOB")
	}
	return cmds
}

// generateNMEAMsgCommands maps NMEA flags to Unicore NMEA commands
func generateNMEAMsgCommands(flags gpsprot.NMEAMsgFlags) []string {
	var cmds []string
	msgs := []struct {
		flag gpsprot.NMEAMsgFlags
		cmd  string
	}{
		{gpsprot.NMEAMsgRMC, "GPRMC"},
		{gpsprot.NMEAMsgGGA, "GPGGA"},
		{gpsprot.NMEAMsgGSA, "GPGSA"},
		{gpsprot.NMEAMsgGSV, "GPGSV"},
		{gpsprot.NMEAMsgZDA, "GPZDA"},
		{gpsprot.NMEAMsgVTG, "GPVTG"},
	}
	// If flags is 0, we want to disable all NMEA messages
	if flags == 0 {
		for _, m := range msgs {
			cmds = append(cmds, "UNLOG "+m.cmd)
		}
	} else {
		// Enable specific messages that are set
		for _, m := range msgs {
			if flags&m.flag != 0 {
				cmds = append(cmds, m.cmd+" 1")
			}
		}
	}
	return cmds
}

// generateRTCMMsgCommands maps RTCM flags to Unicore RTCM commands
func generateRTCMMsgCommands(flags gpsprot.RTCMMsgFlags) []string {
	var cmds []string
	if flags&gpsprot.RTCMMsgMSM4 != 0 {
		cmds = append(cmds, "RTCM1074 1") // GPS MSM4
		cmds = append(cmds, "RTCM1084 1") // GLONASS MSM4
		cmds = append(cmds, "RTCM1124 1") // BDS MSM4
		// Could add more based on enabled GNSS
	}
	if flags&gpsprot.RTCMMsgMSM7 != 0 {
		cmds = append(cmds, "RTCM1077 1") // GPS MSM7
		cmds = append(cmds, "RTCM1087 1") // GLONASS MSM7
		cmds = append(cmds, "RTCM1127 1") // BDS MSM7
	}
	if flags&gpsprot.RTCMMsgARP != 0 {
		cmds = append(cmds, "RTCM1005 1")
	}
	return cmds
}

// generateRawMsgCommands maps Raw flags to Unicore raw data commands
func generateRawMsgCommands(flags gpsprot.RawMsgFlags) []string {
	var cmds []string
	if flags&gpsprot.RawMsgObs != 0 {
		cmds = append(cmds, "OBSVMB 1")
	}
	if flags&gpsprot.RawMsgNavData != 0 {
		// Enable ephemeris for all enabled GNSS
		// TODO: Check signalGroup to determine which GNSS are actually enabled
		cmds = append(cmds, "GPSEPHB 1")
		cmds = append(cmds, "BDSEPHB 1")
		cmds = append(cmds, "GLOEPHB 1")
		cmds = append(cmds, "GALEPHB 1")
		cmds = append(cmds, "QZSSEPHB 1")
	}
	return cmds
}

// generateSaveResetCommands generates save and reset commands
func generateSaveResetCommands(opts *gpsprot.ConfigOptions) []string {
	var cmds []string
	if opts.Save != gpsprot.SaveNone {
		cmds = append(cmds, "SAVECONFIG")
	}
	switch opts.Reset {
	case gpsprot.ResetReload:
		cmds = append(cmds, "RESET")
	case gpsprot.ResetCold:
		cmds = append(cmds, "RESET ALL")
	case gpsprot.ResetFactory:
		cmds = append(cmds, "FRESET")
	}
	return cmds
}