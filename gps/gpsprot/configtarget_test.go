package gpsprot

import (
	"encoding/json"
	"reflect"
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

	for i := range 3 {
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

func TestConfigPropsBaudRate(t *testing.T) {
	t.Run("CopyFrom", func(t *testing.T) {
		var dst, src ConfigProps
		src.SetBaudRate(115200)
		dst.CopyFrom(&src)
		if v, ok := dst.GetBaudRate(); !ok || v != 115200 {
			t.Errorf("after CopyFrom: got (%d, %v), want (115200, true)", v, ok)
		}
	})
	t.Run("Inconsistent", func(t *testing.T) {
		var a, b ConfigProps
		a.SetBaudRate(9600)
		b.SetBaudRate(38400)
		got := a.Inconsistent(&b)
		if v, ok := got.GetBaudRate(); !ok || v != 38400 {
			t.Errorf("Inconsistent: got (%d, %v), want (38400, true)", v, ok)
		}
		var c ConfigProps
		c.SetBaudRate(9600)
		got = a.Inconsistent(&c)
		if _, ok := got.GetBaudRate(); ok {
			t.Errorf("Inconsistent with equal values must not flag")
		}
	})
	t.Run("Missing", func(t *testing.T) {
		var a, b ConfigProps
		b.SetBaudRate(9600)
		got := a.Missing(&b)
		if v, ok := got.GetBaudRate(); !ok || v != 9600 {
			t.Errorf("Missing: got (%d, %v), want (9600, true)", v, ok)
		}
	})
}

func TestConfigPropsPort(t *testing.T) {
	t.Run("GetSet", func(t *testing.T) {
		var cp ConfigProps
		if _, ok := cp.GetPort(); ok {
			t.Errorf("port should be unset on a zero ConfigProps")
		}
		cp.SetPort("USB")
		v, ok := cp.GetPort()
		if !ok || v != "USB" {
			t.Errorf("GetPort: got (%q, %v), want (\"USB\", true)", v, ok)
		}
	})
	t.Run("ReadOnlyProps", func(t *testing.T) {
		var cp ConfigProps
		if ro := cp.ReadOnlyProps(); ro != 0 {
			t.Errorf("ReadOnlyProps: got %v, want 0", ro)
		}
		cp.SetPort("UART1")
		if ro := cp.ReadOnlyProps(); ro != PropIDPort {
			t.Errorf("ReadOnlyProps after SetPort: got %v, want PropIDPort", ro)
		}
		cp.SetBaudRate(9600)
		if ro := cp.ReadOnlyProps(); ro != PropIDPort {
			t.Errorf("ReadOnlyProps must only include PropIDsReadOnly bits: got %v", ro)
		}
	})
	t.Run("ClearReadOnlyProps", func(t *testing.T) {
		var cp ConfigProps
		cp.SetPort("UART1")
		cp.SetBaudRate(9600)
		cp.ClearReadOnlyProps()
		if _, ok := cp.GetPort(); ok {
			t.Errorf("port should be cleared")
		}
		if _, ok := cp.GetBaudRate(); !ok {
			t.Errorf("baud rate must not be cleared (not a read-only prop)")
		}
	})
	t.Run("CopyFrom", func(t *testing.T) {
		var dst, src ConfigProps
		src.SetPort("UART2")
		dst.CopyFrom(&src)
		if v, ok := dst.GetPort(); !ok || v != "UART2" {
			t.Errorf("after CopyFrom: got (%q, %v), want (\"UART2\", true)", v, ok)
		}
	})
	t.Run("Inconsistent", func(t *testing.T) {
		var a, b ConfigProps
		a.SetPort("UART1")
		b.SetPort("USB")
		got := a.Inconsistent(&b)
		if v, ok := got.GetPort(); !ok || v != "USB" {
			t.Errorf("Inconsistent: got (%q, %v), want (\"USB\", true)", v, ok)
		}
		var c ConfigProps
		c.SetPort("UART1")
		got = a.Inconsistent(&c)
		if _, ok := got.GetPort(); ok {
			t.Errorf("Inconsistent with equal port values must not flag")
		}
	})
	t.Run("Missing", func(t *testing.T) {
		var a, b ConfigProps
		b.SetPort("USB")
		got := a.Missing(&b)
		if v, ok := got.GetPort(); !ok || v != "USB" {
			t.Errorf("Missing: got (%q, %v), want (\"USB\", true)", v, ok)
		}
	})
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

func TestConfigPropsJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		build func() ConfigProps
	}{
		{
			"signals only",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetSignalsEnabled(SignalSetOf(SigGPSL1CA, SigGPSL2C, SigGALE1, SigGALE5b))
				return cp
			},
		},
		{
			"timeGNSS",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetTimeGNSS(GAL)
				return cp
			},
		},
		{
			"timePulse all fields",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetTimePulse(TimePulse{
					Width:          100 * time.Microsecond,
					Period:         1 * time.Second,
					AlignToGNSS:    true,
					OnlyWhenLocked: true,
					PolarityRising: true,
				})
				return cp
			},
		},
		{
			"timePulse partial",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetTimePulsePeriod(1 * time.Second)
				return cp
			},
		},
		{
			"mode ECEF",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetMode(Mode{
					Static:       true,
					PosType:      PosTypeECEF,
					FixedPosECEF: Point3D{Meters(4000000), Meters(500000), Meters(4800000)},
					FixedPosAcc:  Meters(0.1),
				})
				return cp
			},
		},
		{
			"mode ECEF no accuracy stated",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetMode(Mode{
					Static:       true,
					PosType:      PosTypeECEF,
					FixedPosECEF: Point3D{Meters(4000000), Meters(500000), Meters(4800000)},
				})
				return cp
			},
		},
		{
			"mode LLH",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetMode(Mode{
					Static:      true,
					PosType:     PosTypeLLH,
					FixedPosLLH: [2]Angle{DegreesFromFloat(47.5), DegreesFromFloat(8.75)},
					Height:      Meters(450),
					FixedPosAcc: Meters(0.05),
				})
				return cp
			},
		},
		{
			"mode static no position",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetMode(Mode{Static: true})
				return cp
			},
		},
		{
			"antennaCableDelay",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetAntennaCableDelay(50 * time.Nanosecond)
				return cp
			},
		},
		{
			"navMsgAuth OSNMA",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetNavMsgAuth(NavMsgAuthOSNMA)
				return cp
			},
		},
		{
			"navMsgAuth none",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetNavMsgAuth(NavMsgAuthNone)
				return cp
			},
		},
		{
			"rtcmBaseID",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetRTCMBaseID(1)
				return cp
			},
		},
		{
			"minElevation",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetMinElevation(DegreesFromFloat(10))
				return cp
			},
		},
		{
			"baudRate",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetBaudRate(9600)
				return cp
			},
		},
		{
			"baudRate zero",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetBaudRate(0)
				return cp
			},
		},
		{
			"port",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetPort("USB")
				return cp
			},
		},
		{
			"all properties",
			func() ConfigProps {
				var cp ConfigProps
				cp.SetSignalsEnabled(SignalSetOf(SigGPSL1CA, SigGPSL5, SigGALE1, SigGALE5b))
				cp.SetTimeGNSS(GPS)
				cp.SetTimePulse(TimePulse{
					Width:          100 * time.Microsecond,
					Period:         1 * time.Second,
					AlignToGNSS:    true,
					OnlyWhenLocked: true,
					PolarityRising: true,
				})
				cp.SetMode(Mode{
					Static:       true,
					PosType:      PosTypeECEF,
					FixedPosECEF: Point3D{Meters(4000000), Meters(500000), Meters(4800000)},
					FixedPosAcc:  Meters(0.1),
				})
				cp.SetAntennaCableDelay(50 * time.Nanosecond)
				cp.SetNavMsgAuth(NavMsgAuthOSNMA)
				cp.SetRTCMBaseID(1)
				cp.SetMinElevation(DegreesFromFloat(10))
				return cp
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := tc.build()
			data, err := json.Marshal(&orig)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got ConfigProps
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if got.valid != orig.valid {
				t.Errorf("valid mismatch: got %s, want %s", got.valid, orig.valid)
			}
			if got != orig {
				t.Errorf("round-trip mismatch:\n  json: %s\n  got:  %+v\n  want: %+v", data, got, orig)
			}
		})
	}
}

func TestConfigPropsUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown key", `{"bogus": 1}`},
		{"bad timeGNSS", `{"timeGNSS": "BOGUS"}`},
		{"bad navMsgAuth", `{"navMsgAuth": "BOGUS"}`},
		{"bad signal GNSS", `{"signalsEnabled": {"BOGUS": ["L1"]}}`},
		{"bad signal name", `{"signalsEnabled": {"GPS": ["X99"]}}`},
		{"bad timePulse field", `{"timePulse": {"bogus": 1}}`},
		{"bad mode field", `{"mode": {"bogus": 1}}`},
		{"bad minElevation type", `{"minElevation": "ten"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cp ConfigProps
			if err := json.Unmarshal([]byte(tc.json), &cp); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestPropIDsJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ids  PropIDs
	}{
		{"empty", 0},
		{"single", PropIDSignalsEnabled},
		{"timePulse combined", PropIDTimePulse},
		{"timePulse single", PropIDTimePulseWidth},
		{"timePulse partial", PropIDTimePulseWidth | PropIDTimePulsePeriod},
		{"multiple", PropIDSignalsEnabled | PropIDMode | PropIDMinElevation},
		{"baudRate", PropIDBaudRate},
		{"port", PropIDPort},
		{"all", PropIDSignalsEnabled | PropIDTimeGNSS | PropIDTimePulse | PropIDMode |
			PropIDAntennaCableDelay | PropIDNavMsgAuth | PropIDRTCMBaseID | PropIDMinElevation | PropIDBaudRate | PropIDPort},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ids)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got PropIDs
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if got != tc.ids {
				t.Errorf("round-trip mismatch: got %s, want %s (json: %s)", got, tc.ids, data)
			}
		})
	}
}

func TestPropIDsUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown name", `["bogus"]`},
		{"not array", `"signalsEnabled"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ids PropIDs
			if err := json.Unmarshal([]byte(tc.json), &ids); err == nil {
				t.Error("expected error, got nil")
			}
		})
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

