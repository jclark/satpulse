package gpsprot

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestGNSSSet(t *testing.T) {
	// Create a new GNSSSet
	set := GNSSSetOf(GPS, GLO)

	// Check that Contains works properly for values in the set
	if !set.Contains(GPS) {
		t.Errorf("expected set to contain GPS")
	}
	if !set.Contains(GLO) {
		t.Errorf("expected set to contain GLONASS")
	}

	// Check that Contains works properly for values not in the set
	if set.Contains(BDS) {
		t.Errorf("expected set to not contain BeiDou")
	}
	if set.Contains(GAL) {
		t.Errorf("expected set to not contain Galileo")
	}

	// Check that Contains works properly for the 0 value
	if set.Contains(0) {
		t.Errorf("expected set to not contain 0")
	}

	// Check that Items returns the correct values
	items := set.Items()
	if len(items) != 2 {
		t.Errorf("expected Items to return 2 items, got %d", len(items))
	}
	if items[0] != GPS {
		t.Errorf("expected first item to be GPS, got %v", items[0])
	}
	if items[1] != GLO {
		t.Errorf("expected second item to be GLONASS, got %v", items[1])
	}
}

func TestGNSSSetMarshalJSON(t *testing.T) {
	gnssSet := GNSSSetOf(GPS) | GNSSSetOf(GAL)

	marshaledJSON, err := gnssSet.MarshalJSON()

	if err != nil {
		t.Errorf("MarshalJSON returned error: %v", err)
	}

	expectedJSON := `["GPS","GAL"]`
	if string(marshaledJSON) != expectedJSON {
		t.Errorf("Expected marshaled JSON to be %v, got %v", expectedJSON, string(marshaledJSON))
	}
}

func TestBandJSONMarshal(t *testing.T) {
	tests := []struct {
		band Band
		want string
	}{
		{0, "null"},
		{BandL1, `["L1"]`},
		{BandL1 | BandL5, `["L1","L5"]`},
		{BandL5 | BandE5b, `["L5","E5b"]`},
		{BandL1 | BandL2 | BandE6, `["L1","L2","E6"]`},
		{BandL1 | BandL5 | BandE5b | BandE6, `["L1","L5","E5b","E6"]`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(tt.band)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tt.band, err)
		}
		if got := string(b); got != tt.want {
			t.Errorf("Marshal(%v) = %s, want %s", tt.band, got, tt.want)
		}
	}
}

func TestBandJSONUnmarshal(t *testing.T) {
	tests := []struct {
		json string
		want Band
	}{
		{`["L1"]`, BandL1},
		{`["L1","L5"]`, BandL1 | BandL5},
		{`["E5"]`, BandL5 | BandE5b},
		{`["L6"]`, BandE6},
		{`["E5","L1","E6"]`, BandL1 | BandL5 | BandE5b | BandE6},
	}
	for _, tt := range tests {
		var got Band
		if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tt.json, err)
		}
		if got != tt.want {
			t.Errorf("Unmarshal(%s) = %v, want %v", tt.json, got, tt.want)
		}
	}
}

func TestBandJSONUnmarshalError(t *testing.T) {
	tests := []string{
		`["BOGUS"]`,
		`["L1","INVALID"]`,
		`"L1"`,
	}
	for _, input := range tests {
		var b Band
		if err := json.Unmarshal([]byte(input), &b); err == nil {
			t.Errorf("Unmarshal(%s): expected error, got nil", input)
		}
	}
}

