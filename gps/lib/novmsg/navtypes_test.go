package novmsg

import "testing"

func TestSolStatusRoundTrip(t *testing.T) {
	vals := []struct {
		v SolStatus
		s string
	}{
		{SolComputed, "SOL_COMPUTED"},
		{InsufficientObs, "INSUFFICIENT_OBS"},
		{NoConvergence, "NO_CONVERGENCE"},
		{Singularity, "SINGULARITY"},
		{CovTrace, "COV_TRACE"},
		{TestDist, "TEST_DIST"},
		{ColdStart, "COLD_START"},
		{VHLimit, "V_H_LIMIT"},
		{Variance, "VARIANCE"},
		{Residuals, "RESIDUALS"},
		{IntegrityWarning, "INTEGRITY_WARNING"},
		{Pending, "PENDING"},
		{InvalidFix, "INVALID_FIX"},
		{Unauthorized, "UNAUTHORIZED"},
		{InvalidRate, "INVALID_RATE"},
	}
	for _, tt := range vals {
		if got := tt.v.String(); got != tt.s {
			t.Errorf("SolStatus(%d).String() = %q, want %q", tt.v, got, tt.s)
		}
		parsed, err := ParseSolStatus(tt.s)
		if err != nil {
			t.Errorf("ParseSolStatus(%q) error: %v", tt.s, err)
		} else if parsed != tt.v {
			t.Errorf("ParseSolStatus(%q) = %d, want %d", tt.s, parsed, tt.v)
		}
		// MarshalText/UnmarshalText round-trip
		text, err := tt.v.MarshalText()
		if err != nil {
			t.Errorf("SolStatus(%d).MarshalText() error: %v", tt.v, err)
			continue
		}
		var unmarshaled SolStatus
		if err := unmarshaled.UnmarshalText(text); err != nil {
			t.Errorf("SolStatus.UnmarshalText(%q) error: %v", text, err)
		} else if unmarshaled != tt.v {
			t.Errorf("SolStatus.UnmarshalText(%q) = %d, want %d", text, unmarshaled, tt.v)
		}
	}
}

func TestSolStatusUnknown(t *testing.T) {
	if got := SolStatus(99).String(); got != "99" {
		t.Errorf("SolStatus(99).String() = %q, want %q", got, "99")
	}
	if _, err := ParseSolStatus("BOGUS"); err == nil {
		t.Error("ParseSolStatus(BOGUS) should return error")
	}
}

func TestPosTypeRoundTrip(t *testing.T) {
	vals := []struct {
		v PosType
		s string
	}{
		{PosNone, "NONE"},
		{PosFixedPos, "FIXEDPOS"},
		{PosFixedHeight, "FIXEDHEIGHT"},
		{PosFloatConv, "FLOATCONV"},
		{PosWideLane, "WIDELANE"},
		{PosNarrowLane, "NARROWLANE"},
		{PosDopplerVelocity, "DOPPLER_VELOCITY"},
		{PosSingle, "SINGLE"},
		{PosPSRDiff, "PSRDIFF"},
		{PosWAAS, "WAAS"},
		{PosPropagated, "PROPAGATED"},
		{PosL1Float, "L1_FLOAT"},
		{PosIonoFreeFloat, "IONOFREE_FLOAT"},
		{PosNarrowFloat, "NARROW_FLOAT"},
		{PosL1Int, "L1_INT"},
		{PosWideInt, "WIDE_INT"},
		{PosNarrowInt, "NARROW_INT"},
		{PosRTKDirectINS, "RTK_DIRECT_INS"},
		{PosINSSBAS, "INS_SBAS"},
		{PosINSPSRSP, "INS_PSRSP"},
		{PosINSPSRDiff, "INS_PSRDIFF"},
		{PosINSRTKFloat, "INS_RTKFLOAT"},
		{PosINSRTKFixed, "INS_RTKFIXED"},
		{PosExtConstrained, "EXT_CONSTRAINED"},
		{PosPPPConverging, "PPP_CONVERGING"},
		{PosPPP, "PPP"},
		{PosOperational, "OPERATIONAL"},
		{PosWarning, "WARNING"},
		{PosOutOfBounds, "OUT_OF_BOUNDS"},
		{PosINSPPPConverging, "INS_PPP_CONVERGING"},
		{PosINSPPP, "INS_PPP"},
		{PosPPPBasicConverging, "PPP_BASIC_CONVERGING"},
		{PosPPPBasic, "PPP_BASIC"},
		{PosINSPPPBasicConverging, "INS_PPP_BASIC_CONVERGING"},
		{PosINSPPPBasic, "INS_PPP_BASIC"},
	}
	for _, tt := range vals {
		if got := tt.v.String(); got != tt.s {
			t.Errorf("PosType(%d).String() = %q, want %q", tt.v, got, tt.s)
		}
		parsed, err := ParsePosType(tt.s)
		if err != nil {
			t.Errorf("ParsePosType(%q) error: %v", tt.s, err)
		} else if parsed != tt.v {
			t.Errorf("ParsePosType(%q) = %d, want %d", tt.s, parsed, tt.v)
		}
		text, err := tt.v.MarshalText()
		if err != nil {
			t.Errorf("PosType(%d).MarshalText() error: %v", tt.v, err)
			continue
		}
		var unmarshaled PosType
		if err := unmarshaled.UnmarshalText(text); err != nil {
			t.Errorf("PosType.UnmarshalText(%q) error: %v", text, err)
		} else if unmarshaled != tt.v {
			t.Errorf("PosType.UnmarshalText(%q) = %d, want %d", text, unmarshaled, tt.v)
		}
	}
}

