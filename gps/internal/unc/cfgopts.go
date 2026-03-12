package unc

import "github.com/jclark/satpulse/gps/gpsprot"

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
	// BESTNAV / BESTNAVXYZ for position and velocity.
	// Both carry position AND velocity in a single message.
	// PVTMsgECEF selects BESTNAVXYZ (ECEF) vs BESTNAV (geodetic).
	// PVTMsgQuality also needs a nav message (for quality fields) plus STADOPB.
	wantPos := flags&gpsprot.PVTMsgPos != 0
	wantVel := flags&gpsprot.PVTMsgVel != 0
	wantQual := flags&gpsprot.PVTMsgQuality != 0
	wantNav := wantPos || wantVel || wantQual
	if wantNav {
		if flags&gpsprot.PVTMsgECEF != 0 {
			cmds = append(cmds, "BESTNAVXYZB 1")
		} else {
			cmds = append(cmds, "BESTNAVB 1")
		}
	} else if off {
		cmds = append(cmds, "UNLOG BESTNAVB")
		cmds = append(cmds, "UNLOG BESTNAVXYZB")
	}
	// STADOPB for DOP values (only needed by PVTMsgQuality).
	if wantQual {
		cmds = append(cmds, "STADOPB 1")
	} else if off {
		cmds = append(cmds, "UNLOG STADOPB")
	}
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

// appendMsgCmd appends either "msgName 1" or "UNLOG msgName" depending on enable.
func appendMsgCmd(cmds *[]string, msgName string, enable bool) {
	if enable {
		*cmds = append(*cmds, msgName+" 1")
	} else {
		*cmds = append(*cmds, "UNLOG "+msgName)
	}
}

// addUTCBCmd adds a UTCB command if the GNSS is enabled
func addUTCBCmd(cmds *[]string, enabledGNSS gpsprot.GNSSSet, gnss gpsprot.GNSS, msgName string, enable bool) {
	if enabledGNSS.Contains(gnss) {
		appendMsgCmd(cmds, msgName, enable)
	}
}

// generateSatsMsgCommands maps Sats flags to Unicore message commands
func generateSatsMsgCommands(flags gpsprot.SatsMsgFlags) []string {
	var cmds []string
	// SATSINFOB provides satellite and signal info;
	// BESTSATB provides which satellites are used in the solution.
	enable := flags&gpsprot.SatsMsgAny != 0
	appendMsgCmd(&cmds, "SATSINFOB", enable)
	appendMsgCmd(&cmds, "BESTSATB", enable)
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
		{gpsprot.NMEAMsgGLL, "GPGLL"},
	}
	for _, m := range msgs {
		appendMsgCmd(&cmds, m.cmd, flags&m.flag != 0)
	}
	return cmds
}

// generateRTCMMsgCommands maps RTCM flags to Unicore RTCM commands
func generateRTCMMsgCommands(flags gpsprot.RTCMMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	msm4 := flags&gpsprot.RTCMMsgMSM4 != 0
	msm7 := flags&gpsprot.RTCMMsgMSM7 != 0
	type gnssMsg struct {
		gnss   gpsprot.GNSS
		msm4ID string
		msm7ID string
	}
	gnssMsgs := []gnssMsg{
		{gpsprot.GPS, "RTCM1074", "RTCM1077"},
		{gpsprot.GLO, "RTCM1084", "RTCM1087"},
		{gpsprot.GAL, "RTCM1094", "RTCM1097"},
		{gpsprot.QZSS, "RTCM1114", "RTCM1117"},
		{gpsprot.BDS, "RTCM1124", "RTCM1127"},
	}
	for _, g := range gnssMsgs {
		if enabledGNSS.Contains(g.gnss) {
			appendMsgCmd(&cmds, g.msm4ID, msm4)
			appendMsgCmd(&cmds, g.msm7ID, msm7)
		}
	}
	// GLONASS code-phase biases required when GLONASS MSM messages are enabled
	if enabledGNSS.Contains(gpsprot.GLO) {
		appendMsgCmd(&cmds, "RTCM1230", msm4||msm7)
	}
	appendMsgCmd(&cmds, "RTCM1005", flags&gpsprot.RTCMMsgARP != 0)
	return cmds
}

// generateRawMsgCommands maps Raw flags to Unicore raw data commands
func generateRawMsgCommands(flags gpsprot.RawMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	navData := flags&gpsprot.RawMsgNavData != 0
	appendMsgCmd(&cmds, "OBSVMB", flags&gpsprot.RawMsgObs != 0)
	type gnssEph struct {
		gnss   gpsprot.GNSS
		msgName string
	}
	ephs := []gnssEph{
		{gpsprot.GPS, "GPSEPHB"},
		{gpsprot.BDS, "BDSEPHB"},
		{gpsprot.GLO, "GLOEPHB"},
		{gpsprot.GAL, "GALEPHB"},
		{gpsprot.QZSS, "QZSSEPHB"},
	}
	for _, e := range ephs {
		if enabledGNSS.Contains(e.gnss) {
			appendMsgCmd(&cmds, e.msgName, navData)
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