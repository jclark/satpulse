package gpsprot

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestConfigProps(t *testing.T) {
	cp := new(ConfigProps)

	// Set some values directly with setter methods
	cp.SetTimePulseWidth(1 * time.Millisecond)
	cp.SetTimePulseAlignToGNSS(true)

	validProps := []PropIDs{PropIDTimePulseWidth, PropIDTimePulseAlignToGNSS}
	invalidProps := []PropIDs{PropIDTimePulsePeriod, PropIDMode}

	timePulseWidth, ok := cp.GetTimePulseWidth()
	if !ok {
		t.Errorf("expected timePulseWidth to be set")
	}
	if timePulseWidth != 1*time.Millisecond {
		t.Errorf("expected timePulseWidth to be 1ms, got %v", timePulseWidth)
	}

	timePulsePeriod, ok := cp.GetTimePulsePeriod()
	if ok {
		t.Errorf("expected timePulsePeriod not to be set")
	}
	if timePulsePeriod != 0 {
		t.Errorf("expected timePulsePeriod to be 0, got %v", timePulsePeriod)
	}

	timePulseGNSS, ok := cp.GetTimePulseAlignToGNSS()
	if !ok {
		t.Errorf("expected timePulseAlignToGNSS to be set")
	}
	if !timePulseGNSS {
		t.Error("expected timePulseAlignToGNSS to be true, got false")
	}

	for _, propID := range validProps {
		if cp.valid&propID == 0 {
			t.Errorf("expected PropID %v to be set", propID)
		}
	}

	for _, propID := range invalidProps {
		if cp.valid&propID != 0 {
			t.Errorf("expected PropID %v not to be set", propID)
		}
	}
}

func TestPoint3DRoundTrip(t *testing.T) {
	// Typical ECEF coordinates for New York
	originalPoint := Point3D{1334195 * Meter, -4652309 * Meter, 4138066 * Meter}

	pointStr := originalPoint.String()

	parsedPoint, err := ParsePoint3D(pointStr)
	if err != nil {
		t.Fatalf("ParsePoint3D returned error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if parsedPoint[i] != originalPoint[i] {
			t.Errorf("Expected coordinate %d to be %v, got %v", i, originalPoint[i], parsedPoint[i])
		}
	}

	p, err := ParsePoint3D("1,2,3 ")
	if err != nil {
		t.Errorf("ParsePoint3D returned an error for trailing space")
	} else {
		if p.String() != "1,2,3" {
			t.Errorf("ParsePoint3D returned %v for trailing space", p.String())
		}
	}
}

func TestPropIDOperations(t *testing.T) {
	props := PropIDSignalsEnabled | PropIDTimePulsePeriod

	if props&PropIDSignalsEnabled == 0 {
		t.Errorf("expected PropIDSignalsEnabled to be in the bitfield")
	}

	if props&PropIDTimePulsePeriod == 0 {
		t.Errorf("expected PropIDTimePulsePeriod to be in the bitfield")
	}

	if props&PropIDTimePulseWidth != 0 {
		t.Errorf("expected PropIDTimePulseWidth not to be in the bitfield")
	}
}

func TestConfigOptionsJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		opts ConfigOptions
	}{
		{"empty", ConfigOptions{}},
		{"with NMEAMsg", ConfigOptions{NMEAMsg: opt.Make(NMEAMsgRMC)}},
		{"with RTCMMsg", ConfigOptions{RTCMMsg: opt.Make(RTCMMsgMSM4 | RTCMMsgARP)}},
		{"with SatsMsg", ConfigOptions{SatsMsg: opt.Make(SatsMsgSat | SatsMsgSignal)}},
		{"with RawMsg", ConfigOptions{RawMsg: opt.Make(RawMsgObs)}},
		{"with all msg flags", ConfigOptions{
			NMEAMsg: opt.Make(NMEAMsgRMC | NMEAMsgGGA),
			RTCMMsg: opt.Make(RTCMMsgMSM7),
			SatsMsg: opt.Make(SatsMsgSat),
			RawMsg:  opt.Make(RawMsgObs | RawMsgNavData),
		}},
		{"with zero values set", ConfigOptions{
			NMEAMsg: opt.Make(NMEAMsgNone),
			RTCMMsg: opt.Make(RTCMMsgNone),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.opts)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got ConfigOptions
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got != tc.opts {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, tc.opts)
			}
		})
	}
}