func TestPosTypeUnknown(t *testing.T) {
	if got := PosType(99).String(); got != "99" {
		t.Errorf("PosType(99).String() = %q, want %q", got, "99")
	}
	if _, err := ParsePosType("BOGUS"); err == nil {
		t.Error("ParsePosType(BOGUS) should return error")
	}
}

func TestSinoPosTypeRoundTrip(t *testing.T) {
	vals := []struct {
		v SinoPosType
		s string
	}{
		{PosNone, "NONE"},
		{PosFixedPos, "FIXEDPOS"},
		{PosDopplerVelocity, "DOPPLER_VELOCITY"},
		{SinoPosSingleSmooth, "SINGLE_SMOOTH"},
		{PosSingle, "SINGLE"},
		{PosPSRDiff, "PSRDIFF"},
		{PosSBAS, "SBAS"},
		{PosNarrowFloat, "NARROW_FLOAT"},
		{SinoPosFIXDerivation, "FIX_DERIVATION"},
		{PosWideInt, "WIDE_INT"},
		{PosNarrowInt, "NARROW_INT"},
		{SinoPosSuperWideLane, "SUPER_WIDE_LANE"},
		{PosPPPConverging, "PPP_CONVERGING"},
		{PosPPP, "PPP"},
	}
	for _, tt := range vals {
		if got := tt.v.String(); got != tt.s {
			t.Errorf("SinoPosType(%d).String() = %q, want %q", tt.v, got, tt.s)
		}
		parsed, err := ParseSinoPosType(tt.s)
		if err != nil {
			t.Errorf("ParseSinoPosType(%q) error: %v", tt.s, err)
		} else if parsed != tt.v {
			t.Errorf("ParseSinoPosType(%q) = %d, want %d", tt.s, parsed, tt.v)
		}
		text, err := tt.v.MarshalText()
		if err != nil {
			t.Errorf("SinoPosType(%d).MarshalText() error: %v", tt.v, err)
			continue
		}
		var unmarshaled SinoPosType
		if err := unmarshaled.UnmarshalText(text); err != nil {
			t.Errorf("SinoPosType.UnmarshalText(%q) error: %v", text, err)
		} else if unmarshaled != tt.v {
			t.Errorf("SinoPosType.UnmarshalText(%q) = %d, want %d", text, unmarshaled, tt.v)
		}
	}
}

func TestSinoPosTypeUnknown(t *testing.T) {
	if got := SinoPosType(99).String(); got != "99" {
		t.Errorf("SinoPosType(99).String() = %q, want %q", got, "99")
	}
	if _, err := ParseSinoPosType("BOGUS"); err == nil {
		t.Error("ParseSinoPosType(BOGUS) should return error")
	}
}