func TestFixLevelString(t *testing.T) {
	tests := []struct {
		f    FixLevel
		want string
	}{
		{0, "FixLevel(0)"},
		{FixLevelNone, "none"},
		{FixLevelNotMeasured, "notMeasured"},
		{FixLevelDoppler, "doppler"},
		{FixLevelCode, "code"},
		{FixLevelCarrierFloat, "carrierFloat"},
		{FixLevelCarrierFixed, "carrierFixed"},
	}
	for _, tt := range tests {
		if got := tt.f.String(); got != tt.want {
			t.Errorf("FixLevel(%d).String() = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestFixLevelJSON(t *testing.T) {
	b, err := json.Marshal(FixLevelCarrierFixed)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `"carrierFixed"` {
		t.Errorf("MarshalJSON = %s, want %q", got, "carrierFixed")
	}
}

func TestFixLevelRoundTrip(t *testing.T) {
	for f := FixLevelNone; f <= FixLevelCarrierFixed; f++ {
		text, err := f.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%d): %v", f, err)
		}
		var got FixLevel
		if err := got.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if got != f {
			t.Errorf("round-trip %d: got %d", f, got)
		}
	}
}

func TestSolutionDimString(t *testing.T) {
	tests := []struct {
		d    SolutionDim
		want string
	}{
		{0, "SolutionDim(0)"},
		{SolutionDim2D, "2D"},
		{SolutionDim3D, "3D"},
		{SolutionDimTimeOnly, "timeOnly"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("SolutionDim(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestSolutionDimJSON(t *testing.T) {
	b, err := json.Marshal(SolutionDim3D)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `"3D"` {
		t.Errorf("MarshalJSON = %s, want %q", got, "3D")
	}
}

func TestSolutionDimRoundTrip(t *testing.T) {
	for d := SolutionDim2D; d <= SolutionDimTimeOnly; d++ {
		text, err := d.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%d): %v", d, err)
		}
		var got SolutionDim
		if err := got.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if got != d {
			t.Errorf("round-trip %d: got %d", d, got)
		}
	}
}

func TestAuxSrcJSON(t *testing.T) {
	tests := []struct {
		a    AuxSrc
		want string
	}{
		{0, "null"},
		{AuxSrcDR, `["DR"]`},
		{AuxSrcINS, `["INS"]`},
		{AuxSrcDR | AuxSrcINS, `["DR","INS"]`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(tt.a)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tt.a, err)
		}
		if got := string(b); got != tt.want {
			t.Errorf("AuxSrc(%d) JSON = %s, want %s", tt.a, got, tt.want)
		}
	}
}

func TestAuxSrcString(t *testing.T) {
	tests := []struct {
		a    AuxSrc
		want string
	}{
		{0, "(none)"},
		{AuxSrcDR, "DR"},
		{AuxSrcDR | AuxSrcINS, "DR,INS"},
	}
	for _, tt := range tests {
		if got := tt.a.String(); got != tt.want {
			t.Errorf("AuxSrc(%d).String() = %q, want %q", tt.a, got, tt.want)
		}
	}
}

func TestCorrKindClose(t *testing.T) {
	tests := []struct {
		in   CorrKind
		want CorrKind
	}{
		{0, 0},
		{CorrUsed, CorrUsed},
		{CorrOSR, CorrOSR | CorrUsed},
		{CorrSBAS, CorrSBAS | CorrSSR | CorrUsed},
		{CorrFullDualFreq, CorrFullDualFreq | CorrPartialDualFreq | CorrOSR | CorrUsed},
		{CorrPPPRTK, CorrPPPRTK | CorrPPP | CorrSSR | CorrUsed},
		{CorrPPPConverging, CorrPPPConverging | CorrPPP | CorrSSR | CorrUsed},
		{CorrPPPHAS, CorrPPPHAS | CorrPPP | CorrSSR | CorrUsed},
		{CorrPPPMDC, CorrPPPMDC | CorrPPP | CorrSSR | CorrUsed},
		{CorrPPPB2b, CorrPPPB2b | CorrPPP | CorrSSR | CorrUsed},
		// Multiple bits: union of closures
		{CorrSBAS | CorrRTCM, CorrSBAS | CorrSSR | CorrRTCM | CorrUsed},
	}
	for _, tt := range tests {
		if got := tt.in.Expand(); got != tt.want {
			t.Errorf("CorrKind(%#x).Expand() = %#x, want %#x", uint16(tt.in), uint16(got), uint16(tt.want))
		}
	}
}

func TestCorrKindString(t *testing.T) {
	tests := []struct {
		c    CorrKind
		want string
	}{
		{0, "(none)"},
		{CorrUsed, "used"},
		// OSR implies used, so only "OSR"
		{CorrOSR | CorrUsed, "OSR"},
		// SBAS implies SSR implies used
		{CorrSBAS | CorrSSR | CorrUsed, "SBAS"},
		// fullDualFreq implies partialDualFreq implies OSR implies used
		{CorrFullDualFreq | CorrPartialDualFreq | CorrOSR | CorrUsed, "fullDualFreq"},
		// Two independent leaves: SBAS + RTCM
		{CorrSBAS | CorrRTCM | CorrSSR | CorrUsed, "RTCM,SBAS"},
		// PPP-RTK implies PPP implies SSR implies used
		{CorrPPPRTK | CorrPPP | CorrSSR | CorrUsed, "PPP-RTK"},
		// PPPConverged + RTCM: two independent leaves
		{CorrPPPConverged | CorrPPP | CorrSSR | CorrRTCM | CorrUsed, "RTCM,PPPConverged"},
		// PPP service-specific leaves
		{CorrPPPHAS | CorrPPP | CorrSSR | CorrUsed, "PPP-HAS"},
		{CorrPPPMDC | CorrPPP | CorrSSR | CorrUsed, "PPP-MDC"},
		{CorrPPPB2b | CorrPPP | CorrSSR | CorrUsed, "PPP-B2b"},
		// PPP service + convergence state: two independent leaves
		{CorrPPPHAS | CorrPPPConverging | CorrPPP | CorrSSR | CorrUsed, "PPPConverging,PPP-HAS"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("CorrKind(%#x).String() = %q, want %q", uint16(tt.c), got, tt.want)
		}
	}
}

func TestCorrKindLeaves(t *testing.T) {
	tests := []struct {
		in   CorrKind
		want CorrKind
	}{
		{0, 0},
		{CorrUsed, CorrUsed},
		// OSR implies used, so used is stripped
		{CorrOSR | CorrUsed, CorrOSR},
		// SBAS implies SSR implies used
		{CorrSBAS | CorrSSR | CorrUsed, CorrSBAS},
		// fullDualFreq implies partialDualFreq implies OSR implies used
		{CorrFullDualFreq | CorrPartialDualFreq | CorrOSR | CorrUsed, CorrFullDualFreq},
		// Two independent leaves preserved
		{CorrSBAS | CorrRTCM | CorrSSR | CorrUsed, CorrSBAS | CorrRTCM},
		// PPPConverged + PPP-HAS: two independent leaves under PPP
		{CorrPPPHAS | CorrPPPConverging | CorrPPP | CorrSSR | CorrUsed, CorrPPPHAS | CorrPPPConverging},
	}
	for _, tt := range tests {
		if got := tt.in.Leaves(); got != tt.want {
			t.Errorf("CorrKind(%#x).Leaves() = %#x, want %#x", uint16(tt.in), uint16(got), uint16(tt.want))
		}
	}
}

func TestCorrKindNames(t *testing.T) {
	tests := []struct {
		c    CorrKind
		want string
	}{
		{0, ""},
		{CorrUsed, "used"},
		{CorrSBAS, "SBAS"},
		{CorrSBAS | CorrSSR | CorrUsed, "used,SSR,SBAS"},
		{CorrPPPRTK | CorrPPP, "PPP,PPP-RTK"},
	}
	for _, tt := range tests {
		got := strings.Join(tt.c.Names(), ",")
		if got != tt.want {
			t.Errorf("CorrKind(%#x).Names() = %q, want %q", uint16(tt.c), got, tt.want)
		}
	}
}

func TestCorrKindJSON(t *testing.T) {
	c := CorrSBAS | CorrSSR | CorrUsed
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `["SBAS"]` {
		t.Errorf("MarshalJSON = %s, want %s", got, `["SBAS"]`)
	}
}

func TestCorrKindJSONRoundTrip(t *testing.T) {
	tests := []CorrKind{
		CorrUsed,
		CorrSBAS | CorrSSR | CorrUsed,
		CorrFullDualFreq | CorrPartialDualFreq | CorrOSR | CorrUsed,
		CorrPPPConverged | CorrPPP | CorrSSR | CorrRTCM | CorrUsed,
		CorrPPPRTK | CorrPPP | CorrSSR | CorrUsed,
		CorrPPPHAS | CorrPPP | CorrSSR | CorrUsed,
		CorrPPPMDC | CorrPPP | CorrSSR | CorrUsed,
		CorrPPPB2b | CorrPPP | CorrSSR | CorrUsed,
		CorrPPPHAS | CorrPPPConverged | CorrPPP | CorrSSR | CorrUsed,
	}
	for _, orig := range tests {
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal(%#x): %v", uint16(orig), err)
		}
		var got CorrKind
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if got != orig {
			t.Errorf("round-trip %#x: marshal=%s, got=%#x", uint16(orig), b, uint16(got))
		}
	}
}

func TestPosGeoMergeHigherOverwrites(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Height:   opt.Make(100 * Meter),
		Priority: PriGenericLow,
		Tag:      "NMEA",
	}
	other := &PosGeoMsg{
		LatLon:    [2]Angle{11 * Degrees, 21 * Degrees},
		HeightMSL: opt.Make(95 * Meter),
		Priority:  PriVendorHigh,
		Tag:       "UBX",
	}
	m.Merge(other)
	if m.LatLon[0] != 11*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", m.LatLon[0], 11*Degrees)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	if m.Priority != PriVendorHigh {
		t.Errorf("Priority = %v, want %v", m.Priority, PriVendorHigh)
	}
	// Height gone (higher priority copies everything, other has no Height)
	if m.Height.IsSet() {
		t.Error("Height should not be set after overwrite")
	}
	// HeightMSL from other
	if !m.HeightMSL.IsSet() || m.HeightMSL.Get() != 95*Meter {
		t.Errorf("HeightMSL = %v, want 95m set", m.HeightMSL)
	}
}

