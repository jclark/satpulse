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
// for correction modes with no gpsprot.Tag (RTCMv2, CMR, RTCMV). baseID is the
// last-seen RTCM base station ID, filled only when it came from an RTCM base.
func corReportDiffCorrIn(m *sbfbin.DiffCorrIn, baseID opt.Val[uint16], baseRTCM bool) *gpsprot.CorReportMsg {
	corr := m.Correction
	msg := &gpsprot.CorReportMsg{
		Source:     gpsprot.CorReportSourceReceiver,
		ChecksumOK: opt.Make(true), // receiver only reports frame-verified messages
	}
	switch m.Mode {
	case sbfbin.DiffCorrModeRTCMv3:
		msg.Tag = rtcm.Tag
		msg.MsgID = rtcmbin.ExtractMsgID(string(corr))
		if n, ok := rtcm3FrameLen(corr); ok {
			msg.NBytes = opt.Make(n)
		}
		if native, err := rtcmbin.ParseMsg(string(corr)); err == nil {
			msg.NativeMsg = native
		}
	case sbfbin.DiffCorrModeSPARTN:
		msg.Tag = spartn.Tag
		msg.MsgID = spartnbin.MsgID(corr)
		if n, ok := spartnbin.FrameLen(corr); ok {
			msg.NBytes = opt.Make(n)
		}
		if native, err := spartnbin.Parse(corr); err == nil {
			msg.NativeMsg = native
		}
	default:
		return nil
	}
	if baseRTCM && baseID.IsSet() {
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
