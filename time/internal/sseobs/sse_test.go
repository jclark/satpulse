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
				event := obs.InitEvent()
				obs.sseCh <- event
			},
			eventType:    "init",
			expectedJSON: `{"receiver":{"vendor":"u-blox","firmware":"HPG 1.32 PROTVER 27.31","hardware":"ZED-F9P","supportedGNSS":["GPS","GLO"]}}`,
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
			expectedJSON: `{"offset":123,"freq":1.5,"stepCount":0,"stepCountChanging":true,"syncState":"tracking"}`,
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
			expectedJSON: `{"offset":-456,"freq":-2.3,"stepCount":0,"stepCountChanging":true,"outlier":true,"syncState":"reset"}`,
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
			name: "survey_no_position",
			action: func(obs *SSEObserver) {
				obs.Survey(&gpsprot.SurveyMsg{
					Accuracy:   gpsprot.Meters(5.5),
					ObsTime:    60 * time.Second,
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
							LookAngles: &gpsprot.LookAngles{
								Azimuth:   45,
								Elevation: 30,
							},
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
		bundle gpsprot.MsgBundle
		want   string // empty means nil result
	}{
		{
			name:   "empty_bundle",
			bundle: gpsprot.MsgBundle{},
		},
		{
			name: "geo_position_only",
			bundle: gpsprot.MsgBundle{
				PosGeo: &gpsprot.PosGeoMsg{
					LatLon: [2]gpsprot.Angle{
						gpsprot.DegreesFromFloat(47.5),
						gpsprot.DegreesFromFloat(7.6),
					},
					Height:    opt.Make(gpsprot.Meters(540.5)),
					HeightMSL: opt.Make(gpsprot.Meters(492.3)),
				},
			},
			want: `{"latLon":[47.5,7.6],"height":540.5,"heightMSL":492.3}`,
		},
		{
			name: "ecef_position",
			bundle: gpsprot.MsgBundle{
				PosECEF: &gpsprot.PosECEFMsg{
					Pos: gpsprot.Point3D{
						gpsprot.Meters(4000000),
						gpsprot.Meters(500000),
						gpsprot.Meters(4700000),
					},
				},
			},
			want: `{"posECEF":[4000000,500000,4700000]}`,
		},
		{
			name: "geo_velocity",
			bundle: gpsprot.MsgBundle{
				VelGeo: &gpsprot.VelGeoMsg{
					GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(1.5)),
					Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(1.6)),
					Course:      opt.Make(gpsprot.DegreesFromFloat(180.5)),
					VelNED:      opt.Make([3]gpsprot.Speed{gpsprot.MeterPerSecond, 2 * gpsprot.MeterPerSecond, -gpsprot.MeterPerSecond / 2}),
				},
			},
			want: `{"groundSpeed":1.5,"speed3D":1.6,"course":180.5,"velNED":[1,2,-0.5]}`,
		},
		{
			name: "ecef_velocity",
			bundle: gpsprot.MsgBundle{
				VelECEF: &gpsprot.VelECEFMsg{
					Vel: [3]gpsprot.Speed{gpsprot.MeterPerSecond, 2 * gpsprot.MeterPerSecond, 3 * gpsprot.MeterPerSecond},
				},
			},
			want: `{"velECEF":[1,2,3]}`,
		},
		{
			name: "all_fields",
			bundle: gpsprot.MsgBundle{
				PosGeo: &gpsprot.PosGeoMsg{
					LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.5), gpsprot.DegreesFromFloat(7.6)},
					Height: opt.Make(gpsprot.Meters(500)),
				},
				PosECEF: &gpsprot.PosECEFMsg{
					Pos: gpsprot.Point3D{gpsprot.Meters(4000000), gpsprot.Meters(500000), gpsprot.Meters(4700000)},
				},
				VelGeo: &gpsprot.VelGeoMsg{
					GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(0.5)),
				},
				VelECEF: &gpsprot.VelECEFMsg{
					Vel: [3]gpsprot.Speed{gpsprot.MeterPerSecond, 0, 0},
				},
			},
			want: `{"latLon":[47.5,7.6],"height":500,"posECEF":[4000000,500000,4700000],"groundSpeed":0.5,"velECEF":[1,0,0]}`,
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
				FixLevel: gpsprot.FixLevelCode,
				FixDim:   gpsprot.FixDim3D,
			},
			want: `{"fix":["code","3D"]}`,
		},
		{
			name: "carrier_fixed_with_dop",
			msg: gpsprot.NavEpochMsg{
				FixLevel:   gpsprot.FixLevelCarrierFixed,
				FixDim:     gpsprot.FixDim3D,
				Correction: gpsprot.CorrBaseStation,
				DOP: gpsprot.DOP{
					Pos:  opt.Make(1.2),
					Hor:  opt.Make(0.8),
					Vert: opt.Make(0.9),
				},
				NumSVUsed: opt.Make[uint16](12),
			},
			want: `{"fix":["carrierFixed","3D"],"corrections":["baseStation"],"pdop":1.2,"hdop":0.8,"vdop":0.9,"numSVUsed":12}`,
		},
		{
			name: "with_accuracy",
			msg: gpsprot.NavEpochMsg{
				FixLevel: gpsprot.FixLevelCode,
				FixDim:   gpsprot.FixDim3D,
				Acc: gpsprot.Accuracy{
					Hor:  opt.Make(gpsprot.Meters(2.5)),
					Vert: opt.Make(gpsprot.Meters(4.0)),
				},
			},
			want: `{"fix":["code","3D"],"accHor":2.5,"accVert":4}`,
		},
		{
			name: "with_signals_used",
			msg: gpsprot.NavEpochMsg{
				FixLevel:    gpsprot.FixLevelCode,
				FixDim:      gpsprot.FixDim3D,
				SignalsUsed: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1),
			},
			want: `{"fix":["code","3D"],"signalsUsed":{"GPS":["L1"],"GAL":["E1"]}}`,
		},
		{
			name: "with_diffage",
			msg: gpsprot.NavEpochMsg{
				FixLevel: gpsprot.FixLevelCodeCorrected,
				FixDim:   gpsprot.FixDim3D,
				DiffAge:  opt.Make(2 * time.Second),
			},
			want: `{"fix":["codeCorrected","3D"],"diffAge":2}`,
		},
		{
			name: "no_fix",
			msg: gpsprot.NavEpochMsg{
				FixLevel: gpsprot.FixLevelNone,
			},
			want: `{"fix":["none"]}`,
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

func TestBuildFixKeywords(t *testing.T) {
	tests := []struct {
		name string
		msg  gpsprot.NavEpochMsg
		want []string
	}{
		{
			name: "code_3d",
			msg:  gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode, FixDim: gpsprot.FixDim3D},
			want: []string{"code", "3D"},
		},
		{
			name: "code_with_dr",
			msg:  gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelCode, FixDim: gpsprot.FixDim3D, AuxSrc: gpsprot.AuxSrcDR},
			want: []string{"code", "3D", "DR"},
		},
		{
			name: "level_only",
			msg:  gpsprot.NavEpochMsg{FixLevel: gpsprot.FixLevelNone},
			want: []string{"none"},
		},
		{
			name: "zero",
			msg:  gpsprot.NavEpochMsg{},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFixKeywords(&tt.msg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNavEpochSSE(t *testing.T) {
	ch := make(chan sse.Event, 4)
	cfgResult := &gpscfg.Result{
		ReceiverInfo: &gpsprot.ReceiverInfo{Vendor: "test"},
	}
	obs := New(ch, ptime.LeapSecond{}, slog.Default(), cfgResult)
	// Simulate accumulated position
	obs.PosGeo(&gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.5), gpsprot.DegreesFromFloat(7.6)},
	}, time.Now())
	// Fire NavEpoch
	obs.NavEpoch(&gpsprot.NavEpochMsg{
		FixLevel: gpsprot.FixLevelCode,
		FixDim:   gpsprot.FixDim3D,
	}, time.Now())
	// Should produce posvel then quality
	posvel := <-ch
	if !strings.Contains(posvel.Format(), "event: posvel\n") {
		t.Errorf("expected posvel event, got: %s", posvel.Format())
	}
	quality := <-ch
	if !strings.Contains(quality.Format(), "event: quality\n") {
		t.Errorf("expected quality event, got: %s", quality.Format())
	}
	// Bundle should be cleared after NavEpoch
	if obs.Bundle.PosGeo != nil {
		t.Error("expected Bundle.PosGeo to be nil after NavEpoch")
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