func TestPosGeoMergeLowerDoesNothing(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Height:   opt.Make(100 * Meter),
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}
	other := &PosGeoMsg{
		LatLon:    [2]Angle{11 * Degrees, 21 * Degrees},
		Height:    opt.Make(200 * Meter),
		HeightMSL: opt.Make(95 * Meter),
		Priority:  PriGenericLow,
		Tag:       "NMEA",
	}
	m.Merge(other)
	if m.LatLon[0] != 10*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", m.LatLon[0], 10*Degrees)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	if m.Height.Get() != 100*Meter {
		t.Errorf("Height = %v, want 100m", m.Height.Get())
	}
	// HeightMSL NOT filled (lower priority does nothing)
	if m.HeightMSL.IsSet() {
		t.Error("HeightMSL should not be set (lower priority does nothing)")
	}
}

func TestPosGeoMergeEqualFillsOnly(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Priority: PriVendorLow,
		Tag:      "A",
	}
	other := &PosGeoMsg{
		LatLon:    [2]Angle{11 * Degrees, 21 * Degrees},
		HeightMSL: opt.Make(95 * Meter),
		Priority:  PriVendorLow,
		Tag:       "B",
	}
	m.Merge(other)
	// First wins at equal priority: LatLon and Tag unchanged
	if m.LatLon[0] != 10*Degrees {
		t.Errorf("LatLon[0] = %v, want %v (first wins)", m.LatLon[0], 10*Degrees)
	}
	if m.Tag != "A" {
		t.Errorf("Tag = %v, want A (first wins)", m.Tag)
	}
	// HeightMSL filled (was unset)
	if !m.HeightMSL.IsSet() || m.HeightMSL.Get() != 95*Meter {
		t.Errorf("HeightMSL = %v, want 95m set", m.HeightMSL)
	}
}

func TestVelGeoMergeHigherOverwrites(t *testing.T) {
	m := &VelGeoMsg{
		GroundSpeed: opt.Make(5 * MeterPerSecond),
		Course:      opt.Make(90 * Degrees),
		Priority:    PriGenericLow,
		Tag:         "NMEA",
	}
	other := &VelGeoMsg{
		VelNED:      opt.Make([3]Speed{1, 2, 3}),
		GroundSpeed: opt.Make(6 * MeterPerSecond),
		Priority:    PriVendorHigh,
		Tag:         "UBX",
	}
	m.Merge(other)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	if m.GroundSpeed.Get() != 6*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 6", m.GroundSpeed.Get())
	}
	if !m.VelNED.IsSet() {
		t.Error("VelNED not set")
	}
	// Course from other (higher priority copies everything)
	if m.Course.IsSet() {
		t.Error("Course should not be set (other did not set it)")
	}
}

func TestVelGeoMergeLowerDoesNothing(t *testing.T) {
	m := &VelGeoMsg{
		GroundSpeed: opt.Make(5 * MeterPerSecond),
		Priority:    PriVendorHigh,
	}
	other := &VelGeoMsg{
		GroundSpeed: opt.Make(6 * MeterPerSecond),
		Course:      opt.Make(90 * Degrees),
		Priority:    PriGenericLow,
	}
	m.Merge(other)
	if m.GroundSpeed.Get() != 5*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 5", m.GroundSpeed.Get())
	}
	// Course NOT filled (lower priority does nothing)
	if m.Course.IsSet() {
		t.Error("Course should not be set (lower priority does nothing)")
	}
}

func TestPosECEFMerge(t *testing.T) {
	m := &PosECEFMsg{
		Pos:      Point3D{1, 2, 3},
		Priority: PriVendorLow,
		Tag:      "A",
	}
	other := &PosECEFMsg{
		Pos:      Point3D{4, 5, 6},
		Priority: PriVendorHigh,
		Tag:      "B",
	}
	m.Merge(other)
	if m.Pos != (Point3D{4, 5, 6}) {
		t.Errorf("Pos = %v, want {4,5,6}", m.Pos)
	}
	if m.Tag != "B" {
		t.Errorf("Tag = %v, want B", m.Tag)
	}
	// Lower priority does not overwrite
	low := &PosECEFMsg{Pos: Point3D{7, 8, 9}, Priority: PriGenericLow, Tag: "C"}
	m.Merge(low)
	if m.Pos != (Point3D{4, 5, 6}) {
		t.Errorf("Pos = %v, want {4,5,6} (lower priority)", m.Pos)
	}
	// Equal priority does not overwrite
	eq := &PosECEFMsg{Pos: Point3D{10, 11, 12}, Priority: PriVendorHigh, Tag: "D"}
	m.Merge(eq)
	if m.Pos != (Point3D{4, 5, 6}) {
		t.Errorf("Pos = %v, want {4,5,6} (equal priority, first wins)", m.Pos)
	}
}

func TestVelECEFMerge(t *testing.T) {
	m := &VelECEFMsg{
		Vel:      [3]Speed{1, 2, 3},
		Priority: PriVendorLow,
	}
	other := &VelECEFMsg{
		Vel:      [3]Speed{4, 5, 6},
		Priority: PriVendorHigh,
	}
	m.Merge(other)
	if m.Vel != [3]Speed{4, 5, 6} {
		t.Errorf("Vel = %v, want {4,5,6}", m.Vel)
	}
	// Lower priority does not overwrite
	low := &VelECEFMsg{Vel: [3]Speed{7, 8, 9}, Priority: PriGenericLow}
	m.Merge(low)
	if m.Vel != [3]Speed{4, 5, 6} {
		t.Errorf("Vel = %v, want {4,5,6} (lower priority)", m.Vel)
	}
}

