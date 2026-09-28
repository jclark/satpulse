package sseobs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/lib/sse"
	"github.com/jclark/satpulse/time/phctime"
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
				events := obs.InitEvents()
				if len(events) != 1 {
					panic("expected single init event before any ModeChanged")
				}
				obs.sseCh <- events[0]
			},
			eventType:    "init",
			expectedJSON: `{"receiver":{"vendor":"u-blox","firmware":"HPG 1.32 PROTVER 27.31","hardware":"ZED-F9P","supportedGNSS":["GPS","GLO"]}}`,
		},
		{
			name: "mode_changed",
			action: func(obs *SSEObserver) {
				obs.ModeChanged(phcsync.ModeReset, phcsync.ModeTracking)
			},
			eventType:    "mode",
			expectedJSON: `{"mode":"tracking"}`,
		},
		{
			name: "sample_ok",
			action: func(obs *SSEObserver) {
				obs.Sample(phcsync.Sample{
					Kind:   phcsync.SampleOK,
					Offset: 123 * time.Nanosecond,
					Freq:   1.5,
					Mode:   phcsync.ModeTracking,
					Era:    phctime.Era(0),
				})
			},
			eventType:    "phc",
			expectedJSON: `{"offset":123,"freq":1.5,"stepCount":0,"stepCountChanging":true,"mode":"tracking"}`,
		},
		{
			name: "sample_outlier",
			action: func(obs *SSEObserver) {
				obs.Sample(phcsync.Sample{
					Kind:   phcsync.SampleOutlier,
					Offset: -456 * time.Nanosecond,
					Freq:   -2.3,
					Mode:   phcsync.ModeReset,
					Era:    phctime.Era(0),
				})
			},
			eventType:    "phc",
			expectedJSON: `{"offset":-456,"freq":-2.3,"stepCount":0,"stepCountChanging":true,"outlier":true,"mode":"reset"}`,
		},
		{
			name: "time",
			action: func(obs *SSEObserver) {
				obs.Tick(&gpsprot.TimeMsg{
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
					ObsTime:    30 * gpsprot.Second,
					ObsCount:   150,
					InProgress: true,
					Valid:      true,
				}, time.Now())
			},
			eventType:    "survey",
			expectedJSON: `{"x":1000,"y":2000,"z":3000,"accuracy":5.5,"obsTime":30,"obsCount":150,"inProgress":true,"valid":true}`,
		},
		{
			name: "survey_no_position",
			action: func(obs *SSEObserver) {
				obs.Survey(&gpsprot.SurveyMsg{
					Accuracy:   gpsprot.Meters(5.5),
					ObsTime:    60 * gpsprot.Second,
					ObsCount:   60,
					InProgress: true,
					Valid:      false,
				}, time.Now())
			},
			eventType:    "survey",
			expectedJSON: `{"accuracy":5.5,"obsTime":60,"obsCount":60,"inProgress":true,"valid":false}`,
		},
		{
			name: "satellites",
			action: func(obs *SSEObserver) {
				obs.Satellites(&gpsprot.SatellitesMsg{
					SVs: []gpsprot.SVInfo{
						{
							ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 1},
							LookAngles: opt.Make(gpsprot.LookAngles{
								Azimuth:   45,
								Elevation: 30,
							}),
							Signals: []gpsprot.SignalInfo{{ID: "L1"}},
							Used:    true,
						},
					},
				}, time.Now())
			},
			eventType:    "satellites",
			expectedJSON: `{"svs":[{"id":"G01","lookAngles":{"azimuth":45,"elevation":30},"signals":[{"id":"L1","cn0":0}],"used":true}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan sse.Event, 1)
			cfgResult := &gpscfg.Result{
				ReceiverInfo: &gpsprot.ReceiverInfo{
					Vendor:        "u-blox",
					Hardware:      "ZED-F9P",
					Firmware:      "HPG 1.32 PROTVER 27.31",
					SupportedGNSS: gpsprot.GNSSSetOf(gpsprot.GPS) | gpsprot.GNSSSetOf(gpsprot.GLO),
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

func TestBuildPosVelSSE(t *testing.T) {
	tests := []struct {
		name   string
		bundle gpsprot.PVMsgBundle
		want   string // empty means nil result
	}{
		{
			name:   "empty_bundle",
			bundle: gpsprot.PVMsgBundle{},
		},
		{
			name: "geo_position_only",
			bundle: gpsprot.PVMsgBundle{
				PosGeo: opt.Make(gpsprot.PosGeoMsg{
					LatLon: [2]gpsprot.Angle{
						gpsprot.DegreesFromFloat(47.5),
						gpsprot.DegreesFromFloat(7.6),
					},
					Height:    opt.Make(gpsprot.Meters(540.5)),
					HeightMSL: opt.Make(gpsprot.Meters(492.3)),
				}),
			},
			want: `{"latLon":[47.5,7.6],"height":540.5,"heightMSL":492.3}`,
		},
		{
			name: "ecef_position",
			bundle: gpsprot.PVMsgBundle{
				PosECEF: opt.Make(gpsprot.PosECEFMsg{
					Pos: gpsprot.Point3D{
						gpsprot.Meters(4000000),
						gpsprot.Meters(500000),
						gpsprot.Meters(4700000),
					},
				}),
			},
			want: `{"posECEFX":4000000,"posECEFY":500000,"posECEFZ":4700000}`,
		},
		{
			name: "geo_velocity",
			bundle: gpsprot.PVMsgBundle{
				VelGeo: opt.Make(gpsprot.VelGeoMsg{
					GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(1.5)),
					Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(1.6)),
					Course:      opt.Make(gpsprot.DegreesFromFloat(180.5)),
					VelNED:      opt.Make([3]gpsprot.Speed{gpsprot.MeterPerSecond, 2 * gpsprot.MeterPerSecond, -gpsprot.MeterPerSecond / 2}),
				}),
			},
			want: `{"groundSpeed":1.5,"speed3D":1.6,"course":180.5,"velN":1,"velE":2,"velD":-0.5}`,
		},
		{
			name: "ecef_velocity",
			bundle: gpsprot.PVMsgBundle{
				VelECEF: opt.Make(gpsprot.VelECEFMsg{
					Vel: [3]gpsprot.Speed{gpsprot.MeterPerSecond, 2 * gpsprot.MeterPerSecond, 3 * gpsprot.MeterPerSecond},
				}),
			},
			want: `{"velECEFX":1,"velECEFY":2,"velECEFZ":3}`,
		},
		{
			name: "all_fields",
			bundle: gpsprot.PVMsgBundle{
				PosGeo: opt.Make(gpsprot.PosGeoMsg{
					LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.5), gpsprot.DegreesFromFloat(7.6)},
					Height: opt.Make(gpsprot.Meters(500)),
				}),
				PosECEF: opt.Make(gpsprot.PosECEFMsg{
					Pos: gpsprot.Point3D{gpsprot.Meters(4000000), gpsprot.Meters(500000), gpsprot.Meters(4700000)},
				}),
				VelGeo: opt.Make(gpsprot.VelGeoMsg{
					GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(0.5)),
				}),
				VelECEF: opt.Make(gpsprot.VelECEFMsg{
					Vel: [3]gpsprot.Speed{gpsprot.MeterPerSecond, 0, 0},
				}),
			},
			want: `{"latLon":[47.5,7.6],"height":500,"posECEFX":4000000,"posECEFY":500000,"posECEFZ":4700000,"groundSpeed":0.5,"velECEFX":1,"velECEFY":0,"velECEFZ":0}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPosVelSSE(&tt.bundle)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(string(gotJSON), tt.want) {
				t.Errorf("JSON mismatch:\nGot:      %s\nExpected: %s", gotJSON, tt.want)
			}
		})
	}
}

