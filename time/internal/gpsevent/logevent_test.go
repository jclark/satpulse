package gpsevent

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestLogEventRoundTrip(t *testing.T) {
	tm := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name  string
		event LogEvent
	}{
		{
			name: "pulseEdge",
			event: LogEvent{
				Type: pulseEdgeType,
				T:    tm,
				Mono: 1500 * gpsprot.Millisecond,
				Data: &PulseEdge{
					T:     ptime.Time(100 * time.Second),
					Era:   1,
					TRead: ptime.Time(100*time.Second + 99*time.Microsecond),
				},
			},
		},
		{
			name: "time",
			event: LogEvent{
				Type: "time",
				T:    tm,
				Mono: 2 * gpsprot.Second,
				Data: &gpsprot.TimeMsg{TAITime: ptime.Time(123 * time.Second), GNSS: gpsprot.GPS},
			},
		},
		{
			name: "corReport",
			event: LogEvent{
				Type: "corReport",
				T:    tm,
				Mono: 3 * gpsprot.Second,
				Data: &gpsprot.CorReportMsg{Source: gpsprot.CorReportSourcePull, Tag: "RTCM", MsgID: "1005"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got LogEvent
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(got, tc.event) {
				t.Errorf("got  %+v\nwant %+v", got, tc.event)
			}
		})
	}
}