func TestPVMsgAccumFirstStored(t *testing.T) {
	var a PVMsgAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow}, now)
	if !a.PVMsgBundle.PosGeo.IsSet() {
		t.Fatal("first PosGeo should be stored")
	}
	if a.PVMsgBundle.PosGeo.Get().LatLon[0] != 1 {
		t.Fatal("first PosGeo LatLon[0] wrong")
	}
}

func TestPVMsgAccumMerge(t *testing.T) {
	var a PVMsgAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{
		LatLon:   [2]Angle{1 * Degrees, 2 * Degrees},
		Priority: PriGenericLow,
		Tag:      "NMEA",
	}, now)
	a.PosGeo(&PosGeoMsg{
		LatLon:   [2]Angle{3 * Degrees, 4 * Degrees},
		Height:   opt.Make(50 * Meter),
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}, now)
	pg := a.PVMsgBundle.PosGeo.Get()
	if pg.LatLon[0] != 3*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", pg.LatLon[0], 3*Degrees)
	}
	if pg.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", pg.Tag)
	}
	if !pg.Height.IsSet() {
		t.Error("Height not set after merge")
	}
}

func TestPVMsgAccumClear(t *testing.T) {
	var a PVMsgAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}}, now)
	a.VelGeo(&VelGeoMsg{GroundSpeed: opt.Make(Speed(5))}, now)
	a.NavEpoch(&NavEpochMsg{}, now)
	if a.PVMsgBundle.PosGeo.IsSet() {
		t.Error("PosGeo not cleared")
	}
	if a.PVMsgBundle.VelGeo.IsSet() {
		t.Error("VelGeo not cleared")
	}
}

func TestPVMsgAccumIndependent(t *testing.T) {
	var a PVMsgAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow}, now)
	a.VelGeo(&VelGeoMsg{GroundSpeed: opt.Make(Speed(5)), Priority: PriVendorLow}, now)
	a.PosECEF(&PosECEFMsg{Pos: Point3D{10, 20, 30}, Priority: PriVendorHigh}, now)
	a.VelECEF(&VelECEFMsg{Vel: [3]Speed{1, 2, 3}, Priority: PriGenericHigh}, now)
	if !a.PVMsgBundle.PosGeo.IsSet() || !a.PVMsgBundle.VelGeo.IsSet() ||
		!a.PVMsgBundle.PosECEF.IsSet() || !a.PVMsgBundle.VelECEF.IsSet() {
		t.Error("not all message types accumulated")
	}
}

func TestPVMsgAccumAfterClear(t *testing.T) {
	var a PVMsgAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow, Tag: "old"}, now)
	a.NavEpoch(&NavEpochMsg{}, now)
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{3, 4}, Priority: PriVendorHigh, Tag: "new"}, now)
	if a.PVMsgBundle.PosGeo.Get().Tag != "new" {
		t.Errorf("Tag = %v, want new (fresh after clear)", a.PVMsgBundle.PosGeo.Get().Tag)
	}
}

func TestAccuracyFillSetsUnset(t *testing.T) {
	a := Accuracy{
		Pos:   opt.Make(10 * Meter),
		Hor:   opt.Make(5 * Meter),
		Speed: opt.Make(2 * MeterPerSecond),
	}
	b := Accuracy{
		Pos:         opt.Make(20 * Meter),
		Vert:        opt.Make(8 * Meter),
		GroundSpeed: opt.Make(3 * MeterPerSecond),
	}
	a.Fill(&b)
	// Pos already set, not overwritten
	if a.Pos.Get() != 10*Meter {
		t.Errorf("Pos = %v, want 10m (already set)", a.Pos.Get())
	}
	// Hor already set, kept
	if a.Hor.Get() != 5*Meter {
		t.Errorf("Hor = %v, want 5m", a.Hor.Get())
	}
	// Vert filled from b
	if a.Vert.Get() != 8*Meter {
		t.Errorf("Vert = %v, want 8m", a.Vert.Get())
	}
	// Speed already set, kept
	if a.Speed.Get() != 2*MeterPerSecond {
		t.Errorf("Speed = %v, want 2m/s", a.Speed.Get())
	}
	// GroundSpeed filled from b
	if a.GroundSpeed.Get() != 3*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 3m/s", a.GroundSpeed.Get())
	}
}

func TestAccuracyFillDoesNotOverwrite(t *testing.T) {
	a := Accuracy{
		Pos: opt.Make(10 * Meter),
	}
	b := Accuracy{
		Pos:  opt.Make(20 * Meter),
		Vert: opt.Make(8 * Meter),
	}
	a.Fill(&b)
	// Pos already set, not overwritten
	if a.Pos.Get() != 10*Meter {
		t.Errorf("Pos = %v, want 10m", a.Pos.Get())
	}
	// Vert filled (was unset)
	if a.Vert.Get() != 8*Meter {
		t.Errorf("Vert = %v, want 8m", a.Vert.Get())
	}
}

func TestMergeNavEpochNils(t *testing.T) {
	msg := &NavEpochMsg{Tag: "UBX"}
	m := MergeNavEpoch(nil, msg)
	if m != msg {
		t.Errorf("MergeNavEpoch(nil, msg) = %v; want msg", m)
	}
	m = MergeNavEpoch(msg, nil)
	if m != msg {
		t.Errorf("MergeNavEpoch(msg, nil) = %v; want msg", m)
	}
	m = MergeNavEpoch(nil, nil)
	if m != nil {
		t.Errorf("MergeNavEpoch(nil, nil) = %v; want nil", m)
	}
}

func TestMergeNavEpochFirstWins(t *testing.T) {
	// Higher priority (UBX) passed first; lower priority (NMEA) fills gaps.
	ubx := &NavEpochMsg{Tag: "UBX", Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	nmea := &NavEpochMsg{Tag: "NMEA", Acc: Accuracy{Hor: opt.Make(5 * Meter)}}
	m := MergeNavEpoch(ubx, nmea)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX (first wins)", m.Tag)
	}
	if m.Acc.Pos.Get() != 10*Meter {
		t.Errorf("Acc.Pos = %v, want 10m (from UBX)", m.Acc.Pos.Get())
	}
	if m.Acc.Hor.Get() != 5*Meter {
		t.Errorf("Acc.Hor = %v, want 5m (filled from NMEA)", m.Acc.Hor.Get())
	}
}

// testFlusher is a fake EpochFlusher for testing NavEpochManager.
type testFlusher struct {
	msg *NavEpochMsg
	pri MsgPriority
	mh  MsgHandler
}

