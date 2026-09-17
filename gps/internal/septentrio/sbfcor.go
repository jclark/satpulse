package septentrio

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
)

// corReportDiffCorrIn converts a DiffCorrIn block (an inbound correction
// message the receiver already accepted) into a CorReportMsg. It returns nil
// for correction modes with no gpsprot.Tag (RTCMv2, CMR, RTCMV) or an invalid
// embedded frame. baseID is the last-seen RTCM base station ID, filled only
// on RTCMv3 reports when it came from an RTCM base.
func corReportDiffCorrIn(m *sbfbin.DiffCorrIn, baseID opt.Val[uint16], baseRTCM bool) *gpsprot.CorReportMsg {
	corr := m.Correction
	var pf gpsprot.PacketFormat
	var n int
	var ok bool
	switch m.Mode {
	case sbfbin.DiffCorrModeRTCMv3:
		pf = rtcm.PacketFormat
		n, ok = rtcm3FrameLen(corr)
	case sbfbin.DiffCorrModeSPARTN:
		pf = spartn.PacketFormat
		n, ok = spartnbin.FrameLen(corr)
	default:
		return nil
	}
	if !ok || n > len(corr) {
		return nil
	}
	corr = corr[:n]
	if !gpsprot.IsValidPacket(pf, corr) {
		return nil
	}
	msg := &gpsprot.CorReportMsg{
		Source:     gpsprot.CorReportSourceReceiver,
		Tag:        pf.Tag(),
		MsgID:      pf.MsgID(corr),
		NBytes:     opt.Make(n),
		ChecksumOK: opt.Make(true), // receiver only reports frame-verified messages
	}
	if m.Mode == sbfbin.DiffCorrModeRTCMv3 {
		if native, err := rtcmbin.ParseMsg(string(corr)); err == nil {
			msg.NativeMsg = native
		}
	} else if native, err := spartnbin.Parse(corr); err == nil {
		msg.NativeMsg = native
	}
	if m.Mode == sbfbin.DiffCorrModeRTCMv3 && baseRTCM && baseID.IsSet() {
		msg.RTCMRefBaseID = baseID
	}
	return msg
}

// rtcm3FrameLen returns the framed length of the RTCM3 message: the 3-byte
// header (preamble + 10-bit payload length), the payload, and the 3-byte CRC.
// It reads the inner protocol's own length, never SBF's padded Length-16.
func rtcm3FrameLen(corr []byte) (int, bool) {
	if len(corr) < 3 || corr[0] != rtcmbin.PreambleByte {
		return 0, false
	}
	payload := int(corr[1]&0x03)<<8 | int(corr[2])
	return 3 + payload + 3, true
}
