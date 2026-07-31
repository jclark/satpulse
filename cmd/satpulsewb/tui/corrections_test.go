package tui

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func corPacket(msgID string, finalFragment opt.Val[bool]) *gpsprot.CorReportMsg {
	return &gpsprot.CorReportMsg{Tag: "RTCM", MsgID: msgID, FinalFragment: finalFragment}
}

// TestCorrectionsCounting checks the logical-message counting rules:
// a fragmentable message counts once per epoch with extra fragments
// as splits, a non-fragmentable message counts per packet.
func TestCorrectionsCounting(t *testing.T) {
	final := opt.Make(true)
	nonFinal := opt.Make(false)
	var none opt.Val[bool]
	tests := []struct {
		name         string
		packets      []*gpsprot.CorReportMsg
		msgID        string
		expectCount  int
		expectSplits int
	}{
		{
			name: "one fragmentable message per epoch",
			packets: []*gpsprot.CorReportMsg{
				corPacket("1077", final), corPacket("1077", final), corPacket("1077", final),
			},
			msgID: "1077", expectCount: 3, expectSplits: 0,
		},
		{
			name: "split fragments count once with a split",
			packets: []*gpsprot.CorReportMsg{
				corPacket("1077", nonFinal), corPacket("1077", final),
				corPacket("1077", nonFinal), corPacket("1077", final),
			},
			msgID: "1077", expectCount: 2, expectSplits: 2,
		},
		{
			name: "non-fragmentable counts per packet",
			packets: []*gpsprot.CorReportMsg{
				corPacket("1005", none), corPacket("1005", none), corPacket("1005", none),
			},
			msgID: "1005", expectCount: 3, expectSplits: 0,
		},
		{
			name: "epoch shared across message types",
			packets: []*gpsprot.CorReportMsg{
				corPacket("1077", nonFinal), corPacket("1087", final),
				corPacket("1077", nonFinal), corPacket("1087", final),
			},
			msgID: "1077", expectCount: 2, expectSplits: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &correctionsView{rows: make(map[string]*corRow)}
			for _, p := range tc.packets {
				v.handleCorPacket(p)
			}
			r := v.rows[tc.msgID]
			if r == nil {
				t.Fatalf("no row for %s", tc.msgID)
			}
			if r.count != tc.expectCount || r.splits != tc.expectSplits {
				t.Errorf("count %d splits %d, want %d and %d",
					r.count, r.splits, tc.expectCount, tc.expectSplits)
			}
		})
	}
}