func (f *testFlusher) FlushNavEpoch(tRead time.Time) (*NavEpochMsg, MsgPriority, MsgHandler) {
	msg := f.msg
	f.msg = nil
	return msg, f.pri, f.mh
}

// epochRecorder records NavEpoch calls.
type epochRecorder struct {
	DefaultHandler
	epochs []NavEpochMsg
	tReads []time.Time
}

func (r *epochRecorder) NavEpoch(msg *NavEpochMsg, tRead time.Time) {
	r.epochs = append(r.epochs, *msg)
	r.tReads = append(r.tReads, tRead)
}

func TestNavEpochManagerSingleProtocol(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	f := &testFlusher{pri: PriVendorLow, mh: rec}
	// First epoch start: f added to active set, no flush
	mgr.EpochStarted(f, t0)
	if len(rec.epochs) != 0 {
		t.Fatalf("expected no epochs after first EpochStarted, got %d", len(rec.epochs))
	}
	// Set up what will be flushed
	f.msg = &NavEpochMsg{Tag: "UBX", Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	// Second epoch start: triggers flush of first epoch
	mgr.EpochStarted(f, t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after second EpochStarted, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
	if rec.tReads[0] != t1 {
		t.Errorf("tRead = %v, want %v", rec.tReads[0], t1)
	}
	// Set up second epoch data
	f.msg = &NavEpochMsg{Tag: "UBX"}
	// Third epoch start: flushes second epoch
	mgr.EpochStarted(f, t2)
	if len(rec.epochs) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(rec.epochs))
	}
}

func TestNavEpochManagerMultiProtocol(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Millisecond)
	t2 := t0.Add(time.Second)
	binary := &testFlusher{pri: PriVendorLow, mh: rec}
	nmeaF := &testFlusher{pri: PriGenericHigh, mh: rec}
	// Epoch 1: both protocols register
	mgr.EpochStarted(binary, t0)
	mgr.EpochStarted(nmeaF, t1)
	if len(rec.epochs) != 0 {
		t.Fatalf("no flush expected in first epoch, got %d", len(rec.epochs))
	}
	// Set up epoch 1 data for flush
	binary.msg = &NavEpochMsg{Tag: "UBX", Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	nmeaF.msg = &NavEpochMsg{Tag: "NMEA", Acc: Accuracy{Hor: opt.Make(5 * Meter)}}
	// Epoch 2: binary starts first, triggering flush
	mgr.EpochStarted(binary, t2)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after binary starts epoch 2, got %d", len(rec.epochs))
	}
	m := rec.epochs[0]
	// Tag should come from binary (higher priority)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	// Accuracy should be merged
	if m.Acc.Pos.Get() != 10*Meter {
		t.Errorf("Acc.Pos = %v, want 10m", m.Acc.Pos.Get())
	}
	if m.Acc.Hor.Get() != 5*Meter {
		t.Errorf("Acc.Hor = %v, want 5m", m.Acc.Hor.Get())
	}
}

func TestNavEpochManagerEndOfEpochSingle(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	f := &testFlusher{pri: PriVendorLow, mh: rec}
	mgr.EpochStarted(f, t0)
	f.msg = &NavEpochMsg{Tag: "UBX"}
	// EndOfEpoch with one active processor: flushes immediately
	mgr.EndOfEpoch(t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after EndOfEpoch, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
}

func TestNavEpochManagerEndOfEpochMultiple(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	f1 := &testFlusher{pri: PriVendorLow, mh: rec}
	f2 := &testFlusher{pri: PriGenericHigh, mh: rec}
	mgr.EpochStarted(f1, t0)
	mgr.EpochStarted(f2, t0)
	f1.msg = &NavEpochMsg{Tag: "UBX"}
	f2.msg = &NavEpochMsg{Tag: "NMEA"}
	// EndOfEpoch with multiple active: flushes all processors
	mgr.EndOfEpoch(t0)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after EndOfEpoch, got %d", len(rec.epochs))
	}
	// UBX (PriVendorLow=3) wins over NMEA (PriGenericHigh=2) for Tag
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
}

func TestNavEpochManagerNilContribution(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	binary := &testFlusher{pri: PriVendorLow, mh: rec}
	casicF := &testFlusher{pri: PriVendorLow, mh: rec}
	// Epoch 1: both register
	mgr.EpochStarted(binary, t0)
	mgr.EpochStarted(casicF, t0)
	// Only binary has data; CASIC returns nil
	binary.msg = &NavEpochMsg{Tag: "UBX"}
	casicF.msg = nil
	// Epoch 2: triggers flush
	mgr.EpochStarted(binary, t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
	// Now test all-nil: no emission
	binary.msg = nil
	casicF.msg = nil
	mgr.EpochStarted(casicF, t1)
	mgr.EpochStarted(binary, t2)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected still 1 epoch (all nil), got %d", len(rec.epochs))
	}
}

// approxSpeed checks whether two Speed values are within tol m/s of each other.
func approxSpeed(a, b Speed, tol float64) bool {
	return math.Abs(a.MetersPerSecond()-b.MetersPerSecond()) < tol
}

// testPos is a geodetic position used across FillDerived tests.
// 47.5 N, 7.6 E, 500 m height.
var testPos = PosGeoMsg{
	LatLon: [2]Angle{DegreesFromFloat(47.5), DegreesFromFloat(7.6)},
	Height: opt.Make(Meters(500)),
	Tag:    "T",
}

func TestFillDerivedPosGeoToECEF(t *testing.T) {
	b := PVMsgBundle{PosGeo: opt.Make(testPos)}
	b.FillDerived()
	if !b.PosECEF.IsSet() {
		t.Fatal("PosECEF not derived from PosGeo")
	}
	pe := b.PosECEF.Get()
	// Sanity: ECEF should be on-Earth
	ecef := geopos.ECEF{pe.Pos[0].Meters(), pe.Pos[1].Meters(), pe.Pos[2].Meters()}
	if err := ecef.CheckOnEarth(); err != nil {
		t.Fatalf("derived ECEF not on Earth: %v", err)
	}
	if pe.Tag != "T" {
		t.Errorf("Tag = %v, want T", pe.Tag)
	}
}

