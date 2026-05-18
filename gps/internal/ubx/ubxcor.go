package ubx

import (
	"fmt"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// corReportRxmCor converts a UBX-RXM-COR message to a protocol-independent
// CorReportMsg.  Returns nil if the correction protocol is not RTCM3:
// only RTCM is currently mapped into the gpsprot.Tag space.
func corReportRxmCor(m *ubxbin.RxmCor) *gpsprot.CorReportMsg {
	si := m.StatusInfo
	if si&ubxbin.RxmCorProtocol != ubxbin.RxmCorProtocolRTCM3 {
		return nil
	}
	msg := &gpsprot.CorReportMsg{
		Source: gpsprot.CorReportSourceReceiver,
		Tag:    rtcm.Tag,
		MsgID:  rxmCorMsgID(m),
	}
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

// rxmCorMsgID formats the RTCM message ID, including the subtype for
// proprietary RTCM 4072 messages (e.g. "4072.1").
func rxmCorMsgID(m *ubxbin.RxmCor) string {
	si := m.StatusInfo
	if si&ubxbin.RxmCorMsgTypeValid == 0 {
		return ""
	}
	mt := rtcmbin.MsgType(m.MsgType)
	if mt == 4072 && si&ubxbin.RxmCorMsgSubTypeValid != 0 {
		return fmt.Sprintf("%d.%d", mt, m.MsgSubType)
	}
	return mt.String()
}
