package gpscmd

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestPrintJSON(t *testing.T) {
	props := &gpsprot.ConfigProps{}
	props.SetSignalsEnabled((1 << gpsprot.SigGPSL1CA) | (1 << gpsprot.SigGPSL2C))
	props.SetTimeGNSS(gpsprot.GPS)
	props.SetTimePulse(gpsprot.TimePulse{
		Width:          100 * time.Millisecond,
		Period:         time.Second,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
		PolarityRising: true,
	})
	props.SetMode(gpsprot.Mode{
		Static:  true,
		PosType: gpsprot.PosTypeECEF,
		FixedPosECEF: gpsprot.Point3D{
			gpsprot.Meters(-1144698.0),
			gpsprot.Meters(6090335.5),
			gpsprot.Meters(1504171.25),
		},
		FixedPosAcc: gpsprot.Meters(10),
	})
	props.SetMinElevation(gpsprot.DegreesFromFloat(10))
	props.SetAntennaCableDelay(50 * time.Nanosecond)
	rcvr := &gpsprot.ReceiverInfo{
		Vendor:        "u-blox",
		Firmware:      "HPG 1.51 PROTVER 27.50",
		Hardware:      "ZED-F9P",
		SupportedGNSS: gpsprot.GNSSSetOf(gpsprot.GPS) | gpsprot.GNSSSetOf(gpsprot.GAL),
	}
	tests := []struct {
		name   string
		rslt   *gpscfg.Result
		err    error
		expect map[string]any
	}{
		{
			name: "full result",
			rslt: &gpscfg.Result{
				ReceiverInfo:          rcvr,
				ConfigSupport:         gpsprot.ConfigSupportSignal | gpsprot.ConfigSupportSpeed,
				ConfigProps:           props,
				PacketFormatsDetected: []gpsprot.Tag{"UBX", "NMEA"},
			},
			expect: map[string]any{
				"receiver": map[string]any{
					"vendor":        "u-blox",
					"firmware":      "HPG 1.51 PROTVER 27.50",
					"hardware":      "ZED-F9P",
					"supportedGNSS": []any{"GPS", "GAL"},
				},
				"supports":      []any{"signal", "speed"},
				"packetFormats": []any{"UBX", "NMEA"},
				"config": map[string]any{
					"signalsEnabled": map[string]any{"GPS": []any{"L1", "L2C"}},
					"timeGNSS":       "GPS",
					"timePulse": map[string]any{
						"width":          0.1,
						"period":         1.0,
						"alignToGNSS":    true,
						"onlyWhenLocked": true,
						"polarityRising": true,
					},
					"mode": map[string]any{
						"static":       true,
						"fixedPosECEF": []any{-1144698.0, 6090335.5, 1504171.25},
						"fixedPosAcc":  10.0,
					},
					"minElevation":      10.0,
					"antennaCableDelay": 50.0,
				},
			},
		},
		{
			name:   "error without result",
			err:    errors.New("GPS detection failed"),
			expect: map[string]any{"error": "GPS detection failed"},
		},
		{
			name: "error with partial result",
			rslt: &gpscfg.Result{
				ReceiverInfo:          rcvr,
				ConfigProps:           &gpsprot.ConfigProps{},
				PacketFormatsDetected: []gpsprot.Tag{"UBX"},
			},
			err: errors.New("configuration failed"),
			expect: map[string]any{
				"receiver": map[string]any{
					"vendor":        "u-blox",
					"firmware":      "HPG 1.51 PROTVER 27.50",
					"hardware":      "ZED-F9P",
					"supportedGNSS": []any{"GPS", "GAL"},
				},
				"supports":      []any{},
				"packetFormats": []any{"UBX"},
				"error":         "configuration failed",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "json-out-*.json")
			if err != nil {
				t.Fatal(err)
			}
			printJSON(f, tc.rslt, tc.err)
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(f.Name())
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput: %s", err, b)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
