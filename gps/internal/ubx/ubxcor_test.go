package ubx

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/spartn"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// noCorrectionID sets the CorrectionID field (bits 9..24) to 0xFFFF, the
// value u-blox reports when no reference station ID applies (e.g. SPARTN).
const noCorrectionID ubxbin.RxmCorStatusInfo = 0xFFFF << 9

func TestCorReportRxmCor(t *testing.T) {
	tests := []struct {
		name   string
		m      ubxbin.RxmCor
		expect *gpsprot.CorReportMsg
	}{
		{
			name: "SPARTN used and error-free",
			m: ubxbin.RxmCor{
				StatusInfo: ubxbin.RxmCorProtocolSPARTN | ubxbin.RxmCorMsgTypeValid |
					ubxbin.RxmCorMsgSubTypeValid | ubxbin.RxmCorErrStatusErrorFree |
					ubxbin.RxmCorMsgUsedUsed | noCorrectionID,
				MsgType:    1,
				MsgSubType: 2,
			},
			expect: &gpsprot.CorReportMsg{
				Source:     gpsprot.CorReportSourceReceiver,
				Tag:        spartn.Tag,
				MsgID:      "1.2",
				ChecksumOK: opt.Make(true),
				Used:       opt.Make(true),
			},
		},
		{
			name: "SPARTN with invalid type omits MsgID",
			m: ubxbin.RxmCor{
				StatusInfo: ubxbin.RxmCorProtocolSPARTN | ubxbin.RxmCorErrStatusErroneous |
					ubxbin.RxmCorMsgUsedNotUsed | noCorrectionID,
			},
			expect: &gpsprot.CorReportMsg{
				Source:     gpsprot.CorReportSourceReceiver,
				Tag:        spartn.Tag,
				ChecksumOK: opt.Make(false),
				Used:       opt.Make(false),
			},
		},
		{
			name: "RTCM3 still maps to RTCM tag",
			m: ubxbin.RxmCor{
				StatusInfo: ubxbin.RxmCorProtocolRTCM3 | ubxbin.RxmCorMsgTypeValid |
					ubxbin.RxmCorErrStatusErrorFree | ubxbin.RxmCorMsgUsedUsed | noCorrectionID,
				MsgType: 1077,
			},
			expect: &gpsprot.CorReportMsg{
				Source:     gpsprot.CorReportSourceReceiver,
				Tag:        rtcm.Tag,
				MsgID:      "1077",
				ChecksumOK: opt.Make(true),
				Used:       opt.Make(true),
			},
		},
		{
			name:   "unknown protocol returns nil",
			m:      ubxbin.RxmCor{StatusInfo: ubxbin.RxmCorProtocolUnknown | noCorrectionID},
			expect: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := corReportRxmCor(&tc.m)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
