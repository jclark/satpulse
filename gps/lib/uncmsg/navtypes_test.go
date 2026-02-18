package uncmsg

import "testing"

func TestSolStatusRoundTrip(t *testing.T) {
	tests := []struct {
		val SolStatus
		str string
	}{
		{SolComputed, "SOL_COMPUTED"},
		{InsufficientObs, "INSUFFICIENT_OBS"},
		{NoConvergence, "NO_CONVERGENCE"},
		{Singularity, "SINGULARITY"},
		{CovTrace, "COV_TRACE"},
	}
	for _, tc := range tests {
		if got := tc.val.String(); got != tc.str {
			t.Errorf("SolStatus(%d).String() = %q, want %q", tc.val, got, tc.str)
		}
		parsed, err := ParseSolStatus(tc.str)
		if err != nil {
			t.Errorf("ParseSolStatus(%q) error: %v", tc.str, err)
		} else if parsed != tc.val {
			t.Errorf("ParseSolStatus(%q) = %d, want %d", tc.str, parsed, tc.val)
		}
		b, err := tc.val.MarshalText()
		if err != nil {
			t.Errorf("SolStatus(%d).MarshalText() error: %v", tc.val, err)
		}
		var unmarshaled SolStatus
		if err := unmarshaled.UnmarshalText(b); err != nil {
			t.Errorf("SolStatus.UnmarshalText(%q) error: %v", b, err)
		} else if unmarshaled != tc.val {
			t.Errorf("SolStatus.UnmarshalText(%q) = %d, want %d", b, unmarshaled, tc.val)
		}
	}
}

func TestSolStatusUnknown(t *testing.T) {
	s := SolStatus(99)
	if got := s.String(); got != "99" {
		t.Errorf("SolStatus(99).String() = %q, want %q", got, "99")
	}
	if _, err := ParseSolStatus("BOGUS"); err == nil {
		t.Error("ParseSolStatus(BOGUS) should return error")
	}
}

func TestPosVelTypeRoundTrip(t *testing.T) {
	tests := []struct {
		val PosVelType
		str string
	}{
		{PosVelNone, "NONE"},
		{PosVelFixedPos, "FIXEDPOS"},
		{PosVelFixedHeight, "FIXEDHEIGHT"},
		{PosVelDopplerVelocity, "DOPPLER_VELOCITY"},
		{PosVelSingle, "SINGLE"},
		{PosVelPSRDiff, "PSRDIFF"},
		{PosVelSBAS, "SBAS"},
		{PosVelL1Float, "L1_FLOAT"},
		{PosVelIonoFreeFloat, "IONOFREE_FLOAT"},
		{PosVelNarrowFloat, "NARROW_FLOAT"},
		{PosVelL1Int, "L1_INT"},
		{PosVelWideInt, "WIDE_INT"},
		{PosVelNarrowInt, "NARROW_INT"},
		{PosVelINS, "INS"},
		{PosVelINSPSRSP, "INS_PSRSP"},
		{PosVelINSPSRDiff, "INS_PSRDIFF"},
		{PosVelINSRTKFloat, "INS_RTKFLOAT"},
		{PosVelINSRTKFixed, "INS_RTKFIXED"},
		{PosVelPPPConverging, "PPP_CONVERGING"},
		{PosVelPPP, "PPP"},
		{PosVelPPPAR, "PPP_AR"},
		{PosVelPPPRTK, "PPP_RTK"},
	}
	for _, tc := range tests {
		if got := tc.val.String(); got != tc.str {
			t.Errorf("PosVelType(%d).String() = %q, want %q", tc.val, got, tc.str)
		}
		parsed, err := ParsePosVelType(tc.str)
		if err != nil {
			t.Errorf("ParsePosVelType(%q) error: %v", tc.str, err)
		} else if parsed != tc.val {
			t.Errorf("ParsePosVelType(%q) = %d, want %d", tc.str, parsed, tc.val)
		}
		b, err := tc.val.MarshalText()
		if err != nil {
			t.Errorf("PosVelType(%d).MarshalText() error: %v", tc.val, err)
		}
		var unmarshaled PosVelType
		if err := unmarshaled.UnmarshalText(b); err != nil {
			t.Errorf("PosVelType.UnmarshalText(%q) error: %v", b, err)
		} else if unmarshaled != tc.val {
			t.Errorf("PosVelType.UnmarshalText(%q) = %d, want %d", b, unmarshaled, tc.val)
		}
	}
}

func TestPosVelTypePPPLowercase(t *testing.T) {
	parsed, err := ParsePosVelType("ppp")
	if err != nil {
		t.Fatalf("ParsePosVelType(ppp) error: %v", err)
	}
	if parsed != PosVelPPP {
		t.Errorf("ParsePosVelType(ppp) = %d, want %d", parsed, PosVelPPP)
	}
}

func TestPosVelTypeUnknown(t *testing.T) {
	p := PosVelType(255)
	if got := p.String(); got != "255" {
		t.Errorf("PosVelType(255).String() = %q, want %q", got, "255")
	}
	if _, err := ParsePosVelType("BOGUS"); err == nil {
		t.Error("ParsePosVelType(BOGUS) should return error")
	}
}

func TestStationIDUnmarshalText(t *testing.T) {
	tests := []struct {
		input string
		want  StationID
	}{
		{`""`, StationID{}},
		{`"0"`, StationID{'0'}},
		{`"ABCD"`, StationID{'A', 'B', 'C', 'D'}},
	}
	for _, tc := range tests {
		var s StationID
		if err := s.UnmarshalText([]byte(tc.input)); err != nil {
			t.Errorf("StationID.UnmarshalText(%q) error: %v", tc.input, err)
		} else if s != tc.want {
			t.Errorf("StationID.UnmarshalText(%q) = %v, want %v", tc.input, s, tc.want)
		}
	}
}

func TestStationIDMarshalRoundTrip(t *testing.T) {
	tests := []struct {
		id  StationID
		str string
	}{
		{StationID{}, `""`},
		{StationID{'0'}, `"0"`},
		{StationID{'A', 'B'}, `"AB"`},
	}
	for _, tc := range tests {
		b, err := tc.id.MarshalText()
		if err != nil {
			t.Errorf("StationID(%v).MarshalText() error: %v", tc.id, err)
		}
		if string(b) != tc.str {
			t.Errorf("StationID(%v).MarshalText() = %q, want %q", tc.id, b, tc.str)
		}
		var s StationID
		if err := s.UnmarshalText(b); err != nil {
			t.Errorf("StationID.UnmarshalText(%q) error: %v", b, err)
		} else if s != tc.id {
			t.Errorf("StationID.UnmarshalText(%q) = %v, want %v", b, s, tc.id)
		}
	}
}
