package ubx

import (
	"fmt"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// corReportRxmCor converts a UBX-RXM-COR message to a protocol-independent
// CorReportMsg.  Returns nil if the correction protocol is not mapped into
// the gpsprot.Tag space.
func corReportRxmCor(m *ubxbin.RxmCor) *gpsprot.CorReportMsg {
	tag, msgID := rxmCorTagMsgID(m)
	if tag == gpsprot.EmptyTag {
		return nil
	}
	msg := &gpsprot.CorReportMsg{
		Source: gpsprot.CorReportSourceReceiver,
		Tag:    tag,
		MsgID:  msgID,
	}
	si := m.StatusInfo
	switch si & ubxbin.RxmCorMsgUsed {
	case ubxbin.RxmCorMsgUsedNotUsed:
		msg.Used = opt.Make(false)
	case ubxbin.RxmCorMsgUsedUsed:
		msg.Used = opt.Make(true)
	}
	switch si & ubxbin.RxmCorErrStatus {
	case ubxbin.RxmCorErrStatusErrorFree:
		msg.ChecksumOK = opt.Make(true)
	case ubxbin.RxmCorErrStatusErroneous:
		msg.ChecksumOK = opt.Make(false)
	}
	// CorrectionID is 0xFFFF when no reference station ID applies
	// (other protocols, or RTCM messages without DF003).
	if id := si.CorrectionID(); id != 0xFFFF {
		msg.RTCMRefBaseID = opt.Make(id)
	}
	return msg
}

// rxmCorTagMsgID returns the protocol tag and message ID for a UBX-RXM-COR message.
func rxmCorTagMsgID(m *ubxbin.RxmCor) (gpsprot.Tag, string) {
	si := m.StatusInfo
	switch si & ubxbin.RxmCorProtocol {
	case ubxbin.RxmCorProtocolRTCM3:
		if si&ubxbin.RxmCorMsgTypeValid == 0 {
			return rtcm.Tag, ""
		}
		mt := rtcmbin.MsgType(m.MsgType)
		if mt == 4072 && si&ubxbin.RxmCorMsgSubTypeValid != 0 {
			return rtcm.Tag, fmt.Sprintf("%d.%d", mt, m.MsgSubType)
		}
		return rtcm.Tag, mt.String()
	case ubxbin.RxmCorProtocolSPARTN:
		// A SPARTN message ID is "type.subtype"; emit it only when both
		// fields are valid so it matches the pull-side report.
		if si&ubxbin.RxmCorMsgTypeValid == 0 || si&ubxbin.RxmCorMsgSubTypeValid == 0 {
			return spartn.Tag, ""
		}
		return spartn.Tag, spartnbin.FormatMsgID(uint8(m.MsgType), uint8(m.MsgSubType))
	default:
		return gpsprot.EmptyTag, ""
	}
}