func TestConfigFlagJSON(t *testing.T) {
	tests := []struct {
		name    string
		empty   any
		one     any
		all     any
		unknown any
		new     func() any
		oneJSON string
		allJSON string
	}{
		{"NMEAMsgFlags", NMEAMsgFlags(0), NMEAMsgGGA, NMEAMsgAny, NMEAMsgFlags(1 << 14), func() any { return new(NMEAMsgFlags) }, `["GGA"]`, `["RMC","GGA","GSA","GSV","ZDA","VTG","GLL","other"]`},
		{"RTCMMsgFlags", RTCMMsgFlags(0), RTCMMsgMSM7, RTCMMsgMSM4 | RTCMMsgMSM7 | RTCMMsgARP | RTCMMsgLax | RTCMMsgOther, RTCMMsgFlags(1 << 1), func() any { return new(RTCMMsgFlags) }, `["MSM7"]`, `["MSM4","MSM7","ARP","lax","other"]`},
		{"PVTMsgFlags", PVTMsgFlags(0), PVTMsgTimePulseAfter, PVTMsgPos | PVTMsgVel | PVTMsgTime | PVTMsgTimePulse | PVTMsgLeapSecond | PVTMsgSurvey | PVTMsgTAI | PVTMsgECEF | PVTMsgTimePulseAfter | PVTMsgQuality | PVTMsgEpoch | PVTMsgOff, PVTMsgFlags(1 << 15), func() any { return new(PVTMsgFlags) }, `["timePulseAfter"]`, `["pos","vel","time","timePulse","leapSecond","survey","tai","ecef","timePulseAfter","quality","epoch","off"]`},
		{"SatsMsgFlags", SatsMsgFlags(0), SatsMsgSignal, SatsMsgAny, SatsMsgFlags(1 << 7), func() any { return new(SatsMsgFlags) }, `["signal"]`, `["sat","signal"]`},
		{"RawMsgFlags", RawMsgFlags(0), RawMsgNavData, RawMsgAny, RawMsgFlags(1 << 7), func() any { return new(RawMsgFlags) }, `["navData"]`, `["obs","navData"]`},
		{"SurveyFlags", SurveyFlags(0), SurveyAgain, SurveyAgain, SurveyFlags(1 << 1), func() any { return new(SurveyFlags) }, `["again"]`, `["again"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				val  any
				json string
			}{
				{"empty", tc.empty, `[]`},
				{"one", tc.one, tc.oneJSON},
				{"all", tc.all, tc.allJSON},
			} {
				t.Run(test.name, func(t *testing.T) {
					data, err := json.Marshal(test.val)
					if err != nil {
						t.Fatalf("Marshal: %v", err)
					}
					if string(data) != test.json {
						t.Errorf("JSON = %s, want %s", data, test.json)
					}
					got := tc.new()
					if err := json.Unmarshal(data, got); err != nil {
						t.Fatalf("Unmarshal: %v", err)
					}
					if !reflect.DeepEqual(reflect.ValueOf(got).Elem().Interface(), test.val) {
						t.Errorf("round trip = %v, want %v", reflect.ValueOf(got).Elem(), test.val)
					}
				})
			}
			if err := json.Unmarshal([]byte(`["unknown"]`), tc.new()); err == nil {
				t.Error("unknown name did not return an error")
			}
			if err := json.Unmarshal([]byte(`null`), tc.new()); err == nil {
				t.Error("null did not return an error")
			}
			if _, err := json.Marshal(tc.unknown); err == nil {
				t.Error("unknown bit did not return an error")
			}
		})
	}
}

func TestConfigEnumJSON(t *testing.T) {
	tests := []struct {
		name string
		val  any
		new  func() any
		json string
	}{
		{"save none", SaveNone, func() any { return new(SaveType) }, `"none"`},
		{"save minimal", SaveMinimal, func() any { return new(SaveType) }, `"minimal"`},
		{"save all", SaveAll, func() any { return new(SaveType) }, `"all"`},
		{"reset none", ResetNone, func() any { return new(ResetType) }, `"none"`},
		{"reset reload", ResetReload, func() any { return new(ResetType) }, `"reload"`},
		{"reset cold", ResetCold, func() any { return new(ResetType) }, `"cold"`},
		{"reset factory", ResetFactory, func() any { return new(ResetType) }, `"factory"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.val)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(data) != tc.json {
				t.Errorf("JSON = %s, want %s", data, tc.json)
			}
			got := tc.new()
			if err := json.Unmarshal(data, got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(reflect.ValueOf(got).Elem().Interface(), tc.val) {
				t.Errorf("round trip = %v, want %v", reflect.ValueOf(got).Elem(), tc.val)
			}
			if err := json.Unmarshal([]byte(`"unknown"`), tc.new()); err == nil {
				t.Error("unknown name did not return an error")
			}
			if err := json.Unmarshal([]byte(`1`), tc.new()); err == nil {
				t.Error("numeric value did not return an error")
			}
		})
	}
}

