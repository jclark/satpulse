package unc

import "github.com/jclark/satpulse/internal/gpsprot"

// generateOptsCommands generates native commands from ConfigOptions
func generateOptsCommands(opts *gpsprot.ConfigOptions, enabledGNSS gpsprot.GNSSSet) []nativeCommand {
	strs := append(generateMsgCommands(opts, enabledGNSS), generateSaveResetCommands(opts)...)
	cmds := make([]nativeCommand, len(strs))
	for i, cmd := range strs {
		cmds[i] = nativeCommand{cmd: cmd}
	}
	return cmds
}

// generateMsgCommands generates all message enable/disable commands from ConfigOptions
func generateMsgCommands(opts *gpsprot.ConfigOptions, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	if opts.PVTMsg.IsSet() {
		cmds = append(cmds, generatePVTMsgCommands(opts.PVTMsg.Get(), enabledGNSS)...)
	}
	if opts.SatsMsg.IsSet() {
		cmds = append(cmds, generateSatsMsgCommands(opts.SatsMsg.Get())...)
	}
	if opts.NMEAMsg.IsSet() {
		cmds = append(cmds, generateNMEAMsgCommands(opts.NMEAMsg.Get())...)
	}
	if opts.RTCMMsg.IsSet() {
		cmds = append(cmds, generateRTCMMsgCommands(opts.RTCMMsg.Get(), enabledGNSS)...)
	}
	if opts.RawMsg.IsSet() {
		cmds = append(cmds, generateRawMsgCommands(opts.RawMsg.Get(), enabledGNSS)...)
	}
	return cmds
}

// generatePVTMsgCommands maps PVT flags to Unicore message commands
func generatePVTMsgCommands(flags gpsprot.PVTMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	off := flags&gpsprot.PVTMsgOff != 0
	// RECTIMEB is used for Time and TimePulse
	if flags&(gpsprot.PVTMsgTime|gpsprot.PVTMsgTimePulse) != 0 {
		cmds = append(cmds, "RECTIMEB 1")
	} else if off {
		cmds = append(cmds, "UNLOG RECTIMEB")
	}
	// Note: We only use RECTIMEB for time pulse, not PPSSTATUS
	// Enable/disable UTCB messages based on enabled GNSS systems
	var enable bool
	if flags&gpsprot.PVTMsgLeapSecond != 0 {
		enable = true
	} else if off {
		enable = false
	} else {
		return cmds
	}
	addUTCBCmd(&cmds, enabledGNSS, gpsprot.GPS, "GPSUTCB", enable)
	addUTCBCmd(&cmds, enabledGNSS, gpsprot.BDS, "BD3UTCB", enable)
	addUTCBCmd(&cmds, enabledGNSS, gpsprot.GAL, "GALUTCB", enable)
	return cmds
}

// addUTCBCmd adds a UTCB command if the GNSS is enabled
func addUTCBCmd(cmds *[]string, enabledGNSS gpsprot.GNSSSet, gnss gpsprot.GNSS, msgName string, enable bool) {
	if enabledGNSS.Contains(gnss) {
		if enable {
			*cmds = append(*cmds, msgName+" 1")
		} else {
			*cmds = append(*cmds, "UNLOG "+msgName)
		}
	}
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
func generateRTCMMsgCommands(flags gpsprot.RTCMMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	gloEnabled := false
	
	if flags&gpsprot.RTCMMsgMSM4 != 0 {
		if enabledGNSS.Contains(gpsprot.GPS) {
			cmds = append(cmds, "RTCM1074 1") // GPS MSM4
		}
		if enabledGNSS.Contains(gpsprot.GLO) {
			cmds = append(cmds, "RTCM1084 1") // GLONASS MSM4
			gloEnabled = true
		}
		if enabledGNSS.Contains(gpsprot.GAL) {
			cmds = append(cmds, "RTCM1094 1") // Galileo MSM4
		}
		if enabledGNSS.Contains(gpsprot.QZSS) {
			cmds = append(cmds, "RTCM1114 1") // QZSS MSM4
		}
		if enabledGNSS.Contains(gpsprot.BDS) {
			cmds = append(cmds, "RTCM1124 1") // BDS MSM4
		}
	}
	if flags&gpsprot.RTCMMsgMSM7 != 0 {
		if enabledGNSS.Contains(gpsprot.GPS) {
			cmds = append(cmds, "RTCM1077 1") // GPS MSM7
		}
		if enabledGNSS.Contains(gpsprot.GLO) {
			cmds = append(cmds, "RTCM1087 1") // GLONASS MSM7
			gloEnabled = true
		}
		if enabledGNSS.Contains(gpsprot.GAL) {
			cmds = append(cmds, "RTCM1097 1") // Galileo MSM7
		}
		if enabledGNSS.Contains(gpsprot.QZSS) {
			cmds = append(cmds, "RTCM1117 1") // QZSS MSM7
		}
		if enabledGNSS.Contains(gpsprot.BDS) {
			cmds = append(cmds, "RTCM1127 1") // BDS MSM7
		}
	}
	
	// Add GLONASS bias if GLONASS is enabled
	if gloEnabled {
		cmds = append(cmds, "RTCM1230 1") // GLONASS code-phase biases
	}
	
	if flags&gpsprot.RTCMMsgARP != 0 {
		cmds = append(cmds, "RTCM1005 1")
	}
	return cmds
}

// generateRawMsgCommands maps Raw flags to Unicore raw data commands
func generateRawMsgCommands(flags gpsprot.RawMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	if flags&gpsprot.RawMsgObs != 0 {
		cmds = append(cmds, "OBSVMB 1")
	}
	if flags&gpsprot.RawMsgNavData != 0 {
		// Only enable ephemeris for enabled GNSS
		if enabledGNSS.Contains(gpsprot.GPS) {
			cmds = append(cmds, "GPSEPHB 1")
		}
		if enabledGNSS.Contains(gpsprot.BDS) {
			cmds = append(cmds, "BDSEPHB 1")
		}
		if enabledGNSS.Contains(gpsprot.GLO) {
			cmds = append(cmds, "GLOEPHB 1")
		}
		if enabledGNSS.Contains(gpsprot.GAL) {
			cmds = append(cmds, "GALEPHB 1")
		}
		if enabledGNSS.Contains(gpsprot.QZSS) {
			cmds = append(cmds, "QZSSEPHB 1")
		}
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