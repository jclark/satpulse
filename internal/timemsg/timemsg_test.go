package timemsg

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/ubx"
)

func TestGetPostTimeMessages(t *testing.T) {
	startTime := time.Now()

	timeAt := func(nanos int64) time.Time {
		return startTime.Add(time.Duration(nanos))
	}

	tai := func(sec int64, ns int64) ptime.Time {
		return ptime.Time(sec*1e9 + ns)
	}

	tests := []struct {
		name       string
		entries    []struct {
			msg   *gpsprot.TimeMsg
			tRead time.Time
		}
		n           int
		expectLast  ptime.Time
		expectTRead []int64 // expected tRead values as nanos offsets
	}{
		{
			name: "real_data_ubx_timegps_and_timtp",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442831, 999795255),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-NAV-TIMEGPS",
					},
					tRead: timeAt(989951707),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442833, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(1010185067),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442832, 999795042),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-NAV-TIMEGPS",
					},
					tRead: timeAt(1992953605),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442834, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(2016301635),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442833, 999794828),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-NAV-TIMEGPS",
					},
					tRead: timeAt(2999948446),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442835, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(3023123219),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442834, 999794615),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-NAV-TIMEGPS",
					},
					tRead: timeAt(3992241034),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442836, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(4011345185),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442835, 999794402),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.NavSolution,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-NAV-TIMEGPS",
					},
					tRead: timeAt(4996451086),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442837, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(5018143724),
				},
			},
			n:          3,
			expectLast: tai(1707442837, 0),
			expectTRead: []int64{3023123219, 4011345185, 5018143724}, // TIM-TP messages for seconds 1707442835, 1707442836, 1707442837
		},
		{
			name: "missing_second_should_fail",
			entries: []struct {
				msg   *gpsprot.TimeMsg
				tRead time.Time
			}{
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442831, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(100000000),
				},
				// Missing 1707442832
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442833, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(2100000000),
				},
				{
					msg: &gpsprot.TimeMsg{
						TAITime:     tai(1707442834, 0),
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
					},
					tRead: timeAt(3100000000),
				},
			},
			n:          3,
			expectLast: 0,
			expectTRead: nil, // Should fail due to missing second
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lg := slog.New(slog.NewTextHandler(io.Discard, nil))
			buf := NewBuffer(lg, 10*time.Second, ptime.LeapSecond{UTCOffAfter: 37}, gpsprot.GPS)

			for _, e := range tt.entries {
				buf.Time(e.msg, e.tRead)
			}

			gotLast, gotTRead := buf.GetPostTimeMessages(tt.n)

			if gotLast != tt.expectLast {
				t.Errorf("GetPostTimeMessages() lastSec = %v, want %v", gotLast, tt.expectLast)
			}

			if len(gotTRead) != len(tt.expectTRead) {
				t.Errorf("GetPostTimeMessages() returned %d times, want %d", len(gotTRead), len(tt.expectTRead))
			}

			// Check that the tRead values match expected nanos offsets
			for i, tr := range gotTRead {
				if i < len(tt.expectTRead) {
					expectedTime := timeAt(tt.expectTRead[i])
					if !tr.Equal(expectedTime) {
						t.Errorf("tRead[%d] = %v, want %v (nanos: %d)", i, tr, expectedTime, tt.expectTRead[i])
					}
				}
			}
		})
	}
}