func TestFillDerivedECEFToPosGeo(t *testing.T) {
	// Compute a known ECEF from the test position, then derive back.
	llh := geopos.LLH{Lat: 47.5, Lon: 7.6, Height: 500}
	ecef := geopos.WGS84.LLHtoECEF(llh)
	b := PVMsgBundle{
		PosECEF: opt.Make(PosECEFMsg{
			Pos: Point3D{Meters(ecef[0]), Meters(ecef[1]), Meters(ecef[2])},
			Tag: "E",
		}),
	}
	b.FillDerived()
	if !b.PosGeo.IsSet() {
		t.Fatal("PosGeo not derived from PosECEF")
	}
	pg := b.PosGeo.Get()
	if math.Abs(pg.LatLon[0].Degrees()-47.5) > 1e-6 {
		t.Errorf("Lat = %v, want ~47.5", pg.LatLon[0].Degrees())
	}
	if math.Abs(pg.LatLon[1].Degrees()-7.6) > 1e-6 {
		t.Errorf("Lon = %v, want ~7.6", pg.LatLon[1].Degrees())
	}
	if pg.Tag != "E" {
		t.Errorf("Tag = %v, want E", pg.Tag)
	}
}

func TestFillDerivedVelNEDToECEF(t *testing.T) {
	b := PVMsgBundle{
		PosGeo: opt.Make(testPos),
		VelGeo: opt.Make(VelGeoMsg{
			VelNED: opt.Make([3]Speed{MeterPerSecond, 2 * MeterPerSecond, 0}),
			Tag:    "V",
		}),
	}
	b.FillDerived()
	if !b.VelECEF.IsSet() {
		t.Fatal("VelECEF not derived from VelNED")
	}
	ve := b.VelECEF.Get()
	// Speed magnitude should be preserved.
	mag := math.Sqrt(
		ve.Vel[0].MetersPerSecond()*ve.Vel[0].MetersPerSecond() +
			ve.Vel[1].MetersPerSecond()*ve.Vel[1].MetersPerSecond() +
			ve.Vel[2].MetersPerSecond()*ve.Vel[2].MetersPerSecond())
	wantMag := math.Sqrt(1 + 4) // sqrt(1^2 + 2^2)
	if math.Abs(mag-wantMag) > 0.01 {
		t.Errorf("ECEF speed magnitude = %v, want ~%v", mag, wantMag)
	}
	if ve.Tag != "V" {
		t.Errorf("Tag = %v, want V", ve.Tag)
	}
}

func TestFillDerivedSpeed3DFromVelNED(t *testing.T) {
	b := PVMsgBundle{
		PosGeo: opt.Make(testPos),
		VelGeo: opt.Make(VelGeoMsg{
			VelNED: opt.Make([3]Speed{
				MetersPerSecondFromFloat(3),
				MetersPerSecondFromFloat(4),
				MetersPerSecondFromFloat(0),
			}),
		}),
	}
	b.FillDerived()
	vg := b.VelGeo.Get()
	if !vg.Speed3D.IsSet() {
		t.Fatal("Speed3D not derived from VelNED")
	}
	if !approxSpeed(vg.Speed3D.Get(), MetersPerSecondFromFloat(5), 0.001) {
		t.Errorf("Speed3D = %v, want ~5 m/s", vg.Speed3D.Get().MetersPerSecond())
	}
}

func TestFillDerivedVelECEFToNED(t *testing.T) {
	// Compute reference: start with known NED, convert to ECEF, then
	// verify FillDerived recovers the NED.
	lat, lon := 47.5, 7.6
	origNED := geopos.VelNED{1, 2, -0.5}
	ecefV := geopos.WGS84.NEDtoECEF(origNED, lat, lon)
	llh := geopos.LLH{Lat: lat, Lon: lon, Height: 500}
	ecefP := geopos.WGS84.LLHtoECEF(llh)
	b := PVMsgBundle{
		PosECEF: opt.Make(PosECEFMsg{
			Pos: Point3D{Meters(ecefP[0]), Meters(ecefP[1]), Meters(ecefP[2])},
		}),
		VelECEF: opt.Make(VelECEFMsg{
			Vel: [3]Speed{
				MetersPerSecondFromFloat(ecefV[0]),
				MetersPerSecondFromFloat(ecefV[1]),
				MetersPerSecondFromFloat(ecefV[2]),
			},
		}),
	}
	b.FillDerived()
	if !b.VelGeo.IsSet() {
		t.Fatal("VelGeo not derived from VelECEF")
	}
	vg := b.VelGeo.Get()
	if !vg.VelNED.IsSet() {
		t.Fatal("VelNED not set")
	}
	ned := vg.VelNED.Get()
	if !approxSpeed(ned[0], MetersPerSecondFromFloat(origNED[0]), 0.001) {
		t.Errorf("VelN = %v, want ~%v", ned[0].MetersPerSecond(), origNED[0])
	}
	if !approxSpeed(ned[1], MetersPerSecondFromFloat(origNED[1]), 0.001) {
		t.Errorf("VelE = %v, want ~%v", ned[1].MetersPerSecond(), origNED[1])
	}
	if !approxSpeed(ned[2], MetersPerSecondFromFloat(origNED[2]), 0.001) {
		t.Errorf("VelD = %v, want ~%v", ned[2].MetersPerSecond(), origNED[2])
	}
	if !vg.Speed3D.IsSet() {
		t.Fatal("Speed3D not derived")
	}
}