func TestBuildQualitySSE(t *testing.T) {
	tests := []struct {
		name string
		msg  gpsprot.NavEpochMsg
		want string
	}{
		{
			name: "minimal_code_3d",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				SolutionDim: gpsprot.SolutionDim3D,
			},
			want: `{"fixLevel":"code","solutionDim":"3D"}`,
		},
		{
			name: "carrier_fixed_with_dop",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCarrierFixed,
				SolutionDim: gpsprot.SolutionDim3D,
				Correction:  gpsprot.CorrOSR,
				DOP: gpsprot.DOP{
					Pos:  opt.Make(1.2),
					Hor:  opt.Make(0.8),
					Vert: opt.Make(0.9),
				},
				NumSVUsed: opt.Make[uint16](12),
			},
			want: `{"fixLevel":"carrierFixed","solutionDim":"3D","corrections":["OSR"],"pdop":1.2,"hdop":0.8,"vdop":0.9,"numSVUsed":12}`,
		},
		{
			name: "with_accuracy",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				SolutionDim: gpsprot.SolutionDim3D,
				Acc: gpsprot.Accuracy{
					Hor:  opt.Make(gpsprot.Meters(2.5)),
					Vert: opt.Make(gpsprot.Meters(4.0)),
				},
			},
			want: `{"fixLevel":"code","solutionDim":"3D","accHor":2.5,"accVert":4}`,
		},
		{
			name: "with_gnss_bands_used",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				SolutionDim: gpsprot.SolutionDim3D,
				GNSSUsed:    gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GAL),
				BandsUsed:   gpsprot.BandL1 | gpsprot.BandL5,
			},
			want: `{"fixLevel":"code","solutionDim":"3D","gnssUsed":["GPS","GAL"],"bandsUsed":["L1","L5"]}`,
		},
		{
			name: "with_diffage",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				SolutionDim: gpsprot.SolutionDim3D,
				DiffAge:     opt.Make(2 * gpsprot.Second),
			},
			want: `{"fixLevel":"code","solutionDim":"3D","diffAge":2}`,
		},
		{
			name: "no_fix",
			msg: gpsprot.NavEpochMsg{
				FixLevel: gpsprot.FixLevelNone,
			},
			want: `{"fixLevel":"none"}`,
		},
		{
			name: "zero_fixlevel",
			msg:  gpsprot.NavEpochMsg{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQualitySSE(&tt.msg)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(string(gotJSON), tt.want) {
				t.Errorf("JSON mismatch:\nGot:      %s\nExpected: %s", gotJSON, tt.want)
			}
		})
	}
}