func TestConfigTargetJSONRoundTrip(t *testing.T) {
	target := NewConfigTarget()
	target.Props.SetMode(Mode{Static: true})
	target.Get = PropIDSignalsEnabled | PropIDTimePulse
	target.Opts.Save = SaveMinimal
	target.Opts.Reset = ResetCold
	target.Opts.PVTMsg = PVTMsgTimePulse | PVTMsgTimePulseAfter
	target.Opts.NMEAMsg.Set(NMEAMsgNone)
	target.Opts.RTCMMsg.Set(RTCMMsgMSM7 | RTCMMsgARP)
	target.Opts.SatsMsg.Set(SatsMsgAny)
	target.Opts.RawMsg.Set(RawMsgObs)
	target.Opts.Survey.Flags = SurveyAgain
	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got ConfigTarget
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(%s): %v", data, err)
	}
	if got != *target {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, *target)
	}
	if !got.Opts.NMEAMsg.IsSet() || got.Opts.NMEAMsg.Get() != NMEAMsgNone {
		t.Errorf("set-empty NMEAMsg did not round trip: %+v", got.Opts.NMEAMsg)
	}
	if got.Opts.TimeAssist != (TimeEstimate{}) {
		t.Errorf("unset option changed: %+v", got.Opts.TimeAssist)
	}
}

func TestFixedPosToECEF(t *testing.T) {
	llh := Mode{Static: true, PosType: PosTypeLLH}
	if p, ok := llh.FixedPosToECEF(); !ok || p != (Point3D{Meters(6378137), 0, 0}) {
		t.Errorf("LLH origin = %v,%v, want the WGS84 semi-major axis on X", p, ok)
	}
	ecef := Mode{Static: true, PosType: PosTypeECEF, FixedPosECEF: Point3D{1, 2, 3}}
	if p, ok := ecef.FixedPosToECEF(); !ok || p != ecef.FixedPosECEF {
		t.Errorf("ECEF = %v,%v, want pass-through", p, ok)
	}
	if _, ok := (Mode{Static: true}).FixedPosToECEF(); ok {
		t.Error("PosTypeNone reported a position")
	}
}