// TestFillDerivedPartialVelGeoWithVelECEF tests the mixed-source case:
// VelGeo exists with GroundSpeed/Course (from e.g. PQTMNAV) but no VelNED,
// VelECEF exists (from another source), position is known.
// FillDerived should fill VelNED and Speed3D into the existing VelGeo.
func TestFillDerivedPartialVelGeoWithVelECEF(t *testing.T) {
	lat, lon := 47.5, 7.6
	origNED := geopos.VelNED{1, 2, -0.5}
	ecefV := geopos.WGS84.NEDtoECEF(origNED, lat, lon)
	b := PVMsgBundle{
		PosGeo: opt.Make(testPos),
		VelGeo: opt.Make(VelGeoMsg{
			GroundSpeed: opt.Make(MetersPerSecondFromFloat(1.5)),
			Course:      opt.Make(DegreesFromFloat(45)),
			Tag:         "NAV",
		}),
		VelECEF: opt.Make(VelECEFMsg{
			Vel: [3]Speed{
				MetersPerSecondFromFloat(ecefV[0]),
				MetersPerSecondFromFloat(ecefV[1]),
				MetersPerSecondFromFloat(ecefV[2]),
			},
			Tag: "BIN",
		}),
	}
	b.FillDerived()
	vg := b.VelGeo.Get()
	// GroundSpeed and Course should be preserved from the original VelGeo.
	if !vg.GroundSpeed.IsSet() || vg.GroundSpeed.Get() != MetersPerSecondFromFloat(1.5) {
		t.Errorf("GroundSpeed = %v, want 1.5 m/s (preserved)", vg.GroundSpeed)
	}
	if !vg.Course.IsSet() || vg.Course.Get() != DegreesFromFloat(45) {
		t.Errorf("Course = %v, want 45 deg (preserved)", vg.Course)
	}
	// Tag should be preserved from the original VelGeo.
	if vg.Tag != "NAV" {
		t.Errorf("Tag = %v, want NAV (preserved)", vg.Tag)
	}
	// VelNED should be filled from VelECEF derivation.
	if !vg.VelNED.IsSet() {
		t.Fatal("VelNED not derived into existing VelGeo")
	}
	ned := vg.VelNED.Get()
	if !approxSpeed(ned[0], MetersPerSecondFromFloat(origNED[0]), 0.001) {
		t.Errorf("VelN = %v, want ~%v", ned[0].MetersPerSecond(), origNED[0])
	}
	if !approxSpeed(ned[1], MetersPerSecondFromFloat(origNED[1]), 0.001) {
		t.Errorf("VelE = %v, want ~%v", ned[1].MetersPerSecond(), origNED[1])
	}
	if !approxSpeed(ned[2], MetersPerSecondFromFloat(origNED[2]), 0.001) {
		t.Errorf("VelD = %v, want ~%v", ned[2].MetersPerSecond(), origNED[2])
	}
	// Speed3D should be derived.
	if !vg.Speed3D.IsSet() {
		t.Fatal("Speed3D not derived into existing VelGeo")
	}
	wantSpd := math.Sqrt(origNED[0]*origNED[0] + origNED[1]*origNED[1] + origNED[2]*origNED[2])
	if !approxSpeed(vg.Speed3D.Get(), MetersPerSecondFromFloat(wantSpd), 0.001) {
		t.Errorf("Speed3D = %v, want ~%v", vg.Speed3D.Get().MetersPerSecond(), wantSpd)
	}
}

// TestFillDerivedPartialVelGeoVelNEDNotOverwritten verifies that when
// VelGeo already has VelNED and VelECEF also exists, the existing VelNED
// is not overwritten.
func TestFillDerivedPartialVelGeoVelNEDNotOverwritten(t *testing.T) {
	lat, lon := 47.5, 7.6
	ecefV := geopos.WGS84.NEDtoECEF(geopos.VelNED{10, 20, 30}, lat, lon)
	origNED := [3]Speed{MeterPerSecond, 2 * MeterPerSecond, 3 * MeterPerSecond}
	b := PVMsgBundle{
		PosGeo: opt.Make(testPos),
		VelGeo: opt.Make(VelGeoMsg{
			VelNED: opt.Make(origNED),
		}),
		VelECEF: opt.Make(VelECEFMsg{
			Vel: [3]Speed{
				MetersPerSecondFromFloat(ecefV[0]),
				MetersPerSecondFromFloat(ecefV[1]),
				MetersPerSecondFromFloat(ecefV[2]),
			},
		}),
	}
	b.FillDerived()
	vg := b.VelGeo.Get()
	ned := vg.VelNED.Get()
	if ned != origNED {
		t.Errorf("VelNED = %v, want %v (not overwritten)", ned, origNED)
	}
}

func TestFillDerivedNoPosition(t *testing.T) {
	// Without position, velocity frame conversions are impossible.
	b := PVMsgBundle{
		VelGeo: opt.Make(VelGeoMsg{
			VelNED: opt.Make([3]Speed{MeterPerSecond, 0, 0}),
		}),
	}
	b.FillDerived()
	if b.VelECEF.IsSet() {
		t.Error("VelECEF should not be derived without position")
	}
	// Speed3D should still be derived from VelNED alone.
	vg := b.VelGeo.Get()
	if !vg.Speed3D.IsSet() {
		t.Error("Speed3D should be derived from VelNED even without position")
	}
}

func TestFillDerivedNoop(t *testing.T) {
	// Everything already set: FillDerived should not overwrite.
	lat, lon := 47.5, 7.6
	llh := geopos.LLH{Lat: lat, Lon: lon, Height: 500}
	ecefP := geopos.WGS84.LLHtoECEF(llh)
	origECEF := PosECEFMsg{Pos: Point3D{Meters(ecefP[0]), Meters(ecefP[1]), Meters(ecefP[2])}, Tag: "orig"}
	b := PVMsgBundle{
		PosGeo:  opt.Make(testPos),
		PosECEF: opt.Make(origECEF),
	}
	b.FillDerived()
	if b.PosECEF.Get().Tag != "orig" {
		t.Error("PosECEF overwritten when already set")
	}
}