func TestNavEpochPVSSE(t *testing.T) {
	ch := make(chan sse.Event, 4)
	cfgResult := &gpscfg.Result{
		ReceiverInfo: &gpsprot.ReceiverInfo{Vendor: "test"},
	}
	obs := New(ch, ptime.LeapSecond{}, slog.Default(), cfgResult)
	// Build a PVMsgBundle as the Dispatcher would
	pv := &gpsprot.PVMsgBundle{
		PosGeo: opt.Make(gpsprot.PosGeoMsg{
			LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.5), gpsprot.DegreesFromFloat(7.6)},
		}),
	}
	// Fire NavEpochPV
	obs.NavEpochPV(&gpsprot.NavEpochMsg{
		FixLevel:    gpsprot.FixLevelCode,
		SolutionDim: gpsprot.SolutionDim3D,
	}, pv, time.Now())
	// Should produce posvel then quality
	posvel := <-ch
	if !strings.Contains(posvel.Format(), "event: posvel\n") {
		t.Errorf("expected posvel event, got: %s", posvel.Format())
	}
	quality := <-ch
	if !strings.Contains(quality.Format(), "event: quality\n") {
		t.Errorf("expected quality event, got: %s", quality.Format())
	}
}

// TestInitEventsAfterModeChanged checks that once ModeChanged has been
// observed, InitEvents includes the cached mode event so a fresh client
// learns the current mode at connect time.
func TestInitEventsAfterModeChanged(t *testing.T) {
	ch := make(chan sse.Event, 1)
	cfgResult := &gpscfg.Result{ReceiverInfo: &gpsprot.ReceiverInfo{Vendor: "test"}}
	obs := New(ch, ptime.LeapSecond{}, slog.Default(), cfgResult)
	if got := obs.InitEvents(); len(got) != 1 {
		t.Fatalf("expected 1 init event before ModeChanged, got %d", len(got))
	}
	obs.ModeChanged(phcsync.ModeInvalid, phcsync.ModeReset)
	<-ch // drain the broadcast emit
	events := obs.InitEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 init events after ModeChanged, got %d", len(events))
	}
	if !strings.Contains(events[0].Format(), "event: init\n") {
		t.Errorf("first init event should be init, got: %s", events[0].Format())
	}
	wire := events[1].Format()
	if !strings.Contains(wire, "event: mode\n") {
		t.Errorf("second init event should be mode, got: %s", wire)
	}
	if !strings.Contains(wire, `{"mode":"reset"}`) {
		t.Errorf("mode event payload should be {\"mode\":\"reset\"}, got: %s", wire)
	}
	// A subsequent transition replaces the cached mode.
	obs.ModeChanged(phcsync.ModeReset, phcsync.ModeTracking)
	<-ch
	events = obs.InitEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 init events, got %d", len(events))
	}
	if !strings.Contains(events[1].Format(), `{"mode":"tracking"}`) {
		t.Errorf("expected mode=tracking after second ModeChanged, got: %s", events[1].Format())
	}
}

