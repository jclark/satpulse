package sseobs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/sse"
	"github.com/jclark/satpulse/internal/ubx"
)

const oneYearSecs = 365 * 24 * 3600

func TestSSEObserver_Events(t *testing.T) {
	tests := []struct {
		name         string
		action       func(*SSEObserver)
		eventType    string
		expectedJSON string
	}{
		{
			name: "init",
			action: func(obs *SSEObserver) {
				event := obs.InitEvent()
				obs.sseCh <- event
			},
			eventType:    "init",
			expectedJSON: `{"version":{"hw":"00080000","sw":"EXT CORE 1.00 (61b2dd)","mod":"ZED-F9P","runsFromFlash":true,"prot":{"major":27,"minor":31},"fw":{"productCategory":"HPG","major":1,"minor":32}}}`,
		},
		{
			name: "sample_ok",
			action: func(obs *SSEObserver) {
				obs.Sample(mon.SampleData{
					Kind:      mon.SampleOK,
					Offset:    123 * time.Nanosecond,
					Freq:      1.5,
					SyncState: mon.InSync,
					Era:       ptime.Era(0),
				})
			},
			eventType:    "phc",
			expectedJSON: `{"offset":123,"freq":1.5,"stepCount":0,"stepCountChanging":true,"syncState":"in sync"}`,
		},
		{
			name: "sample_outlier",
			action: func(obs *SSEObserver) {
				obs.Sample(mon.SampleData{
					Kind:      mon.SampleOutlier,
					Offset:    -456 * time.Nanosecond,
					Freq:      -2.3,
					SyncState: mon.NoSync,
					Era:       ptime.Era(0),
				})
			},
			eventType:    "phc",
			expectedJSON: `{"offset":-456,"freq":-2.3,"stepCount":0,"stepCountChanging":true,"outlier":true,"syncState":"out of sync"}`,
		},
		{
			name: "time",
			action: func(obs *SSEObserver) {
				obs.Time(&gpsprot.TimeMsg{
					Ref:     gpsprot.PostPulse,
					TAITime: ptime.Time(oneYearSecs * 1e9),
				}, time.Now())
			},
			eventType:    "time",
			expectedJSON: fmt.Sprintf(`{"utc":"1971-01-01T00:00:00Z","tai":%d}`, oneYearSecs),
		},
		{
			name: "survey",
			action: func(obs *SSEObserver) {
				obs.Survey(&gpsprot.SurveyMsg{
					Position:   gpsprot.Point3D{gpsprot.Meters(1000), gpsprot.Meters(2000), gpsprot.Meters(3000)},
					Accuracy:   gpsprot.Meters(5.5),
					ObsTime:    30 * time.Second,
					ObsCount:   150,
					InProgress: true,
					Valid:      true,
				}, time.Now())
			},
			eventType:    "survey",
			expectedJSON: `{"x":1000,"y":2000,"z":3000,"accuracy":5.5,"obsTime":30,"obsCount":150,"inProgress":true,"valid":true}`,
		},
		{
			name: "satellites",
			action: func(obs *SSEObserver) {
				obs.Satellites(&gpsprot.SatellitesMsg{
					SVs: []gpsprot.SVInfo{
						{
							ID:        gpsprot.SVID{GNSS: gpsprot.GPS, PRN: 1},
							Azimuth:   45,
							Elevation: 30,
							Signals:   []gpsprot.SignalInfo{{ID: "L1"}},
							Used:      true,
						},
					},
				}, time.Now())
			},
			eventType:    "satellites",
			expectedJSON: `{"svs":[{"id":"G01","azimuth":45,"elevation":30,"signals":[{"id":"L1","cn0":0}],"used":true}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan sse.Event, 1)
			cfgResult := &gpscfg.Result{
				Version: &ubx.Version{
					HW:            "00080000",
					SW:            "EXT CORE 1.00 (61b2dd)",
					Mod:           "ZED-F9P",
					RunsFromFlash: true,
					Prot:          &ubx.ProtVer{Major: 27, Minor: 31},
					FW:            &ubx.FWVer{ProductCategory: "HPG", Major: 1, Minor: 32},
				},
			}
			obs := New(ch, ptime.LeapSecond{}, slog.Default(), cfgResult)

			tt.action(obs)

			event := <-ch
			wireFormat := event.Format()

			// Check event type
			expectedEventLine := "event: " + tt.eventType + "\n"
			if !strings.Contains(wireFormat, expectedEventLine) {
				t.Errorf("Expected event type %q in wire format", tt.eventType)
			}

			// Extract JSON from wire format
			lines := strings.Split(wireFormat, "\n")
			var jsonData string
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					jsonData = strings.TrimPrefix(line, "data: ")
					break
				}
			}

			// Compare JSON equivalence
			if !jsonEqual(jsonData, tt.expectedJSON) {
				t.Errorf("JSON mismatch:\nGot:      %s\nExpected: %s", jsonData, tt.expectedJSON)
			}
		})
	}
}

// jsonEqual compares two JSON strings for equivalence by parsing and using reflect.DeepEqual
func jsonEqual(a, b string) bool {
	var objA, objB interface{}
	if err := json.Unmarshal([]byte(a), &objA); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &objB); err != nil {
		return false
	}
	
	return reflect.DeepEqual(objA, objB)
}