func TestMsgType(t *testing.T) {
	tests := []struct {
		msg  Msg
		want string
	}{
		{&TimeMsg{}, "time"},
		{&PosGeoMsg{}, "posGeo"},
		{&PosECEFMsg{}, "posECEF"},
		{&VelGeoMsg{}, "velGeo"},
		{&VelECEFMsg{}, "velECEF"},
		{&LeapSecondMsg{}, "leapSecond"},
		{&SurveyMsg{}, "survey"},
		{&SatellitesMsg{}, "satellites"},
		{&NavEpochMsg{}, "navEpoch"},
		{&CorReportMsg{}, "corReport"},
	}
	for _, tt := range tests {
		if got := tt.msg.MsgType(); got != tt.want {
			t.Errorf("%T.MsgType() = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestCorReportSourceString(t *testing.T) {
	tests := []struct {
		s    CorReportSource
		want string
	}{
		{CorReportSourcePull, "pull"},
		{CorReportSourceReceiver, "receiver"},
		{CorReportSource(42), "CorReportSource(42)"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("CorReportSource(%d).String() = %q, want %q", uint8(tt.s), got, tt.want)
		}
	}
}

func TestCorReportSourceMarshalText(t *testing.T) {
	tests := []struct {
		s    CorReportSource
		want string
	}{
		{CorReportSourcePull, "pull"},
		{CorReportSourceReceiver, "receiver"},
	}
	for _, tt := range tests {
		got, err := tt.s.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%d): %v", uint8(tt.s), err)
		}
		if string(got) != tt.want {
			t.Errorf("CorReportSource(%d).MarshalText() = %q, want %q", uint8(tt.s), got, tt.want)
		}
	}
}

func TestCorReportMsgJSON(t *testing.T) {
	msg := CorReportMsg{
		Source:        CorReportSourcePull,
		Tag:           "RTCM",
		MsgID:         "1077",
		NBytes:        opt.Make(42),
		ChecksumOK:    opt.Make(true),
		RTCMRefBaseID: opt.Make(uint16(123)),
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"source":        "pull",
		"tag":           "RTCM",
		"msgID":         "1077",
		"nBytes":        float64(42),
		"checksumOK":    true,
		"rtcmRefBaseID": float64(123),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// corReportRecorder records CorReport calls for fan-out tests.
type corReportRecorder struct {
	DefaultHandler
	msgs   []*CorReportMsg
	tReads []time.Time
}

func (r *corReportRecorder) CorReport(msg *CorReportMsg, tRead time.Time) {
	r.msgs = append(r.msgs, msg)
	r.tReads = append(r.tReads, tRead)
}

func TestCorReportMsgDispatch(t *testing.T) {
	rec := &corReportRecorder{}
	msg := &CorReportMsg{Source: CorReportSourceReceiver, Tag: "RTCM", MsgID: "1077"}
	tRead := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	msg.Dispatch(rec, tRead)
	if len(rec.msgs) != 1 {
		t.Fatalf("expected 1 CorReport call, got %d", len(rec.msgs))
	}
	if rec.msgs[0] != msg {
		t.Errorf("got msg %p, want %p", rec.msgs[0], msg)
	}
	if rec.tReads[0] != tRead {
		t.Errorf("got tRead %v, want %v", rec.tReads[0], tRead)
	}
}

func TestMultiHandlerCorReport(t *testing.T) {
	r1 := &corReportRecorder{}
	r2 := &corReportRecorder{}
	mh := NewMultiHandler(r1, r2)
	msg := &CorReportMsg{Source: CorReportSourcePull, Tag: "RTCM", MsgID: "1077"}
	tRead := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	mh.CorReport(msg, tRead)
	for i, r := range []*corReportRecorder{r1, r2} {
		if len(r.msgs) != 1 || r.msgs[0] != msg {
			t.Errorf("handler %d: msgs = %v, want one call with %p", i, r.msgs, msg)
		}
	}
}

func TestGenericHandlerCorReport(t *testing.T) {
	var got Msg
	gh := &GenericHandler{Handle: func(m Msg, _ time.Time) { got = m }}
	msg := &CorReportMsg{Source: CorReportSourcePull}
	gh.CorReport(msg, time.Time{})
	if got != msg {
		t.Errorf("GenericHandler.CorReport: got %v, want %v", got, msg)
	}
}

func TestUnmarshalMsg(t *testing.T) {
	for name, factory := range msgRegistry {
		msg := factory()
		if msg.MsgType() != name {
			t.Errorf("registry %q: MsgType() = %q", name, msg.MsgType())
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("%q: marshal: %v", name, err)
		}
		got, err := UnmarshalMsg(name, data)
		if err != nil {
			t.Fatalf("%q: UnmarshalMsg: %v", name, err)
		}
		if got.MsgType() != name {
			t.Errorf("%q: round-tripped MsgType() = %q", name, got.MsgType())
		}
	}
	if _, err := UnmarshalMsg("bogus", []byte(`{}`)); err == nil {
		t.Error("UnmarshalMsg(bogus): expected error")
	}
}

func TestEventMarshal(t *testing.T) {
	msg := &PosGeoMsg{
		LatLon: [2]Angle{47 * Degrees, 8 * Degrees},
		Tag:    "UBX",
	}
	tRead := time.Date(2026, 3, 27, 10, 0, 0, 500000, time.UTC)
	ev := NewMsgEvent(msg, tRead, 123456*Microsecond)
	b, err := json.Marshal(&ev)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var typ string
	if err := json.Unmarshal(raw["type"], &typ); err != nil {
		t.Fatal(err)
	}
	if typ != "posGeo" {
		t.Errorf("type = %q, want %q", typ, "posGeo")
	}
	var mono float64
	if err := json.Unmarshal(raw["mono"], &mono); err != nil {
		t.Fatal(err)
	}
	if math.Abs(mono-0.123456) > 1e-9 {
		t.Errorf("mono = %v, want 0.123456", mono)
	}
	if _, ok := raw["data"]; !ok {
		t.Error("missing data field")
	}
	if _, ok := raw["t"]; !ok {
		t.Error("missing t field")
	}
}

// recordingNativeHandler records NativeMsg and HandledNativeMsg calls.
// It implements both NativeMsgHandler and HandledNativeMsgHandler.
type recordingNativeHandler struct {
	native  []string
	handled []string
}

func (h *recordingNativeHandler) NativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error {
	h.native = append(h.native, msgID)
	return nil
}

func (h *recordingNativeHandler) HandledNativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error {
	h.handled = append(h.handled, msgID)
	return nil
}

// plainNativeHandler implements only NativeMsgHandler, not the handled
// channel; the Multi handler must not blow up forwarding to it.
type plainNativeHandler struct{ native []string }

func (h *plainNativeHandler) NativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error {
	h.native = append(h.native, msgID)
	return nil
}

// TestMultiNativeMsgHandlerForwarding checks the forwarding trap: the
// Multi handler is what is installed during configuration, so it must
// forward HandledNativeMsg to children that implement the optional
// interface (and skip those that do not) - otherwise the packet
// processors' type assertion fails exactly when observations are needed.
func TestMultiNativeMsgHandlerForwarding(t *testing.T) {
	rec := &recordingNativeHandler{}
	plain := &plainNativeHandler{}
	m := NewMultiNativeMsgHandler(plain, rec)

	// A Multi still satisfies the optional interface, so a caller's type
	// assertion on the installed handler succeeds.
	hh, ok := any(m).(HandledNativeMsgHandler)
	if !ok {
		t.Fatal("MultiNativeMsgHandler does not implement HandledNativeMsgHandler")
	}
	if err := hh.HandledNativeMsg("NMEA", "GNRMC", nil, time.Time{}); err != nil {
		t.Fatalf("HandledNativeMsg: %v", err)
	}
	if err := m.NativeMsg("NMEA", "GPGSV", nil, time.Time{}); err != nil {
		t.Fatalf("NativeMsg: %v", err)
	}
	if len(rec.handled) != 1 || rec.handled[0] != "GNRMC" {
		t.Errorf("recording child handled = %v, want [GNRMC]", rec.handled)
	}
	if len(plain.native) != 1 || plain.native[0] != "GPGSV" {
		t.Errorf("plain child native = %v, want [GPGSV]", plain.native)
	}
	if len(rec.native) != 1 {
		t.Errorf("recording child native = %v, want [GPGSV]", rec.native)
	}
}