// TestSSEObserverCorReportLatch exercises the source-preference state
// machine in SSEObserver.CorReport: starts in pull, switches to
// receiver on any receiver-source event, and switches back to pull
// only on a pull-source event arriving more than corReportSourceTimeout
// after the last receiver-source event.
func TestSSEObserverCorReportLatch(t *testing.T) {
	t0 := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	pull := gpsprot.CorReportSourcePull
	recv := gpsprot.CorReportSourceReceiver

	type input struct {
		source gpsprot.CorReportSource
		tRead  time.Time
	}
	tests := []struct {
		name   string
		events []input
		expect []string
	}{
		{
			name: "pull stays in pull mode",
			events: []input{
				{pull, t0},
				{pull, t0.Add(time.Second)},
			},
			expect: []string{"pull", "pull"},
		},
		{
			name: "receiver switches and drops pull within timeout",
			events: []input{
				{pull, t0},
				{recv, t0.Add(time.Second)},
				{pull, t0.Add(2 * time.Second)},
			},
			expect: []string{"pull", "receiver"},
		},
		{
			name: "pull at exactly timeout still dropped",
			events: []input{
				{recv, t0},
				{pull, t0.Add(corReportSourceTimeout)},
			},
			expect: []string{"receiver"},
		},
		{
			name: "pull past timeout switches back to pull",
			events: []input{
				{recv, t0},
				{pull, t0.Add(corReportSourceTimeout + time.Nanosecond)},
				{pull, t0.Add(corReportSourceTimeout + time.Second)},
			},
			expect: []string{"receiver", "pull", "pull"},
		},
		{
			name: "receiver refreshes timeout",
			events: []input{
				{recv, t0},
				{recv, t0.Add(corReportSourceTimeout - time.Second)},
				{pull, t0.Add(corReportSourceTimeout + time.Second)},
			},
			expect: []string{"receiver", "receiver"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan sse.Event, len(tc.events))
			obs := New(ch, ptime.LeapSecond{}, slog.Default(), &gpscfg.Result{})
			for _, ev := range tc.events {
				obs.CorReport(&gpsprot.CorReportMsg{
					Source: ev.source,
					Tag:    "RTCM",
					MsgID:  "1077",
				}, ev.tRead)
			}
			close(ch)
			var got []string
			for ev := range ch {
				got = append(got, parseCorReportSource(t, ev))
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}

// parseCorReportSource extracts the source field from a corReport SSE event.
func parseCorReportSource(t *testing.T, ev sse.Event) string {
	t.Helper()
	var data string
	for line := range strings.SplitSeq(ev.Format(), "\n") {
		if d, ok := strings.CutPrefix(line, "data: "); ok {
			data = d
			break
		}
	}
	var payload struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	return payload.Source
}

// TestNewNilCfgResult checks that New tolerates a nil cfgResult and
// emits no init event in that case. This happens in degraded startup
// when GPS detection fails but the daemon continues running because no
// PHC is configured.
func TestNewNilCfgResult(t *testing.T) {
	ch := make(chan sse.Event, 1)
	obs := New(ch, ptime.LeapSecond{}, slog.Default(), nil)
	if got := obs.InitEvents(); len(got) != 0 {
		t.Fatalf("expected 0 init events with nil cfgResult, got %d", len(got))
	}
}

// jsonEqual compares two JSON strings for equivalence by parsing and using reflect.DeepEqual
func jsonEqual(a, b string) bool {
	var objA, objB any
	if err := json.Unmarshal([]byte(a), &objA); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &objB); err != nil {
		return false
	}

	return reflect.DeepEqual(objA, objB)
}
